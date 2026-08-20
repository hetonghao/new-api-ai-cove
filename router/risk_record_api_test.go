package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type riskRecordAPIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		Page     int                 `json:"page"`
		PageSize int                 `json:"page_size"`
		Total    int                 `json:"total"`
		Items    []*model.RiskRecord `json:"items"`
	} `json:"data"`
}

func setupRiskRecordRouterTest(t *testing.T, role int) (*httptest.Server, *http.Client, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalLogDB := model.LOG_DB
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&model.RiskRecord{},
		&model.SevereRiskRecord{},
		&model.RiskRecordGovernance{},
		&model.Channel{},
		&model.User{},
		&model.Token{},
	))
	accessToken := "risk-record-route-test-" + strings.ReplaceAll(t.Name(), "/", "-")
	user := &model.User{
		Username: "root", Password: "password", Role: role, Status: common.UserStatusEnabled,
		Group: "default", AuthVersion: 1, AffCode: "risk-record-route-test",
	}
	user.SetAccessToken(accessToken)
	require.NoError(t, db.Create(user).Error)

	engine := gin.New()
	registerRiskPolicyRoutes(engine.Group("/api"))
	server := httptest.NewServer(engine)
	client := server.Client()
	t.Cleanup(func() {
		server.Close()
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
		common.RedisEnabled = originalRedisEnabled
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return server, client, accessToken
}

func TestSevereRiskRecordAPI_listsIndependentRecords(t *testing.T) {
	// Given
	server, client, accessToken := setupRiskRecordRouterTest(t, common.RoleRootUser)
	require.NoError(t, model.DB.Create(&model.SevereRiskRecord{
		RequestID: "severe-route-request", ChannelID: 37, ChannelName: "channel", UserID: 56,
		Username: "user@example.com", TokenID: 8, TokenName: "token", Model: "gpt-5.6-sol", Path: "/v1/responses",
		ErrorCode: "invalid_prompt", ErrorDetail: "Invalid prompt", ContextHash: "hash", ContextEncrypted: "cipher",
		ChannelScope: model.SevereRiskChannelScopeAll, UserActionStatus: model.SevereRiskActionSuccess,
		ChannelActionStatus: model.SevereRiskActionSuccess, TriggeredAt: time.Now().UTC(),
	}).Error)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/risk/severe-records?p=1&page_size=20", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+accessToken)

	// When
	response, err := client.Do(request)

	// Then
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var payload struct {
		Success bool `json:"success"`
		Data    struct {
			Total int                      `json:"total"`
			Items []model.SevereRiskRecord `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.DecodeJson(response.Body, &payload))
	require.True(t, payload.Success)
	require.Equal(t, 1, payload.Data.Total)
	require.Equal(t, "severe-route-request", payload.Data.Items[0].RequestID)
}

func TestRiskRecordAPI_returnsRootOnlyPaginatedMetadata(t *testing.T) {
	// Given
	server, client, accessToken := setupRiskRecordRouterTest(t, common.RoleRootUser)
	baseTime := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)
	for index, requestID := range []string{"req-old", "req-new"} {
		chunks := []model.RiskRecordChunk{}
		if index == 1 {
			chunks = []model.RiskRecordChunk{{
				Index: 0, Result: model.RiskRecordResultSafe, Categories: []string{"clean"}, LatencyMS: 41,
				PromptTokens: 11, CompletionTokens: 2, TotalTokens: 13, Neurons: 7,
			}}
		}
		require.NoError(t, model.RecordRiskObservation(context.Background(), model.RiskRecordInput{
			RequestID: requestID, ChannelID: 12, UserID: 34, RuleIDs: []int{5}, ProviderID: 21,
			ProviderName: "Cloudflare", Result: model.RiskRecordResultSafe, Categories: []string{},
			LatencyMS: 93, PromptTokens: 11, CompletionTokens: 2, TotalTokens: 13, Neurons: 7,
			Chunks:         chunks,
			ProviderCalled: index == 1,
			ObservedAt:     baseTime.Add(time.Duration(index) * time.Minute),
		}))
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/risk/records?p=1&page_size=1", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+accessToken)

	// When
	response, err := client.Do(request)

	// Then
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var payload riskRecordAPIResponse
	require.NoError(t, common.DecodeJson(response.Body, &payload))
	require.True(t, payload.Success, payload.Message)
	assert.Equal(t, 1, payload.Data.Page)
	assert.Equal(t, 1, payload.Data.PageSize)
	assert.Equal(t, 2, payload.Data.Total)
	require.Len(t, payload.Data.Items, 1)
	record := payload.Data.Items[0]
	assert.Equal(t, "req-new", record.RequestID)
	assert.Equal(t, 12, record.ChannelID)
	assert.Equal(t, 34, record.UserID)
	assert.Equal(t, []int{5}, record.RuleIDs)
	assert.Equal(t, 21, record.ProviderID)
	assert.Equal(t, "Cloudflare", record.ProviderName)
	assert.Equal(t, model.RiskRecordResultSafe, record.Result)
	assert.Empty(t, record.Categories)
	assert.EqualValues(t, 93, record.LatencyMS)
	assert.Equal(t, 11, record.PromptTokens)
	assert.Equal(t, 2, record.CompletionTokens)
	assert.Equal(t, 13, record.TotalTokens)
	assert.EqualValues(t, 7, record.Neurons)
	assert.Equal(t, []model.RiskRecordChunk{{
		Index: 0, Result: model.RiskRecordResultSafe, Categories: []string{"clean"}, LatencyMS: 41,
		PromptTokens: 11, CompletionTokens: 2, TotalTokens: 13, Neurons: 7,
	}}, record.Chunks)
	assert.Empty(t, record.ErrorCode)
	assert.True(t, record.ProviderCalled)
	assert.Equal(t, baseTime.Add(time.Minute), record.ObservedAt)
}

func TestRiskStatisticsAPI_returnsAggregatedData(t *testing.T) {
	// Given
	server, client, accessToken := setupRiskRecordRouterTest(t, common.RoleRootUser)
	user := &model.User{Username: "statistics-user", Password: "password", AffCode: "statistics-user"}
	channel := &model.Channel{Name: "Statistics channel", Key: "statistics-channel"}
	require.NoError(t, model.DB.Create(user).Error)
	require.NoError(t, model.DB.Create(channel).Error)
	baseTime := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)
	require.NoError(t, model.RecordRiskObservation(context.Background(), model.RiskRecordInput{
		RequestID: "statistics-provider", ChannelID: channel.Id, UserID: user.Id,
		ProviderID: 21, ProviderName: "Cloudflare", ProviderType: model.RiskProviderCloudflare,
		Result: model.RiskRecordResultSafe, Source: model.RiskRecordSourceProvider,
		ProviderCalled: true, LatencyMS: 100, ObservedAt: baseTime,
	}))
	require.NoError(t, model.RecordRiskObservation(context.Background(), model.RiskRecordInput{
		RequestID: "statistics-local", ChannelID: channel.Id, UserID: user.Id,
		Result: model.RiskRecordResultNotReviewed, Source: model.RiskRecordSourceLocal,
		LatencyMS: 20, ObservedAt: baseTime.Add(time.Hour),
	}))
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		fmt.Sprintf("%s/api/risk/statistics?start_timestamp=%d&end_timestamp=%d&granularity=hour", server.URL, baseTime.Unix(), baseTime.Add(2*time.Hour).Unix()), nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+accessToken)

	// When
	response, err := client.Do(request)

	// Then
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var payload struct {
		Success bool                 `json:"success"`
		Message string               `json:"message"`
		Data    model.RiskStatistics `json:"data"`
	}
	require.NoError(t, common.DecodeJson(response.Body, &payload))
	require.True(t, payload.Success, payload.Message)
	assert.Equal(t, int64(2), payload.Data.Summary.Records)
	assert.Equal(t, int64(1), payload.Data.Summary.ProviderCalls)
	assert.Equal(t, int64(100), payload.Data.Summary.P95LatencyMS)
	assert.Len(t, payload.Data.SourceTrend, 3)
}

func TestRiskRecordAPI_rejectsNonRootAdmin(t *testing.T) {
	// Given
	server, client, accessToken := setupRiskRecordRouterTest(t, common.RoleAdminUser)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/risk/records", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+accessToken)

	// When
	response, err := client.Do(request)

	// Then
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusForbidden, response.StatusCode)
}

func TestRiskRecordAPI_filtersGovernedRecords(t *testing.T) {
	// Given
	server, client, accessToken := setupRiskRecordRouterTest(t, common.RoleRootUser)
	require.NoError(t, model.DB.Create(&model.User{Id: 34, Username: "alice", Password: "password", AffCode: "alice"}).Error)
	require.NoError(t, model.DB.Create(&model.User{Id: 35, Username: "bob", Password: "password", AffCode: "bob"}).Error)
	observedAt := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)
	for _, input := range []model.RiskRecordInput{
		{
			RequestID: "req-match", ChannelID: 12, UserID: 34,
			Result: model.RiskRecordResultUnsafe, Source: model.RiskRecordSourceInflight,
			ObservedAt: observedAt,
		},
		{
			RequestID: "req-other-user", ChannelID: 12, UserID: 35,
			Result: model.RiskRecordResultUnsafe, Source: model.RiskRecordSourceInflight,
			ObservedAt: observedAt,
		},
	} {
		require.NoError(t, model.RecordRiskObservation(context.Background(), input))
	}
	url := fmt.Sprintf(
		"%s/api/risk/records?p=1&page_size=20&start_timestamp=%d&end_timestamp=%d&channel_id=12&username=alice&result=unsafe&source=inflight",
		server.URL, observedAt.Unix(), observedAt.Unix(),
	)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+accessToken)

	// When
	response, err := client.Do(request)

	// Then
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var payload riskRecordAPIResponse
	require.NoError(t, common.DecodeJson(response.Body, &payload))
	require.True(t, payload.Success, payload.Message)
	assert.Equal(t, 1, payload.Data.Total)
	require.Len(t, payload.Data.Items, 1)
	assert.Equal(t, "req-match", payload.Data.Items[0].RequestID)
	assert.Zero(t, payload.Data.Items[0].ProviderID)
	assert.Empty(t, payload.Data.Items[0].ProviderName)
	assert.Empty(t, payload.Data.Items[0].ProviderType)
}

func TestRiskRecordGovernanceAPI_getsDefaultsAndUpdatesValidatedSettings(t *testing.T) {
	// Given
	server, client, accessToken := setupRiskRecordRouterTest(t, common.RoleRootUser)

	// When
	getRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/risk/records/settings", nil)
	require.NoError(t, err)
	getRequest.Header.Set("Authorization", "Bearer "+accessToken)
	getResponse, err := client.Do(getRequest)
	require.NoError(t, err)
	defer getResponse.Body.Close()

	// Then
	var defaults struct {
		Success bool                       `json:"success"`
		Data    model.RiskRecordGovernance `json:"data"`
	}
	require.NoError(t, common.DecodeJson(getResponse.Body, &defaults))
	require.True(t, defaults.Success)
	assert.Equal(t, model.RiskRecordSaveAll, defaults.Data.SaveScope)
	assert.Equal(t, model.RiskContentSaveAll, defaults.Data.ContentSaveScope)
	assert.Equal(t, 30, defaults.Data.RetentionDays)
	assert.Equal(t, 200, defaults.Data.PreviewChars)
	assert.Equal(t, 200, defaults.Data.SafePreviewChars)
	assert.Equal(t, 200, defaults.Data.NonSafePreviewChars)

	// When
	updateRequest, err := http.NewRequestWithContext(
		context.Background(), http.MethodPut, server.URL+"/api/risk/records/settings",
		strings.NewReader(`{"save_scope":"unsafe","content_save_scope":"unsafe","retention_days":90,"safe_preview_chars":1200,"non_safe_preview_chars":600}`),
	)
	require.NoError(t, err)
	updateRequest.Header.Set("Content-Type", "application/json")
	updateRequest.Header.Set("Authorization", "Bearer "+accessToken)
	updateResponse, err := client.Do(updateRequest)
	require.NoError(t, err)
	defer updateResponse.Body.Close()

	// Then
	var updated struct {
		Success bool                       `json:"success"`
		Message string                     `json:"message"`
		Data    model.RiskRecordGovernance `json:"data"`
	}
	require.NoError(t, common.DecodeJson(updateResponse.Body, &updated))
	require.True(t, updated.Success, updated.Message)
	assert.Equal(t, model.RiskRecordSaveUnsafe, updated.Data.SaveScope)
	assert.Equal(t, model.RiskContentSaveUnsafe, updated.Data.ContentSaveScope)
	assert.Equal(t, 90, updated.Data.RetentionDays)
	assert.Equal(t, 1200, updated.Data.PreviewChars)
	assert.Equal(t, 1200, updated.Data.SafePreviewChars)
	assert.Equal(t, 600, updated.Data.NonSafePreviewChars)
}

func TestRiskRecordGovernanceAPI_acceptsLegacyPreviewChars(t *testing.T) {
	// Given
	server, client, accessToken := setupRiskRecordRouterTest(t, common.RoleRootUser)
	request, err := http.NewRequestWithContext(
		context.Background(), http.MethodPut, server.URL+"/api/risk/records/settings",
		strings.NewReader(`{"save_scope":"all","content_save_scope":"all","retention_days":30,"preview_chars":1200}`),
	)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)

	// When
	response, err := client.Do(request)

	// Then
	require.NoError(t, err)
	defer response.Body.Close()
	var payload struct {
		Success bool                       `json:"success"`
		Data    model.RiskRecordGovernance `json:"data"`
	}
	require.NoError(t, common.DecodeJson(response.Body, &payload))
	require.True(t, payload.Success)
	assert.Equal(t, 1200, payload.Data.SafePreviewChars)
	assert.Equal(t, 1200, payload.Data.NonSafePreviewChars)
}

func TestRiskRecordGovernanceAPI_rejectsPreviewBelowMinimum(t *testing.T) {
	// Given
	server, client, accessToken := setupRiskRecordRouterTest(t, common.RoleRootUser)
	request, err := http.NewRequestWithContext(
		context.Background(), http.MethodPut, server.URL+"/api/risk/records/settings",
		strings.NewReader(`{"save_scope":"all","content_save_scope":"all","retention_days":30,"safe_preview_chars":49,"non_safe_preview_chars":50}`),
	)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)

	// When
	response, err := client.Do(request)

	// Then
	require.NoError(t, err)
	defer response.Body.Close()
	var payload struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.DecodeJson(response.Body, &payload))
	assert.False(t, payload.Success)
}
