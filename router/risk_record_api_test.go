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
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.RiskRecord{}, &model.RiskRecordGovernance{}, &model.User{}))
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
		common.SetDatabaseTypes(originalMainType, originalLogType)
		common.RedisEnabled = originalRedisEnabled
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return server, client, accessToken
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
			RequestID: "req-match", ChannelID: 12, UserID: 34, ProviderID: 21, ProviderName: "Cloudflare",
			Result: model.RiskRecordResultUnsafe, Source: model.RiskRecordSourceInflight,
			ObservedAt: observedAt,
		},
		{
			RequestID: "req-other-user", ChannelID: 12, UserID: 35, ProviderID: 21, ProviderName: "Cloudflare",
			Result: model.RiskRecordResultUnsafe, Source: model.RiskRecordSourceInflight,
			ObservedAt: observedAt,
		},
	} {
		require.NoError(t, model.RecordRiskObservation(context.Background(), input))
	}
	url := fmt.Sprintf(
		"%s/api/risk/records?p=1&page_size=20&start_timestamp=%d&end_timestamp=%d&channel_id=12&username=alice&result=unsafe&source=inflight&provider_id=21",
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

	// When
	updateRequest, err := http.NewRequestWithContext(
		context.Background(), http.MethodPut, server.URL+"/api/risk/records/settings",
		strings.NewReader(`{"save_scope":"unsafe","content_save_scope":"unsafe","retention_days":90}`),
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
}
