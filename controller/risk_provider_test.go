package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type riskProviderAPIResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type riskProviderControllerRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn riskProviderControllerRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type riskProviderTestCall struct {
	Method  string
	Target  string
	Body    any
	Id      int
	Handler gin.HandlerFunc
}

func setupRiskProviderControllerTest(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	originalDB := model.DB
	originalSecret := common.CryptoSecret
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	common.CryptoSecret = "risk-provider-controller-key"
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(
		&model.User{}, &model.Channel{}, &model.Token{}, &model.RiskProvider{},
		&model.RiskPolicy{}, &model.RiskRecord{}, &model.RiskRecordGovernance{},
	))
	service.InitHttpClient()
	t.Cleanup(func() {
		model.DB = originalDB
		common.CryptoSecret = originalSecret
		common.RedisEnabled = originalRedisEnabled
		common.SetDatabaseTypes(originalMainType, originalLogType)
	})
	return db
}

func TestRiskProviderAPIWorkflowManagesPlatformInternalToken(t *testing.T) {
	db := setupRiskProviderControllerTest(t)
	root := &model.User{
		Username: "root", Password: "password", Role: common.RoleRootUser,
		Status: common.UserStatusEnabled, Group: "default", Quota: 1000000,
	}
	require.NoError(t, db.Create(root).Error)
	require.NoError(t, db.Create(&model.User{
		Username: "other-root-admin", Password: "password", Role: common.RoleRootUser,
		Status: common.UserStatusEnabled, Group: "default", Quota: 1000000, AffCode: "other-root-admin",
	}).Error)
	channel := &model.Channel{
		Name: "Internal review", Key: "upstream-key", Status: common.ChannelStatusEnabled,
		Models: "guard-model,guard-model-v2", Group: "default",
	}
	require.NoError(t, db.Create(channel).Error)

	create := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodPost, Target: "/api/risk/providers",
		Body: map[string]any{
			"name": "Platform review", "provider_type": "platform_internal",
			"channel_id": channel.Id, "model": "guard-model",
		},
		Handler: CreateRiskProvider,
	})
	require.True(t, create.Success, create.Message)
	var created RiskProviderResponse
	require.NoError(t, common.Unmarshal(create.Data, &created))
	assert.Equal(t, model.RiskProviderPlatformInternal, created.ProviderType)
	assert.Equal(t, channel.Id, created.ChannelID)
	assert.True(t, created.SystemManaged)
	assert.False(t, created.HasCredential)
	assert.Empty(t, created.AccountID)
	assert.NotContains(t, string(create.Data), "token")

	var stored model.RiskProvider
	require.NoError(t, db.First(&stored, created.Id).Error)
	require.Positive(t, stored.InternalTokenID)
	var internalToken model.Token
	require.NoError(t, db.First(&internalToken, stored.InternalTokenID).Error)
	assert.Equal(t, root.Id, internalToken.UserId)
	assert.True(t, internalToken.SystemManaged)
	assert.True(t, internalToken.UnlimitedQuota)
	assert.True(t, internalToken.ModelLimitsEnabled)
	assert.Equal(t, "guard-model", internalToken.ModelLimits)
	assert.Contains(t, internalToken.Name, "AI 风控内部审核")
	require.NotNil(t, internalToken.AllowIps)
	assert.Equal(t, "127.0.0.1/32\n::1/128", *internalToken.AllowIps)
	visibleTokens, err := model.GetAllUserTokens(root.Id, 0, 100)
	require.NoError(t, err)
	assert.Empty(t, visibleTokens)
	loopback := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/v1/chat/completions", request.URL.Path)
		assert.True(t, strings.HasSuffix(request.Header.Get("Authorization"), "-"+strconv.Itoa(channel.Id)))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"verdict\":\"safe\",\"categories\":[]}"}}],"usage":{"prompt_tokens":5,"completion_tokens":3,"total_tokens":8}}`))
	}))
	t.Cleanup(loopback.Close)
	loopbackURL, err := url.Parse(loopback.URL)
	require.NoError(t, err)
	t.Setenv("PORT", loopbackURL.Port())

	validated := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodPost, Target: "/api/risk/providers/1/validate", Id: created.Id,
		Body: map[string]string{"text": "platform internal validation"}, Handler: ValidateRiskProvider,
	})
	require.True(t, validated.Success, validated.Message)
	var validationRecord model.RiskRecord
	require.NoError(t, db.Take(&validationRecord).Error)
	assert.Equal(t, channel.Id, validationRecord.ChannelID)
	assert.Equal(t, model.RiskRecordResultSafe, validationRecord.Result)
	assert.Equal(t, 8, validationRecord.TotalTokens)

	update := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodPut, Target: "/api/risk/providers/1", Id: created.Id,
		Body: map[string]any{
			"name": "Platform review", "provider_type": "platform_internal",
			"channel_id": channel.Id, "model": "guard-model-v2",
			"timeout_ms": 900, "failure_threshold": 6, "cooldown_seconds": 31,
		},
		Handler: UpdateRiskProvider,
	})
	require.True(t, update.Success, update.Message)
	require.NoError(t, db.First(&internalToken, stored.InternalTokenID).Error)
	assert.Equal(t, "guard-model-v2", internalToken.ModelLimits)
	assert.Equal(t, common.TokenStatusEnabled, internalToken.Status)

	deleted := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodDelete, Target: "/api/risk/providers/1", Id: created.Id,
		Handler: DeleteRiskProvider,
	})
	require.True(t, deleted.Success, deleted.Message)
	require.NoError(t, db.First(&internalToken, stored.InternalTokenID).Error)
	assert.Equal(t, common.TokenStatusDisabled, internalToken.Status)
}

func TestValidateRiskProvider_mapsDeadlineExceededToActionableMessage(t *testing.T) {
	db := setupRiskProviderControllerTest(t)
	ciphertext, err := common.EncryptCredential("cf-token")
	require.NoError(t, err)
	provider := &model.RiskProvider{
		Name: "Cloudflare", ProviderType: model.RiskProviderCloudflare,
		AccountID: "0123456789abcdef0123456789abcdef", Model: "@cf/meta/llama-guard-3-8b",
		CredentialEncrypted: ciphertext, TimeoutMs: 800,
	}
	require.NoError(t, model.CreateRiskProvider(provider))
	client := service.GetHttpClient()
	require.NotNil(t, client)
	originalTransport := client.Transport
	client.Transport = riskProviderControllerRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, context.DeadlineExceeded
	})
	t.Cleanup(func() { client.Transport = originalTransport })

	response := callRiskProviderHandler(t, riskProviderTestCall{
		Method: http.MethodPost, Target: "/api/risk/providers/1/validate", Id: provider.Id, Handler: ValidateRiskProvider,
	})

	assert.False(t, response.Success)
	assert.Equal(t, "Risk provider connection timed out (current timeout: 800 ms). Increase the timeout and try again.", response.Message)
	assert.NotContains(t, response.Message, "api.cloudflare.com")
	assert.NotContains(t, response.Message, provider.AccountID)
	assert.NotContains(t, response.Message, "context deadline exceeded")
	var record model.RiskRecord
	require.NoError(t, db.Take(&record).Error)
	assert.Equal(t, provider.Id, record.ProviderID)
	assert.Equal(t, provider.Name, record.ProviderName)
	assert.Equal(t, provider.ProviderType, record.ProviderType)
	assert.Equal(t, model.RiskRecordResultError, record.Result)
	assert.Equal(t, model.RiskRecordSourceProvider, record.Source)
	assert.True(t, record.ProviderCalled)
	assert.Equal(t, "timeout", record.ErrorCode)
	assert.Equal(t, "Cloudflare request timed out", record.ErrorDetail)
	assert.Equal(t, 0, record.ChannelID)
	assert.Equal(t, 1, record.UserID)
}

func callRiskProviderHandler(t *testing.T, call riskProviderTestCall) riskProviderAPIResponse {
	t.Helper()
	var payload []byte
	if call.Body != nil {
		var err error
		payload, err = common.Marshal(call.Body)
		require.NoError(t, err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(call.Method, call.Target, bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	if call.Id > 0 {
		ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(call.Id)}}
	}
	call.Handler(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response riskProviderAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response
}
