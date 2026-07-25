package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
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

func setupRiskRecordRouterTest(t *testing.T, role int) (*httptest.Server, *http.Client) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.RiskRecord{}, &model.RiskRecordGovernance{}))

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("risk-record-route-test"))))
	engine.GET("/test/session", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("username", "root")
		session.Set("role", role)
		session.Set("id", 1)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", "default")
		if err := session.Save(); err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	registerRiskPolicyRoutes(engine.Group("/api"))
	server := httptest.NewServer(engine)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := server.Client()
	client.Jar = jar
	response, err := client.Get(server.URL + "/test/session")
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	t.Cleanup(func() {
		server.Close()
		model.DB = originalDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return server, client
}

func TestRiskRecordAPI_returnsRootOnlyPaginatedMetadata(t *testing.T) {
	// Given
	server, client := setupRiskRecordRouterTest(t, common.RoleRootUser)
	baseTime := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)
	for index, requestID := range []string{"req-old", "req-new"} {
		require.NoError(t, model.RecordRiskObservation(context.Background(), model.RiskRecordInput{
			RequestID: requestID, ChannelID: 12, UserID: 34, RuleIDs: []int{5}, ProviderID: 21,
			ProviderName: "Cloudflare", Result: model.RiskRecordResultSafe, Categories: []string{},
			LatencyMS: 93, PromptTokens: 11, CompletionTokens: 2, TotalTokens: 13, Neurons: 7,
			ProviderCalled: index == 1,
			ObservedAt:     baseTime.Add(time.Duration(index) * time.Minute),
		}))
	}
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/risk/records?p=1&page_size=1", nil)
	require.NoError(t, err)
	request.Header.Set("New-Api-User", "1")

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
	assert.Empty(t, record.ErrorCode)
	assert.True(t, record.ProviderCalled)
	assert.Equal(t, baseTime.Add(time.Minute), record.ObservedAt)
}

func TestRiskRecordAPI_rejectsNonRootAdmin(t *testing.T) {
	// Given
	server, client := setupRiskRecordRouterTest(t, common.RoleAdminUser)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/risk/records", nil)
	require.NoError(t, err)
	request.Header.Set("New-Api-User", "1")

	// When
	response, err := client.Do(request)

	// Then
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var payload riskRecordAPIResponse
	require.NoError(t, common.DecodeJson(response.Body, &payload))
	assert.False(t, payload.Success)
}

func TestRiskRecordAPI_filtersGovernedRecords(t *testing.T) {
	// Given
	server, client := setupRiskRecordRouterTest(t, common.RoleRootUser)
	observedAt := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)
	for _, input := range []model.RiskRecordInput{
		{
			RequestID: "req-match", ChannelID: 12, UserID: 34, ProviderID: 21, ProviderName: "Cloudflare",
			Result: model.RiskRecordResultUnsafe, Source: model.RiskRecordSourceInflight,
			ObservedAt: observedAt,
		},
		{
			RequestID: "req-other", ChannelID: 99, UserID: 88, ProviderID: 77, ProviderName: "Other",
			Result: model.RiskRecordResultSafe, Source: model.RiskRecordSourceProvider,
			ObservedAt: observedAt.Add(time.Minute),
		},
	} {
		require.NoError(t, model.RecordRiskObservation(context.Background(), input))
	}
	url := fmt.Sprintf(
		"%s/api/risk/records?p=1&page_size=20&start_timestamp=%d&end_timestamp=%d&channel_id=12&user_id=34&result=unsafe&source=inflight&provider_id=21",
		server.URL, observedAt.Unix(), observedAt.Unix(),
	)
	request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	require.NoError(t, err)
	request.Header.Set("New-Api-User", "1")

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
	server, client := setupRiskRecordRouterTest(t, common.RoleRootUser)

	// When
	getRequest, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/risk/records/settings", nil)
	require.NoError(t, err)
	getRequest.Header.Set("New-Api-User", "1")
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
	assert.Equal(t, 30, defaults.Data.RetentionDays)

	// When
	updateRequest, err := http.NewRequestWithContext(
		context.Background(), http.MethodPut, server.URL+"/api/risk/records/settings",
		strings.NewReader(`{"save_scope":"unsafe","retention_days":90}`),
	)
	require.NoError(t, err)
	updateRequest.Header.Set("Content-Type", "application/json")
	updateRequest.Header.Set("New-Api-User", "1")
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
	assert.Equal(t, 90, updated.Data.RetentionDays)
}
