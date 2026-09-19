package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func setupMediaModelsTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.AuditLog{}))
	original := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() { common.MemoryCacheEnabled = original })
	return db
}

func setupMediaPolicyEngineDB(t *testing.T, envName string) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(envName))
	if dsn == "" {
		t.Skipf("%s is not configured", envName)
	}
	originalDSN, hadDSN := os.LookupEnv("SQL_DSN")
	originalMaster := common.IsMasterNode
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	common.IsMasterNode = false
	require.NoError(t, os.Setenv("SQL_DSN", dsn))
	require.NoError(t, model.InitDB())
	db := model.DB
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.IsMasterNode = originalMaster
		if hadDSN {
			_ = os.Setenv("SQL_DSN", originalDSN)
		} else {
			_ = os.Unsetenv("SQL_DSN")
		}
		common.SetDatabaseTypes(originalMainType, originalLogType)

	})
	return db
}

func intPointer(value int) *int {
	return &value
}

func mediaImageProfile(id string) model.MediaModelProfile {
	return model.MediaModelProfile{
		ID:   id,
		Type: "image",
		Operations: map[string]model.MediaOperation{
			"text_to_image": {
				Protocol: "openai_images",
				Path:     "/v1/images/generations",
				Parameters: map[string]model.MediaParameter{
					"n": {Type: "integer", Minimum: intPointer(1), Maximum: intPointer(4)},
				},
				Reference: model.MediaReference{Input: "none", MaxImages: 0},
			},
			"image_to_image": {
				Protocol:   "openai_images",
				Path:       "/v1/images/edits",
				Parameters: map[string]model.MediaParameter{},
				Reference:  model.MediaReference{Input: "inline", MaxImages: 1},
			},
		},
	}
}

func mediaVideoProfile(id string) model.MediaModelProfile {
	return model.MediaModelProfile{
		ID:   id,
		Type: "video",
		Operations: map[string]model.MediaOperation{
			"text_to_video": {
				Protocol: "openai_video",
				Path:     "/v1/videos",
				Parameters: map[string]model.MediaParameter{
					"seconds": {Type: "integer", Minimum: intPointer(1), Maximum: intPointer(15)},
				},
				Reference: model.MediaReference{Input: "none", MaxImages: 0},
			},
		},
	}
}

func mediaValidPolicy() model.MediaPolicy {
	return model.MediaPolicy{
		Version:       1,
		Models:        []model.MediaModelProfile{mediaImageProfile("media-img-1"), mediaVideoProfile("media-video-1")},
		ImagePriority: []string{"media-img-1"},
		VideoPriority: []string{"media-video-1"},
	}
}

func seedMediaPolicy(t *testing.T, policy model.MediaPolicy) model.MediaPolicySnapshot {
	t.Helper()
	snapshot, err := model.GetMediaPolicy()
	require.NoError(t, err)
	updated, err := model.UpdateMediaPolicy(snapshot.ConfigVersion, policy)
	require.NoError(t, err)
	return *updated
}

func seedMediaUser(t *testing.T, db *gorm.DB, id int, group string) model.User {
	t.Helper()
	user := model.User{Id: id, Username: fmt.Sprintf("media-user-%d", id), Password: "password", Group: group, Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func seedMediaChannel(t *testing.T, db *gorm.DB, channel *model.Channel, group string, models ...string) {
	t.Helper()
	channel.Status = common.ChannelStatusEnabled
	channel.Group = group
	channel.Models = strings.Join(models, ",")
	require.NoError(t, db.Create(channel).Error)
	for _, name := range models {
		require.NoError(t, db.Create(&model.Ability{Group: group, Model: name, ChannelId: channel.Id, Enabled: true}).Error)
	}
}

func newMediaCatalogContext(t *testing.T, userID int, query string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/media/models"+query, nil)
	context.Set("id", userID)
	return context, recorder
}

func mediaPolicyExercise(t *testing.T, db *gorm.DB) {
	t.Helper()

	snapshot, err := model.GetMediaPolicy()
	require.NoError(t, err)
	require.NotEmpty(t, snapshot.ConfigVersion)
	assert.Equal(t, 1, snapshot.Policy.Version)
	assert.NotNil(t, snapshot.Policy.Models)
	assert.Empty(t, snapshot.Policy.Models)
	assert.NotNil(t, snapshot.Policy.ImagePriority)
	assert.NotNil(t, snapshot.Policy.VideoPriority)

	hinted := mediaValidPolicy()
	hinted.Models[0].SelectionHint = "Prefer reference consistency."
	updated, err := model.UpdateMediaPolicy(snapshot.ConfigVersion, hinted)
	require.NoError(t, err)
	assert.NotEqual(t, snapshot.ConfigVersion, updated.ConfigVersion)
	require.Len(t, updated.Policy.Models, 2)
	assert.Equal(t, "media-img-1", updated.Policy.Models[0].ID)
	assert.Equal(t, "Prefer reference consistency.", updated.Policy.Models[0].SelectionHint)
	assert.Empty(t, updated.Policy.Models[1].SelectionHint)
	assert.Equal(t, []string{"media-img-1"}, updated.Policy.ImagePriority)

	loaded, err := model.GetMediaPolicy()
	require.NoError(t, err)
	assert.Equal(t, updated.ConfigVersion, loaded.ConfigVersion)
	require.Len(t, loaded.Policy.Models, 2)
	assert.Equal(t, "Prefer reference consistency.", loaded.Policy.Models[0].SelectionHint)

	_, err = model.UpdateMediaPolicy(snapshot.ConfigVersion, mediaValidPolicy())
	require.ErrorIs(t, err, model.ErrMediaPolicyConflict)

	_, err = model.UpdateMediaPolicy("", mediaValidPolicy())
	require.ErrorIs(t, err, model.ErrMediaPolicyConflict)

	require.Error(t, model.UpdateOption(model.MediaModelsPolicyOption, `{"version":1}`))
	require.Error(t, model.UpdateOptionsBulk(map[string]string{model.MediaModelsPolicyOption: `{"version":1}`}))

	invalid := mediaValidPolicy()
	invalid.Models = append(invalid.Models, mediaImageProfile("media-img-1"))
	_, err = model.UpdateMediaPolicy(loaded.ConfigVersion, invalid)
	require.Error(t, err)

	loadedAfter, err := model.GetMediaPolicy()
	require.NoError(t, err)
	assert.Equal(t, updated.ConfigVersion, loadedAfter.ConfigVersion)
	require.Len(t, loadedAfter.Policy.Models, 2)
}

func TestMediaModelsPolicyDatabaseMatrix(t *testing.T) {
	t.Run("sqlite", func(t *testing.T) {
		db := setupMediaModelsTestDB(t)
		mediaPolicyExercise(t, db)
	})
	t.Run("mysql", func(t *testing.T) {
		db := setupMediaPolicyEngineDB(t, "TEST_MEDIA_MYSQL_DSN")
		mediaPolicyExercise(t, db)
	})
	t.Run("postgres", func(t *testing.T) {
		db := setupMediaPolicyEngineDB(t, "TEST_MEDIA_POSTGRES_DSN")
		mediaPolicyExercise(t, db)
	})
}

func TestValidateMediaPolicyRejectsMalformedPolicies(t *testing.T) {
	valid := mediaValidPolicy()
	assert.NoError(t, model.ValidateMediaPolicy(valid))

	cases := []struct {
		name   string
		mutate func(*model.MediaPolicy)
	}{
		{"bad version", func(p *model.MediaPolicy) { p.Version = 2 }},
		{"nil models", func(p *model.MediaPolicy) { p.Models = nil }},
		{"nil image priority", func(p *model.MediaPolicy) { p.ImagePriority = nil }},
		{"duplicate model ids", func(p *model.MediaPolicy) {
			p.Models = append(p.Models, mediaImageProfile("media-img-1"))
		}},
		{"bad model type", func(p *model.MediaPolicy) { p.Models[0].Type = "audio" }},
		{"empty operations", func(p *model.MediaPolicy) { p.Models[0].Operations = map[string]model.MediaOperation{} }},
		{"operation mismatched type", func(p *model.MediaPolicy) {
			p.Models[0].Operations["text_to_video"] = p.Models[0].Operations["text_to_image"]
		}},
		{"unknown protocol", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["text_to_image"]
			op.Protocol = "mystery_protocol"
			p.Models[0].Operations["text_to_image"] = op
		}},
		{"mismatched path", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["text_to_image"]
			op.Path = "/v1/images/edits"
			p.Models[0].Operations["text_to_image"] = op
		}},
		{"protocol with missing operation and empty path", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["text_to_image"]
			op.Protocol, op.Path = "openai_video", ""
			p.Models[0].Operations["text_to_image"] = op
		}},
		{"count cannot be a string", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["text_to_image"]
			op.Parameters["n"] = model.MediaParameter{Type: "string"}
			p.Models[0].Operations["text_to_image"] = op
		}},
		{"nil parameters", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["text_to_image"]
			op.Parameters = nil
			p.Models[0].Operations["text_to_image"] = op
		}},
		{"unknown parameter", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["text_to_image"]
			op.Parameters["webhook_url"] = model.MediaParameter{Type: "string"}
			p.Models[0].Operations["text_to_image"] = op
		}},
		{"string parameter with bounds", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["text_to_image"]
			op.Parameters["size"] = model.MediaParameter{Type: "string", Minimum: intPointer(1), Maximum: intPointer(2)}
			p.Models[0].Operations["text_to_image"] = op
		}},
		{"integer parameter missing bounds", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["text_to_image"]
			op.Parameters["n"] = model.MediaParameter{Type: "integer", Minimum: intPointer(1)}
			p.Models[0].Operations["text_to_image"] = op
		}},
		{"integer parameter unordered bounds", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["text_to_image"]
			op.Parameters["n"] = model.MediaParameter{Type: "integer", Minimum: intPointer(5), Maximum: intPointer(2)}
			p.Models[0].Operations["text_to_image"] = op
		}},
		{"n maximum above cap", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["text_to_image"]
			op.Parameters["n"] = model.MediaParameter{Type: "integer", Minimum: intPointer(1), Maximum: intPointer(1000)}
			p.Models[0].Operations["text_to_image"] = op
		}},
		{"seconds maximum above cap", func(p *model.MediaPolicy) {
			op := p.Models[1].Operations["text_to_video"]
			op.Parameters["seconds"] = model.MediaParameter{Type: "integer", Minimum: intPointer(1), Maximum: intPointer(999999)}
			p.Models[1].Operations["text_to_video"] = op
		}},
		{"enum duplicates", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["text_to_image"]
			op.Parameters["size"] = model.MediaParameter{Type: "string", Enum: []string{"1x", "1x"}}
			p.Models[0].Operations["text_to_image"] = op
		}},
		{"enum blank value", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["text_to_image"]
			op.Parameters["size"] = model.MediaParameter{Type: "string", Enum: []string{"  "}}
			p.Models[0].Operations["text_to_image"] = op
		}},
		{"text operation with reference", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["text_to_image"]
			op.Reference = model.MediaReference{Input: "inline", MaxImages: 1}
			p.Models[0].Operations["text_to_image"] = op
		}},
		{"image operation without reference", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["image_to_image"]
			op.Reference = model.MediaReference{Input: "none", MaxImages: 0}
			p.Models[0].Operations["image_to_image"] = op
		}},
		{"image reference above bound", func(p *model.MediaPolicy) {
			op := p.Models[0].Operations["image_to_image"]
			op.Reference = model.MediaReference{Input: "inline", MaxImages: 17}
			p.Models[0].Operations["image_to_image"] = op
		}},
		{"video reference above one", func(p *model.MediaPolicy) {
			p.Models[1].Operations["image_to_video"] = model.MediaOperation{
				Protocol:   "openai_video",
				Path:       "/v1/videos",
				Parameters: map[string]model.MediaParameter{},
				Reference:  model.MediaReference{Input: "url", MaxImages: 2},
			}
		}},
		{"gemini reference must be inline", func(p *model.MediaPolicy) {
			p.Models[0].Operations["image_to_image"] = model.MediaOperation{
				Protocol:   "gemini_generate_content",
				Path:       "/v1beta/models/{model}:generateContent",
				Parameters: map[string]model.MediaParameter{},
				Reference:  model.MediaReference{Input: "url", MaxImages: 1},
			}
		}},
		{"model id with whitespace", func(p *model.MediaPolicy) { p.Models[0].ID = "media img" }},
		{"model id with query marker", func(p *model.MediaPolicy) { p.Models[0].ID = "media?x=1" }},
		{"model id with fragment", func(p *model.MediaPolicy) { p.Models[0].ID = "media#x" }},
		{"model id with percent", func(p *model.MediaPolicy) { p.Models[0].ID = "media%20" }},
		{"model id with backslash", func(p *model.MediaPolicy) { p.Models[0].ID = `media\id` }},
		{"model id too long", func(p *model.MediaPolicy) { p.Models[0].ID = strings.Repeat("a", 129) }},
		{"duplicate image priority", func(p *model.MediaPolicy) {
			p.ImagePriority = []string{"media-img-1", "media-img-1"}
		}},
		{"unknown image priority", func(p *model.MediaPolicy) { p.ImagePriority = []string{"ghost"} }},
		{"video priority with image model", func(p *model.MediaPolicy) {
			p.VideoPriority = []string{"media-img-1"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := mediaValidPolicy()
			tc.mutate(&policy)
			assert.Error(t, model.ValidateMediaPolicy(policy), tc.name)
		})
	}
}

func TestValidateMediaPolicyRawRejectsUnsupportedKeys(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"policy key", `{"version":1,"auth":{"key":"x"},"models":[],"image_priority":[],"video_priority":[]}`},
		{"model key", `{"version":1,"models":[{"id":"m","type":"image","base_url":"https://x","operations":{}}],"image_priority":[],"video_priority":[]}`},
		{"operation key", `{"version":1,"models":[{"id":"m","type":"image","operations":{"text_to_image":{"protocol":"openai_images","path":"/v1/images/generations","parameters":{},"reference":{"input":"none","max_images":0},"headers":{}}}}],"image_priority":[],"video_priority":[]}`},
		{"parameter key", `{"version":1,"models":[{"id":"m","type":"image","operations":{"text_to_image":{"protocol":"openai_images","path":"/v1/images/generations","parameters":{"n":{"type":"integer","minimum":1,"maximum":1,"secret":"x"}},"reference":{"input":"none","max_images":0}}}}],"image_priority":[],"video_priority":[]}`},
		{"reference key", `{"version":1,"models":[{"id":"m","type":"image","operations":{"text_to_image":{"protocol":"openai_images","path":"/v1/images/generations","parameters":{},"reference":{"input":"none","max_images":0,"upload_url":"x"}}}}],"image_priority":[],"video_priority":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Error(t, model.ValidateMediaPolicyRaw([]byte(tc.body)), tc.name)
		})
	}
	assert.NoError(t, model.ValidateMediaPolicyRaw([]byte(`{"version":1,"models":[],"image_priority":[],"video_priority":[]}`)))
}

func TestMediaModelsAdminPolicyEndpoints(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	require.NoError(t, db.Create(&model.Model{
		ModelName: "media-img-1",
		NameRule:  model.NameRuleExact,
		Endpoints: `{"image-generation":"/v1/images/generations"}`,
	}).Error)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/option/media_models", GetMediaModelsPolicy)
	engine.PUT("/api/option/media_models", UpdateMediaModelsPolicy)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/option/media_models", nil)
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	payload := gjson.Parse(recorder.Body.String())
	assert.True(t, payload.Get("success").Bool())
	configVersion := payload.Get("data.config_version").String()
	require.NotEmpty(t, configVersion)
	assert.Equal(t, float64(1), payload.Get("data.policy.version").Float())
	assert.True(t, payload.Get("data.policy.models").IsArray())

	put := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPut, "/api/option/media_models", strings.NewReader(body))
		engine.ServeHTTP(recorder, request)
		return recorder
	}

	recorder = put(`{"policy":{"version":1,"models":[],"image_priority":[],"video_priority":[]}}`)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Equal(t, "invalid_media_policy", gjson.Parse(recorder.Body.String()).Get("code").String())

	recorder = put(`{invalid json`)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	recorder = put(fmt.Sprintf(`{"expected_version":%q,"policy":{"version":2,"models":[],"image_priority":[],"video_priority":[]}}`, configVersion))
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	policyJSON := `{"version":1,"models":[{"id":"media-img-1","type":"image","operations":{"text_to_image":{"protocol":"openai_images","path":"/v1/images/generations","parameters":{"n":{"type":"integer","minimum":1,"maximum":4}},"reference":{"input":"none","max_images":0}}}}],"image_priority":["media-img-1"],"video_priority":[]}`
	recorder = put(fmt.Sprintf(`{"expected_version":%q,"policy":%s}`, "stale-version", policyJSON))
	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Equal(t, "media_policy_conflict", gjson.Parse(recorder.Body.String()).Get("code").String())

	recorder = put(fmt.Sprintf(`{"expected_version":%q,"policy":%s}`, configVersion, policyJSON))
	require.Equal(t, http.StatusOK, recorder.Code)
	payload = gjson.Parse(recorder.Body.String())
	assert.True(t, payload.Get("success").Bool())
	newVersion := payload.Get("data.config_version").String()
	require.NotEmpty(t, newVersion)
	assert.NotEqual(t, configVersion, newVersion)
	assert.Equal(t, "media-img-1", payload.Get("data.policy.models.0.id").String())

	oversized := `{"expected_version":"x","policy":{"version":1,"models":[],"image_priority":[],"video_priority":[],"` + strings.Repeat("a", mediaPolicyMaxBodyBytes)
	recorder = put(oversized)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestMediaModelsCatalogEmptyPolicy(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	user := seedMediaUser(t, db, 7001, "default")
	context, recorder := newMediaCatalogContext(t, user.Id, "")
	MediaModels(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := gjson.Parse(recorder.Body.String())
	assert.True(t, payload.Get("success").Bool())
	assert.Equal(t, float64(1), payload.Get("version").Float())
	assert.Equal(t, "media_model_catalog", payload.Get("object").String())
	assert.True(t, payload.Get("data").IsArray())
	assert.Equal(t, float64(0), payload.Get("data.#").Float())
	assert.NotEmpty(t, payload.Get("config_version").String())
	assert.NotEmpty(t, payload.Get("generated_at").String())
	assert.Equal(t, "no_available_model", payload.Get("defaults.image.reason_code").String())
	assert.Equal(t, "no_available_model", payload.Get("defaults.video.reason_code").String())
	assert.Equal(t, gjson.Null, payload.Get("defaults.image.model").Type)
}

func TestMediaModelsCatalogRoutableOperationsAndDefaults(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7002, "default")
	seedMediaPolicy(t, mediaValidPolicy())
	seedMediaChannel(t, db, &model.Channel{Id: 7101, Type: constant.ChannelTypeOpenAI, Name: "img", Key: "sk-img-secret"}, "default", "media-img-1")
	seedMediaChannel(t, db, &model.Channel{Id: 7102, Type: constant.ChannelTypeGemini, Name: "img2", Key: "sk-gem-secret"}, "default", "media-img-2")

	geminiProfile := mediaImageProfile("media-img-2")
	geminiProfile.Operations = map[string]model.MediaOperation{
		"text_to_image": {
			Protocol:   "gemini_generate_content",
			Path:       "/v1beta/models/{model}:generateContent",
			Parameters: map[string]model.MediaParameter{},
			Reference:  model.MediaReference{Input: "none", MaxImages: 0},
		},
	}
	snapshot, err := model.GetMediaPolicy()
	require.NoError(t, err)
	policy := mediaValidPolicy()
	policy.Models = append(policy.Models, geminiProfile)
	policy.ImagePriority = append(policy.ImagePriority, "media-img-2")
	_, err = model.UpdateMediaPolicy(snapshot.ConfigVersion, policy)
	require.NoError(t, err)

	context, recorder := newMediaCatalogContext(t, user.Id, "")
	MediaModels(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := gjson.Parse(recorder.Body.String())
	require.True(t, payload.Get("success").Bool())
	assert.Equal(t, float64(2), payload.Get("data.#").Float())
	assert.Equal(t, "media-img-1", payload.Get("data.0.id").String())
	assert.True(t, payload.Get("data.0.availability.routable").Bool())
	assert.Equal(t, "unknown", payload.Get("data.0.availability.health").String())
	assert.True(t, payload.Get("data.0.operations.text_to_image").Exists())
	assert.True(t, payload.Get("data.0.operations.image_to_image").Exists())
	assert.Equal(t, "media-img-1", payload.Get("defaults.image.model").String())
	assert.Equal(t, "ok", payload.Get("defaults.image.reason_code").String())
	assert.Equal(t, "no_available_model", payload.Get("defaults.video.reason_code").String())
	assert.NotContains(t, recorder.Body.String(), "sk-img-secret")
	assert.NotContains(t, recorder.Body.String(), "sk-gem-secret")
	assert.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
}

func TestMediaModelsCatalogQueryValidation(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	user := seedMediaUser(t, db, 7003, "default")
	for _, tc := range []struct {
		name  string
		query string
	}{
		{"contradictory type", "?type=video&operation=text_to_image"},
		{"unknown type", "?type=audio"},
		{"unknown operation", "?operation=resize"},
		{"repeated type", "?type=image&type=video"},
		{"repeated operation", "?operation=text_to_image&operation=image_to_image"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			context, recorder := newMediaCatalogContext(t, user.Id, tc.query)
			MediaModels(context)
			assert.Equal(t, http.StatusBadRequest, recorder.Code, tc.name)
			assert.Equal(t, "invalid_media_models_query", gjson.Parse(recorder.Body.String()).Get("code").String())
		})
	}
}

func TestMediaModelsCatalogTypeAndOperationFilters(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7004, "default")
	seedMediaPolicy(t, mediaValidPolicy())
	seedMediaChannel(t, db, &model.Channel{Id: 7103, Type: constant.ChannelTypeOpenAI, Name: "img", Key: "k"}, "default", "media-img-1")

	context, recorder := newMediaCatalogContext(t, user.Id, "?type=image")
	MediaModels(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := gjson.Parse(recorder.Body.String())
	assert.Equal(t, float64(1), payload.Get("data.#").Float())
	assert.Equal(t, "not_requested", payload.Get("defaults.video.reason_code").String())
	assert.Equal(t, "media-img-1", payload.Get("defaults.image.model").String())

	context, recorder = newMediaCatalogContext(t, user.Id, "?operation=image_to_image")
	MediaModels(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload = gjson.Parse(recorder.Body.String())
	require.Equal(t, float64(1), payload.Get("data.#").Float())
	assert.False(t, payload.Get("data.0.operations.text_to_image").Exists())
	assert.True(t, payload.Get("data.0.operations.image_to_image").Exists())
	assert.Equal(t, "media-img-1", payload.Get("defaults.image.model").String())
}

func TestMediaModelsCatalogOmitsHeterogeneousChannels(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7005, "default")
	seedMediaPolicy(t, mediaValidPolicy())
	seedMediaChannel(t, db, &model.Channel{Id: 7104, Type: constant.ChannelTypeOpenAI, Name: "img", Key: "k"}, "default", "media-img-1")
	seedMediaChannel(t, db, &model.Channel{Id: 7105, Type: constant.ChannelTypeCodex, Name: "codex", Key: "k"}, "default", "media-img-1")

	context, recorder := newMediaCatalogContext(t, user.Id, "")
	MediaModels(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := gjson.Parse(recorder.Body.String())
	assert.Equal(t, float64(0), payload.Get("data.#").Float())
	assert.Equal(t, "no_available_model", payload.Get("defaults.image.reason_code").String())
}

func TestMediaModelsCatalogDisabledChannelAndPriorityFallback(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7006, "default")
	policy := mediaValidPolicy()
	policy.Models = []model.MediaModelProfile{mediaImageProfile("media-img-1"), mediaImageProfile("media-img-2")}
	policy.ImagePriority = []string{"media-img-2", "media-img-1"}
	policy.VideoPriority = []string{}
	seedMediaPolicy(t, policy)

	channel := &model.Channel{Id: 7106, Type: constant.ChannelTypeOpenAI, Name: "img", Key: "k", Group: "default", Models: "media-img-1", Status: common.ChannelStatusManuallyDisabled}
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "media-img-1", ChannelId: channel.Id, Enabled: true}).Error)
	seedMediaChannel(t, db, &model.Channel{Id: 7107, Type: constant.ChannelTypeOpenAI, Name: "img2", Key: "k"}, "default", "media-img-2")

	context, recorder := newMediaCatalogContext(t, user.Id, "")
	MediaModels(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := gjson.Parse(recorder.Body.String())
	require.Equal(t, float64(1), payload.Get("data.#").Float())
	assert.Equal(t, "media-img-2", payload.Get("data.0.id").String())
	assert.Equal(t, "media-img-2", payload.Get("defaults.image.model").String())

	policy.ImagePriority = []string{"media-img-1"}
	snapshot, err := model.GetMediaPolicy()
	require.NoError(t, err)
	_, err = model.UpdateMediaPolicy(snapshot.ConfigVersion, policy)
	require.NoError(t, err)

	context, recorder = newMediaCatalogContext(t, user.Id, "")
	MediaModels(context)
	payload = gjson.Parse(recorder.Body.String())
	assert.Equal(t, "no_recommended_model", payload.Get("defaults.image.reason_code").String())
	assert.Equal(t, gjson.Null, payload.Get("defaults.image.model").Type)
}

func TestMediaModelsCatalogTokenGroupLimitAndPin(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7007, "default")
	seedMediaPolicy(t, mediaValidPolicy())
	seedMediaChannel(t, db, &model.Channel{Id: 7108, Type: constant.ChannelTypeOpenAI, Name: "img-default", Key: "k"}, "default", "media-img-1")
	seedMediaChannel(t, db, &model.Channel{Id: 7109, Type: constant.ChannelTypeOpenAI, Name: "img-vip", Key: "k"}, "vip", "media-img-1")

	context, recorder := newMediaCatalogContext(t, user.Id, "")
	common.SetContextKey(context, constant.ContextKeyTokenGroup, "vip")
	common.SetContextKey(context, constant.ContextKeyUsingGroup, "vip")
	MediaModels(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, float64(1), gjson.Parse(recorder.Body.String()).Get("data.#").Float())

	context, recorder = newMediaCatalogContext(t, user.Id, "")
	common.SetContextKey(context, constant.ContextKeyTokenGroup, "vip")
	common.SetContextKey(context, constant.ContextKeyUsingGroup, "vip")
	common.SetContextKey(context, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(context, constant.ContextKeyTokenModelLimit, map[string]bool{"other-model": true})
	MediaModels(context)
	assert.Equal(t, float64(0), gjson.Parse(recorder.Body.String()).Get("data.#").Float())

	context, recorder = newMediaCatalogContext(t, user.Id, "")
	common.SetContextKey(context, constant.ContextKeyTokenGroup, "vip")
	common.SetContextKey(context, constant.ContextKeyUsingGroup, "vip")
	common.SetContextKey(context, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(context, constant.ContextKeyTokenModelLimit, map[string]bool{"media-img-1": true})
	constraints := &hostdto.ChannelConstraints{}
	constraints.AddPin(hostdto.ChannelPin{ChannelId: 7108, Source: hostdto.PinSourceToken, Rank: hostdto.PinRankToken})
	common.SetContextKey(context, constant.ContextKeyChannelConstraints, constraints)
	MediaModels(context)
	payload := gjson.Parse(recorder.Body.String())
	assert.Equal(t, float64(0), payload.Get("data.#").Float(), "pin to a channel outside the token group must not leak")

	constraints = &hostdto.ChannelConstraints{}
	constraints.AddPin(hostdto.ChannelPin{ChannelId: 7109, Source: hostdto.PinSourceToken, Rank: hostdto.PinRankToken})
	context, recorder = newMediaCatalogContext(t, user.Id, "")
	common.SetContextKey(context, constant.ContextKeyTokenGroup, "vip")
	common.SetContextKey(context, constant.ContextKeyUsingGroup, "vip")
	common.SetContextKey(context, constant.ContextKeyChannelConstraints, constraints)
	MediaModels(context)
	assert.Equal(t, float64(1), gjson.Parse(recorder.Body.String()).Get("data.#").Float())
}

func TestMediaModelsCatalogAutoGroup(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7008, "default")
	seedMediaPolicy(t, mediaValidPolicy())
	seedMediaChannel(t, db, &model.Channel{Id: 7110, Type: constant.ChannelTypeOpenAI, Name: "img-default", Key: "k"}, "default", "media-img-1")
	seedMediaChannel(t, db, &model.Channel{Id: 7111, Type: constant.ChannelTypeOpenAI, Name: "img-vip", Key: "k"}, "vip", "media-img-1")

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["vip"]`))
	t.Cleanup(func() { require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`)) })

	context, recorder := newMediaCatalogContext(t, user.Id, "")
	common.SetContextKey(context, constant.ContextKeyTokenGroup, "auto")
	common.SetContextKey(context, constant.ContextKeyUsingGroup, "auto")
	MediaModels(context)
	payload := gjson.Parse(recorder.Body.String())
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, float64(1), payload.Get("data.#").Float(), "auto group should resolve to the configured vip list")
}

func TestMediaModelsCatalogProtocolRules(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7009, "default")
	seedMediaPolicy(t, mediaValidPolicy())

	xaiProfile := mediaImageProfile("media-img-1")
	xaiProfile.Operations["text_to_image"] = model.MediaOperation{
		Protocol:   "openai_images",
		Path:       "/v1/images/generations",
		Parameters: map[string]model.MediaParameter{"size": {Type: "string", Enum: []string{"1024x1024"}}},
		Reference:  model.MediaReference{Input: "none", MaxImages: 0},
	}
	snapshot, err := model.GetMediaPolicy()
	require.NoError(t, err)
	policy := mediaValidPolicy()
	policy.Models = []model.MediaModelProfile{xaiProfile}
	policy.VideoPriority = []string{}
	_, err = model.UpdateMediaPolicy(snapshot.ConfigVersion, policy)
	require.NoError(t, err)

	seedMediaChannel(t, db, &model.Channel{Id: 7112, Type: constant.ChannelTypeXai, Name: "xai", Key: "k"}, "default", "media-img-1")

	context, recorder := newMediaCatalogContext(t, user.Id, "")
	MediaModels(context)
	payload := gjson.Parse(recorder.Body.String())
	assert.Equal(t, float64(0), payload.Get("data.#").Float(), "xai text_to_image accepts only n/response_format parameters")

	policy.Models[0].Operations["text_to_image"] = model.MediaOperation{
		Protocol: "openai_images",
		Path:     "/v1/images/generations",
		Parameters: map[string]model.MediaParameter{
			"n":               {Type: "integer", Minimum: intPointer(1), Maximum: intPointer(1)},
			"response_format": {Type: "string", Enum: []string{"b64_json"}},
		},
		Reference: model.MediaReference{Input: "none", MaxImages: 0},
	}
	snapshot, err = model.GetMediaPolicy()
	require.NoError(t, err)
	_, err = model.UpdateMediaPolicy(snapshot.ConfigVersion, policy)
	require.NoError(t, err)

	context, recorder = newMediaCatalogContext(t, user.Id, "")
	MediaModels(context)
	payload = gjson.Parse(recorder.Body.String())
	require.Equal(t, float64(1), payload.Get("data.#").Float())
	assert.True(t, payload.Get("data.0.operations.text_to_image").Exists())
	assert.False(t, payload.Get("data.0.operations.image_to_image").Exists(), "xai does not serve image edits")
}

func TestMediaModelsCatalogGeminiImagenMapping(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7010, "default")
	seedMediaPolicy(t, mediaValidPolicy())
	channel := &model.Channel{Id: 7113, Type: constant.ChannelTypeGemini, Name: "gem", Key: "k"}
	seedMediaChannel(t, db, channel, "default", "media-img-1")
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("model_mapping", `{"media-img-1":"imagen-3.0-generate-002"}`).Error)

	context, recorder := newMediaCatalogContext(t, user.Id, "?type=image")
	MediaModels(context)
	payload := gjson.Parse(recorder.Body.String())
	require.Equal(t, float64(1), payload.Get("data.#").Float())
	assert.True(t, payload.Get("data.0.operations.text_to_image").Exists())
	assert.False(t, payload.Get("data.0.operations.image_to_image").Exists(), "gemini openai_images only serves text_to_image")

	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("model_mapping", `{"media-img-1":"gemini-2.0-flash"}`).Error)
	context, recorder = newMediaCatalogContext(t, user.Id, "?type=image")
	MediaModels(context)
	assert.Equal(t, float64(0), gjson.Parse(recorder.Body.String()).Get("data.#").Float())
}

func TestMediaModelsCatalogAdvancedCustomPath(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7011, "default")
	seedMediaPolicy(t, mediaValidPolicy())
	channel := &model.Channel{Id: 7114, Type: constant.ChannelTypeAdvancedCustom, Name: "custom", Key: "k"}
	channel.SetOtherSettings(relaydto.ChannelOtherSettings{AdvancedCustom: &relaydto.AdvancedCustomConfig{
		Routes: []relaydto.AdvancedCustomRoute{{IncomingPath: "/v1/images/generations", UpstreamPath: "/v1/images/generations", Models: []string{"media-img-1"}}},
	}})
	seedMediaChannel(t, db, channel, "default", "media-img-1")

	context, recorder := newMediaCatalogContext(t, user.Id, "")
	MediaModels(context)
	payload := gjson.Parse(recorder.Body.String())
	require.Equal(t, float64(1), payload.Get("data.#").Float())
	assert.True(t, payload.Get("data.0.operations.text_to_image").Exists())
	assert.False(t, payload.Get("data.0.operations.image_to_image").Exists(), "advanced custom without an edits route must not advertise image_to_image")
}

func TestMediaModelsCatalogBillingGate(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeDisabled(t)
	user := seedMediaUser(t, db, 7012, "default")
	seedMediaPolicy(t, mediaValidPolicy())
	seedMediaChannel(t, db, &model.Channel{Id: 7115, Type: constant.ChannelTypeOpenAI, Name: "img", Key: "k"}, "default", "media-img-1")

	context, recorder := newMediaCatalogContext(t, user.Id, "?type=image")
	MediaModels(context)
	payload := gjson.Parse(recorder.Body.String())
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, float64(0), payload.Get("data.#").Float(), "no billing config and no acceptUnset must suppress the model")
}

func mediaTestVideoPluginSource(key string, models string, channelTypes string) string {
	return fmt.Sprintf(`
export const meta = {
  apiVersion: 1,
  key: %q,
  name: %q,
  version: "1.0.0",
  author: {name: "Test"},
  models: %s,
  channelTypes: %s,
  fetchMode: "per_task",
  protocols: ["openai_video"],
};
export function buildSubmitRequest() { return {url: "https://example.com"}; }
export function parseSubmitResponse() { return {taskId: "one"}; }
export function buildQueryRequest() { return {url: "https://example.com"}; }
export function parseTaskResult() { return {status: "SUCCESS"}; }
export function listArtifacts() { return []; }
export function buildContentRequest() { throw new Error("artifact_not_found"); }
export const protocols = {
  openai_video: {
    decodeRequest: function(ctx) { ctx.requestBody = ctx.body.value; return {kind: "submit"}; },
    render: function() { return {}; },
  },
};
`, key, key, models, channelTypes)
}

func TestMediaModelsCatalogVideoChannelSupport(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7013, "default")
	seedMediaPolicy(t, mediaValidPolicy())
	seedMediaChannel(t, db, &model.Channel{Id: 7116, Type: constant.ChannelTypeOpenAI, Name: "video", Key: "k"}, "default", "media-video-1")

	context, recorder := newMediaCatalogContext(t, user.Id, "?type=video")
	MediaModels(context)
	payload := gjson.Parse(recorder.Body.String())
	require.Equal(t, float64(1), payload.Get("data.#").Float(), "sora plugin owns the OpenAI/Sora legacy video fallback")
	assert.True(t, payload.Get("data.0.operations.text_to_video").Exists())
	assert.Equal(t, "media-video-1", payload.Get("defaults.video.model").String())

	seedMediaChannel(t, db, &model.Channel{Id: 7119, Type: constant.ChannelTypeCodex, Name: "codex-video", Key: "k"}, "default", "media-video-1")
	context, recorder = newMediaCatalogContext(t, user.Id, "?type=video")
	MediaModels(context)
	assert.Equal(t, float64(0), gjson.Parse(recorder.Body.String()).Get("data.#").Float(), "a codex channel on the same model suppresses the whole operation")

	require.NoError(t, db.Model(&model.Ability{}).Where("model = ? AND channel_id = ?", "media-video-1", 7119).Update("enabled", false).Error)
	originalEnabled := jsplugin.DefaultRegistry.Enabled()
	jsplugin.DefaultRegistry.SetEnabled(false)
	t.Cleanup(func() { jsplugin.DefaultRegistry.SetEnabled(originalEnabled) })
	context, recorder = newMediaCatalogContext(t, user.Id, "?type=video")
	MediaModels(context)
	assert.Equal(t, float64(0), gjson.Parse(recorder.Body.String()).Get("data.#").Float(), "disabled plugin system must suppress video operations")
	jsplugin.DefaultRegistry.SetEnabled(originalEnabled)
}

func TestMediaModelsCatalogTaskPluginBinding(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7014, "default")
	seedMediaPolicy(t, mediaValidPolicy())

	plugin, err := jsplugin.DefaultRegistry.Register(
		mediaTestVideoPluginSource("media-task-plugin", `["media-video-1"]`, `[]`),
		jsplugin.Options{},
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = jsplugin.DefaultRegistry.Unregister(plugin.Meta.Key) })

	taskChannel := &model.Channel{Id: 7117, Type: constant.ChannelTypeTaskPlugin, Name: "tp", Key: "k"}
	taskChannel.SetSetting(relaydto.ChannelSettings{TaskPluginKey: "media-task-plugin"})
	seedMediaChannel(t, db, taskChannel, "default", "media-video-1")

	context, recorder := newMediaCatalogContext(t, user.Id, "?type=video")
	MediaModels(context)
	payload := gjson.Parse(recorder.Body.String())
	require.Equal(t, float64(1), payload.Get("data.#").Float())
	assert.True(t, payload.Get("data.0.operations.text_to_video").Exists())

	orphan := &model.Channel{Id: 7118, Type: constant.ChannelTypeTaskPlugin, Name: "tp2", Key: "k"}
	orphan.SetSetting(relaydto.ChannelSettings{TaskPluginKey: "nonexistent-plugin"})
	seedMediaChannel(t, db, orphan, "default", "media-video-2")
	policy := mediaValidPolicy()
	policy.Models = append(policy.Models, mediaVideoProfile("media-video-2"))
	policy.VideoPriority = append(policy.VideoPriority, "media-video-2")
	snapshot, err := model.GetMediaPolicy()
	require.NoError(t, err)
	_, err = model.UpdateMediaPolicy(snapshot.ConfigVersion, policy)
	require.NoError(t, err)

	context, recorder = newMediaCatalogContext(t, user.Id, "?type=video")
	MediaModels(context)
	payload = gjson.Parse(recorder.Body.String())
	require.Equal(t, float64(1), payload.Get("data.#").Float())
	assert.Equal(t, "media-video-1", payload.Get("data.0.id").String(), "unbound task plugin channel must not leak its model")
}

func TestMediaModelsSelectionHintValidation(t *testing.T) {
	valid := mediaValidPolicy()
	assert.NoError(t, model.ValidateMediaPolicy(valid))

	hinted := mediaValidPolicy()
	hinted.Models[0].SelectionHint = ""
	assert.NoError(t, model.ValidateMediaPolicy(hinted))
	hinted.Models[0].SelectionHint = strings.Repeat("好", 2000)
	assert.NoError(t, model.ValidateMediaPolicy(hinted))
	hinted.Models[0].SelectionHint = strings.Repeat("好", 2001)
	assert.Error(t, model.ValidateMediaPolicy(hinted))

	encoded, err := common.Marshal(mediaValidPolicy())
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "selection_hint")

	hinted.Models[0].SelectionHint = "Prefer reference consistency."
	encoded, err = common.Marshal(hinted)
	require.NoError(t, err)
	assert.NoError(t, model.ValidateMediaPolicyRaw(encoded))

	var document map[string]any
	require.NoError(t, common.Unmarshal(encoded, &document))
	models := document["models"].([]any)
	profile := models[0].(map[string]any)
	for _, bad := range []any{nil, float64(123), map[string]any{}, []any{}} {
		profile["selection_hint"] = bad
		raw, err := common.Marshal(document)
		require.NoError(t, err)
		assert.Error(t, model.ValidateMediaPolicyRaw(raw), "selection_hint %v must be rejected", bad)
	}
}

func TestMediaModelsCatalogSelectionHints(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7015, "default")

	profileB := mediaImageProfile("media-img-b")
	profileB.SelectionHint = "Prefer for consistent portraits."
	profileC := mediaImageProfile("media-img-c")
	profileC.SelectionHint = "Use when the prompt needs multilingual text."
	policy := mediaValidPolicy()
	policy.Models = []model.MediaModelProfile{
		mediaImageProfile("media-img-a"),
		profileB,
		profileC,
	}
	policy.ImagePriority = []string{"media-img-a", "media-img-b"}
	policy.VideoPriority = []string{}
	seedMediaPolicy(t, policy)
	seedMediaChannel(t, db, &model.Channel{Id: 7120, Type: constant.ChannelTypeOpenAI, Name: "img", Key: "k"}, "default", "media-img-a", "media-img-b", "media-img-c")

	context, recorder := newMediaCatalogContext(t, user.Id, "?type=image")
	MediaModels(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := gjson.Parse(recorder.Body.String())
	require.Equal(t, float64(3), payload.Get("data.#").Float())
	assert.Equal(t, "media-img-a", payload.Get("data.0.id").String())
	assert.Equal(t, "media-img-b", payload.Get("data.1.id").String())
	assert.Equal(t, "media-img-c", payload.Get("data.2.id").String())
	assert.False(t, payload.Get("data.0.selection_hint").Exists())
	assert.Equal(t, "Prefer for consistent portraits.", payload.Get("data.1.selection_hint").String())
	assert.Equal(t, "Use when the prompt needs multilingual text.", payload.Get("data.2.selection_hint").String())
	assert.Equal(t, "media-img-a", payload.Get("defaults.image.model").String())

	context, recorder = newMediaCatalogContext(t, user.Id, "?type=image")
	common.SetContextKey(context, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(context, constant.ContextKeyTokenModelLimit, map[string]bool{"media-img-c": true})
	MediaModels(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload = gjson.Parse(recorder.Body.String())
	require.Equal(t, float64(1), payload.Get("data.#").Float())
	assert.Equal(t, "media-img-c", payload.Get("data.0.id").String())
	assert.Equal(t, "Use when the prompt needs multilingual text.", payload.Get("data.0.selection_hint").String())
	assert.Equal(t, gjson.Null, payload.Get("defaults.image.model").Type)
	assert.Equal(t, "no_recommended_model", payload.Get("defaults.image.reason_code").String())
}

func seedMediaOrdinaryModel(t *testing.T, db *gorm.DB, name string, endpoints string) {
	t.Helper()
	require.NoError(t, db.Create(&model.Model{
		ModelName: name,
		NameRule:  model.NameRuleExact,
		Endpoints: endpoints,
	}).Error)
}

func TestMediaModelsCatalogAutoDiscovery(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7060, "default")
	seedMediaOrdinaryModel(t, db, "opaque-image", `{"image-generation":"/v1/images/generations"}`)
	seedMediaChannel(t, db, &model.Channel{Id: 7201, Type: constant.ChannelTypeOpenAI, Name: "img", Key: "k"}, "default", "opaque-image")

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/option/media_models", GetMediaModelsPolicy)

	get := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/option/media_models", nil))
		return recorder
	}

	recorder := get()
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := gjson.Parse(recorder.Body.String())
	require.True(t, payload.Get("success").Bool())
	assert.Equal(t, "opaque-image", payload.Get("data.policy.models.0.id").String())
	assert.Equal(t, "image", payload.Get("data.policy.models.0.type").String())
	assert.True(t, payload.Get("data.policy.models.0.operations.text_to_image").Exists())
	assert.True(t, payload.Get("data.policy.models.0.operations.image_to_image").Exists())

	context, pubRecorder := newMediaCatalogContext(t, user.Id, "")
	MediaModels(context)
	require.Equal(t, http.StatusOK, pubRecorder.Code)
	pub := gjson.Parse(pubRecorder.Body.String())
	assert.Equal(t, "opaque-image", pub.Get("data.0.id").String())
	assert.Equal(t, "no_recommended_model", pub.Get("defaults.image.reason_code").String())

	seedMediaOrdinaryModel(t, db, "new-image", `{"image-generation":"/v1/images/generations"}`)
	recorder = get()
	require.Equal(t, http.StatusOK, recorder.Code)
	payload = gjson.Parse(recorder.Body.String())
	assert.Equal(t, float64(2), payload.Get("data.policy.models.#").Float())
}

func TestMediaModelsCatalogAcceptsEndpointTypeArrays(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	seedMediaOrdinaryModel(t, db, "gpt-image-2", `["image-generation","openai"]`)
	seedMediaChannel(t, db, &model.Channel{Id: 7301, Type: constant.ChannelTypeOpenAI, Name: "img", Key: "k"}, "default", "gpt-image-2")
	seedMediaOrdinaryModel(t, db, "claude-sonnet-4-6", `["anthropic","openai"]`)
	seedMediaOrdinaryModel(t, db, "broken-model", "not-json")

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/option/media_models", GetMediaModelsPolicy)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/option/media_models", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	payload := gjson.Parse(recorder.Body.String())
	require.True(t, payload.Get("success").Bool())
	ids := make([]string, 0)
	for _, entry := range payload.Get("data.policy.models.#.id").Array() {
		ids = append(ids, entry.String())
	}
	assert.Equal(t, []string{"gpt-image-2"}, ids)
	assert.Equal(t, "image", payload.Get("data.policy.models.0.type").String())
	assert.True(t, payload.Get("data.policy.models.0.operations.text_to_image").Exists())
	assert.True(t, payload.Get("data.policy.models.0.operations.image_to_image").Exists())
}

func TestMediaModelsCatalogPresetRecognition(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7061, "default")
	seedMediaPolicy(t, model.MediaPolicy{
		Version:       1,
		Models:        []model.MediaModelProfile{mediaImageProfile("image-model-id")},
		ImagePriority: []string{"image-model-id"},
		VideoPriority: []string{},
	})
	seedMediaOrdinaryModel(t, db, "gemini-chat-1", "")
	seedMediaOrdinaryModel(t, db, "gemini-3-pro-image", "")
	seedMediaOrdinaryModel(t, db, "gemini-2.5-flash-image", "")
	seedMediaChannel(t, db, &model.Channel{Id: 7210, Type: constant.ChannelTypeGemini, Name: "gem", Key: "k"}, "default", "gemini-chat-1", "gemini-3-pro-image", "gemini-2.5-flash-image")

	context, recorder := newMediaCatalogContext(t, user.Id, "")
	MediaModels(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.NotContains(t, body, "image-model-id")
	assert.NotContains(t, body, "gemini-chat-1")
	payload := gjson.Parse(body)
	require.True(t, payload.Get("success").Bool())
	assert.Equal(t, float64(2), payload.Get("data.#").Float())
	byID := map[string]gjson.Result{}
	for _, item := range payload.Get("data").Array() {
		byID[item.Get("id").String()] = item
	}
	pro := byID["gemini-3-pro-image"]
	require.True(t, pro.Exists())
	assert.Equal(t, "image", pro.Get("type").String())
	assert.Equal(t, "gemini_generate_content", pro.Get("operations.text_to_image.protocol").String())
	assert.Contains(t, pro.Get("operations.text_to_image.parameters.resolution.enum").Raw, `"4K"`)
	assert.Contains(t, pro.Get("operations.text_to_image.parameters.aspect_ratio.enum").Raw, "16:9")
	assert.Equal(t, "inline", pro.Get("operations.image_to_image.reference.input").String())
	assert.Equal(t, float64(1), pro.Get("operations.image_to_image.reference.max_images").Float())
	flash := byID["gemini-2.5-flash-image"]
	require.True(t, flash.Exists())
	assert.True(t, flash.Get("operations.text_to_image.parameters.aspect_ratio").Exists())
	assert.False(t, flash.Get("operations.text_to_image.parameters.resolution").Exists())
}

func TestMediaModelsPolicyUpdateRegeneratesProfiles(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	seedMediaOrdinaryModel(t, db, "opaque-img", `{"image-generation":"/v1/images/generations"}`)
	seedMediaOrdinaryModel(t, db, "chat-model", "")
	seedMediaChannel(t, db, &model.Channel{Id: 7220, Type: constant.ChannelTypeOpenAI, Name: "img", Key: "k"}, "default", "opaque-img", "chat-model")

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/option/media_models", GetMediaModelsPolicy)
	engine.PUT("/api/option/media_models", UpdateMediaModelsPolicy)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/option/media_models", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	version := gjson.Parse(recorder.Body.String()).Get("data.config_version").String()

	put := func(body string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPut, "/api/option/media_models", strings.NewReader(body)))
		return recorder
	}

	malformed := `{"version":1,"models":[{"id":"opaque-img","type":"image","operations":{"text_to_image":{"protocol":"openai_images","path":"/v1/images/edits","parameters":{"n":{"type":"integer","minimum":1,"maximum":9}},"reference":{"input":"url","max_images":7}}},"selection_hint":"prefer this"}],"image_priority":["opaque-img"],"video_priority":[]}`
	recorder = put(fmt.Sprintf(`{"expected_version":%q,"policy":%s}`, version, malformed))
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	incoming := `{"version":1,"models":[{"id":"opaque-img","type":"image","operations":{"text_to_image":{"protocol":"openai_images","path":"/v1/images/generations","parameters":{"n":{"type":"integer","minimum":1,"maximum":9}},"reference":{"input":"none","max_images":0}}},"selection_hint":"prefer this"}],"image_priority":["opaque-img"],"video_priority":[]}`
	recorder = put(fmt.Sprintf(`{"expected_version":%q,"policy":%s}`, version, incoming))
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := gjson.Parse(recorder.Body.String())
	profile := payload.Get("data.policy.models.0")
	assert.Equal(t, "opaque-img", profile.Get("id").String())
	assert.Equal(t, "prefer this", profile.Get("selection_hint").String())
	assert.Equal(t, "/v1/images/generations", profile.Get("operations.text_to_image.path").String())
	assert.Equal(t, "none", profile.Get("operations.text_to_image.reference.input").String())
	assert.False(t, profile.Get("operations.text_to_image.parameters.n").Exists())
	assert.Equal(t, "opaque-img", payload.Get("data.policy.image_priority.0").String())

	savedVersion := payload.Get("data.config_version").String()
	bad := `{"version":1,"models":[{"id":"ghost-model","type":"image","operations":{"text_to_image":{"protocol":"openai_images","path":"/v1/images/generations","parameters":{},"reference":{"input":"none","max_images":0}}}}],"image_priority":[],"video_priority":[]}`
	recorder = put(fmt.Sprintf(`{"expected_version":%q,"policy":%s}`, savedVersion, bad))
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	promote := strings.Replace(bad, "ghost-model", "chat-model", 1)
	recorder = put(fmt.Sprintf(`{"expected_version":%q,"policy":%s}`, savedVersion, promote))
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestMediaModelsCatalogUnavailableWhenSourcesFail(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	require.NoError(t, db.Migrator().DropTable(&model.Channel{}))
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/api/option/media_models", GetMediaModelsPolicy)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/option/media_models", nil))
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.Equal(t, "media_catalog_unavailable", gjson.Parse(recorder.Body.String()).Get("code").String())
}

func TestMediaPresetProfilesAreValid(t *testing.T) {
	cases := []struct {
		id        string
		endpoints map[string]bool
		wantType  string
	}{
		{"opaque-img", map[string]bool{"image-generation": true}, "image"},
		{"opaque-vid", map[string]bool{"openai-video": true}, "video"},
		{"gpt-image-1", map[string]bool{"image-generation": true}, "image"},
		{"gpt-image-1-mini", map[string]bool{"image-generation": true}, "image"},
		{"gpt-image-1.5", map[string]bool{"image-generation": true}, "image"},
		{"gpt-image-2", map[string]bool{"image-generation": true}, "image"},
		{"gemini-2.5-flash-image", map[string]bool{"gemini": true}, "image"},
		{"gemini-3-pro-image", map[string]bool{"gemini": true}, "image"},
		{"gemini-3-pro-image-preview", map[string]bool{"gemini": true}, "image"},
		{"gemini-3.1-flash-image", map[string]bool{"gemini": true}, "image"},
		{"gemini-3.1-flash-image-preview", map[string]bool{"gemini": true}, "image"},
	}
	profiles := make([]model.MediaModelProfile, 0, len(cases))
	var gpt2 model.MediaModelProfile
	for _, tc := range cases {
		profile, ok := mediaPresetProfile(tc.id, tc.endpoints, nil)
		require.True(t, ok, tc.id)
		assert.Equal(t, tc.wantType, profile.Type, tc.id)
		profiles = append(profiles, profile)
		if tc.id == "gpt-image-2" {
			gpt2 = profile
		}
	}
	require.NoError(t, model.ValidateMediaPolicy(model.MediaPolicy{
		Version:       1,
		Models:        profiles,
		ImagePriority: []string{},
		VideoPriority: []string{},
	}))
	assert.Equal(t, "/v1/images/edits", gpt2.Operations["image_to_image"].Path)
	assert.Equal(t, "/v1/images/generations", gpt2.Operations["text_to_image"].Path)
	opaque := profiles[0]
	assert.Equal(t, "/v1/images/edits", opaque.Operations["image_to_image"].Path, "any image model defaults to image edits")
	_, ok := mediaPresetProfile("gemini-unknown-image", map[string]bool{"gemini": true}, nil)
	assert.False(t, ok)
	_, ok = mediaPresetProfile("chat-model", map[string]bool{"openai": true}, nil)
	assert.False(t, ok)
}

func TestMediaModelsCatalogAutoDiscoveryDefaultsAndTokens(t *testing.T) {
	db := setupMediaModelsTestDB(t)
	withSelfUseModeEnabled(t)
	user := seedMediaUser(t, db, 7062, "default")
	for _, name := range []string{"auto-img-a", "auto-img-b", "auto-img-c", "auto-img-d", "auto-img-e"} {
		seedMediaOrdinaryModel(t, db, name, `{"image-generation":"/v1/images/generations"}`)
	}
	seedMediaChannel(t, db, &model.Channel{Id: 7230, Type: constant.ChannelTypeOpenAI, Name: "img", Key: "k"}, "default", "auto-img-a", "auto-img-b", "auto-img-c", "auto-img-d", "auto-img-e")
	snapshot := seedMediaPolicy(t, model.MediaPolicy{
		Version: 1,
		Models: []model.MediaModelProfile{
			mediaImageProfile("auto-img-a"),
			mediaImageProfile("auto-img-b"),
		},
		ImagePriority: []string{"auto-img-a", "auto-img-b"},
		VideoPriority: []string{},
	})

	context, recorder := newMediaCatalogContext(t, user.Id, "")
	common.SetContextKey(context, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(context, constant.ContextKeyTokenModelLimit, map[string]bool{"auto-img-a": true, "auto-img-c": true})
	MediaModels(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload := gjson.Parse(recorder.Body.String())
	assert.Equal(t, float64(2), payload.Get("data.#").Float())
	assert.Equal(t, "auto-img-a", payload.Get("data.0.id").String())
	assert.Equal(t, "auto-img-c", payload.Get("data.1.id").String())
	assert.Equal(t, "auto-img-a", payload.Get("defaults.image.model").String())
	assert.Equal(t, "ok", payload.Get("defaults.image.reason_code").String())

	context, recorder = newMediaCatalogContext(t, user.Id, "")
	common.SetContextKey(context, constant.ContextKeyTokenModelLimitEnabled, true)
	common.SetContextKey(context, constant.ContextKeyTokenModelLimit, map[string]bool{"auto-img-c": true, "auto-img-d": true})
	MediaModels(context)
	require.Equal(t, http.StatusOK, recorder.Code)
	payload = gjson.Parse(recorder.Body.String())
	assert.Equal(t, float64(2), payload.Get("data.#").Float())
	assert.Equal(t, "auto-img-c", payload.Get("data.0.id").String())
	assert.Equal(t, "auto-img-d", payload.Get("data.1.id").String())
	assert.Equal(t, gjson.Null, payload.Get("defaults.image.model").Type)
	assert.Equal(t, "no_recommended_model", payload.Get("defaults.image.reason_code").String())

	loaded, err := model.GetMediaPolicy()
	require.NoError(t, err)
	assert.Equal(t, snapshot.ConfigVersion, loaded.ConfigVersion)
	assert.Len(t, loaded.Policy.Models, 2)
}
