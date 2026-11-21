package controller

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProcessChannelErrorUsesSnapshotWithoutLeakingChannelMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedisEnabled := common.RedisEnabled
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousErrorLogEnabled := constant.ErrorLogEnabled

	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB, model.LOG_DB = database, database
	common.RedisEnabled = false
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	constant.ErrorLogEnabled = true
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled = previousRedisEnabled
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		constant.ErrorLogEnabled = previousErrorLogEnabled
		require.NoError(t, sqlDB.Close())
	})

	require.NoError(t, database.Create(&model.User{Id: 7, Username: "log-owner", Group: "default"}).Error)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set("id", 7)
	ctx.Set("username", "log-owner")
	ctx.Set("token_name", "test-token")
	ctx.Set("token_id", 11)
	ctx.Set("original_model", "gpt-test")
	ctx.Set("group", "default")
	ctx.Set("channel_id", 202)
	ctx.Set("channel_name", "mutable-context-channel")
	ctx.Set("channel_type", 9)
	ctx.Set("use_channel", []string{"101"})
	common.SetContextKey(ctx, constant.ContextKeyRequestStartTime, time.Now().Add(-time.Second))

	channelSnapshot := types.ChannelError{
		ChannelId:   101,
		ChannelType: 1,
		ChannelName: "snapshot-channel",
		AutoBan:     false,
	}
	apiErr := types.NewOpenAIError(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)

	processChannelError(ctx, channelSnapshot, apiErr, nil)

	var stored model.Log
	require.NoError(t, database.First(&stored).Error)
	assert.Equal(t, channelSnapshot.ChannelId, stored.ChannelId)
	storedOther, err := common.StrToMap(stored.Other)
	require.NoError(t, err)
	assert.Equal(t, float64(http.StatusBadGateway), storedOther["status_code"])
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, storedOther, key)
	}
	adminInfo, ok := storedOther["admin_info"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, []any{"101"}, adminInfo["use_channel"])

	logs, total, err := model.GetUserLogs(7, model.LogFilter{Type: model.LogTypeError}, 0, 10)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, logs, 1)
	assert.Equal(t, channelSnapshot.ChannelId, logs[0].ChannelId)
	assert.Empty(t, logs[0].ChannelName)
	userOther, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	assert.NotContains(t, userOther, "admin_info")
	for _, key := range []string{"channel_id", "channel_name", "channel_type"} {
		assert.NotContains(t, userOther, key)
	}
}

func TestChannelHTTP429LogKeepsPublicAndAdminViews(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousEnabled := common.RedisEnabled, constant.ErrorLogEnabled
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Log{}))
	model.DB, model.LOG_DB = database, database
	common.RedisEnabled, constant.ErrorLogEnabled = false, true
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, constant.ErrorLogEnabled = previousRedis, previousEnabled
		common.SetDatabaseTypes(previousMainType, previousLogType)
		require.NoError(t, sqlDB.Close())
	})
	exportSetting := operation_setting.GetLogSetting()
	previousExport := exportSetting.UserExportEnabled
	exportSetting.UserExportEnabled = true
	t.Cleanup(func() { exportSetting.UserExportEnabled = previousExport })
	require.NoError(t, i18n.Init())
	require.NoError(t, database.Create(&model.User{Id: 71, Username: "http429-owner", Group: "default"}).Error)
	const structuredBody = `{"error":{"message":"supplier-message-marker api_key:fixture-secret","type":"supplier-type-marker","code":"supplier-code-marker","param":"supplier-param-marker","metadata":{"provider":"supplier-metadata-marker"}}}`
	for index, tc := range []struct {
		name          string
		status        int
		mapping       string
		body          string
		local         bool
		normalize     bool
		language      string
		publicMessage string
	}{
		{name: "real channel 429", status: 429, body: structuredBody, language: "zh-CN", publicMessage: "服务暂时繁忙，请稍后重试"},
		{name: "real 429 mapped to 503", status: 429, mapping: `{"429":503}`, body: structuredBody, language: "zh-CN", publicMessage: "服务暂时繁忙，请稍后重试"},
		{name: "plain text 429", status: 429, body: "supplier-message-marker api_key:fixture-secret", language: "zh-CN", publicMessage: "服务暂时繁忙，请稍后重试"},
		{name: "english 429", status: 429, body: structuredBody, language: "en", publicMessage: "The service is temporarily busy. Please try again later."},
		{name: "traditional chinese 429", status: 429, body: structuredBody, language: "zh-TW", publicMessage: "服務暫時繁忙，請稍後重試"},
		{name: "normalized policy 429", status: 429, body: strings.ReplaceAll(structuredBody, "supplier-message-marker", "supplier-message-marker Failed check: SAFETY_CHECK_TYPE"), normalize: true, language: "zh-CN", publicMessage: "服务暂时繁忙，请稍后重试"},
		{name: "local 429 unchanged", status: 429, body: structuredBody, local: true},
		{name: "unmarked stream provider 429 unchanged", status: 200, body: structuredBody, local: true},
		{name: "upstream 403 unchanged", status: 403, body: structuredBody},
		{name: "400 mapped to 429 unchanged", status: 400, mapping: `{"400":429}`, body: structuredBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			ctx.Request.Header.Set("Accept-Language", tc.language)
			ctx.Set("id", 71)
			ctx.Set("token_id", index+100)
			ctx.Set(common.RequestIdKey, tc.name)
			common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, "https://supplier.private.example")
			upstreamErr := service.RelayErrorHandler(ctx, &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, false)
			if tc.local {
				upstreamErr = types.WithOpenAIError(types.OpenAIError{Message: "supplier-message-marker api_key:fixture-secret", Type: "supplier-type-marker", Code: "supplier-code-marker", Param: "supplier-param-marker"}, http.StatusTooManyRequests)
			}
			if tc.normalize {
				upstreamErr = service.NormalizeViolationFeeError(upstreamErr)
			}
			service.ResetStatusCode(upstreamErr, tc.mapping)
			originalMessage, originalCode := upstreamErr.Error(), upstreamErr.GetErrorCode()
			originalStatus := upstreamErr.StatusCode

			service.RecordRelayErrorLog(ctx, 101, upstreamErr, nil)

			logs, total, err := model.GetUserLogs(71, model.LogFilter{Type: model.LogTypeError, RequestId: tc.name}, 0, 10)
			require.NoError(t, err)
			require.Equal(t, int64(1), total, "every failed attempt remains recorded")
			require.Len(t, logs, 1)
			assert.Equal(t, tc.name, logs[0].RequestId)
			assert.NotContains(t, logs[0].Content+logs[0].Other, "fixture-secret")
			var other map[string]any
			require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
			assert.EqualValues(t, originalStatus, other["status_code"])
			assert.NotContains(t, other, "admin_info")
			stored, err := model.GetLogByRequestID(tc.name)
			require.NoError(t, err)
			require.NotNil(t, stored)
			model.FormatAdminLogs([]*model.Log{stored})
			var adminOther map[string]any
			require.NoError(t, common.UnmarshalJsonStr(stored.Other, &adminOther))
			admin, ok := adminOther["admin_info"].(map[string]any)
			require.True(t, ok)
			if tc.publicMessage != "" {
				assert.Contains(t, logs[0].Content, tc.publicMessage)
				assert.Equal(t, logs[0].Content, stored.Content)
				assert.NotContains(t, logs[0].Content+logs[0].Other, "supplier-")
				assert.Contains(t, admin["reject_reason"], "supplier-message-marker")
				assert.Contains(t, admin["reject_reason"], "status_code=429")
				assert.Contains(t, admin["reject_reason"], "api_key:fixture-secret")
				tokenLogs, err := model.GetLogByTokenId(index + 100)
				require.NoError(t, err)
				require.Len(t, tokenLogs, 1)
				assert.NotContains(t, tokenLogs[0].Content+tokenLogs[0].Other, "supplier-")
				assert.NotContains(t, tokenLogs[0].Content+tokenLogs[0].Other, "fixture-secret")
				exported := callLogHandler(t, ExportUserLogs, 71, common.RoleCommonUser, "request_id="+url.QueryEscape(logs[0].RequestId))
				require.Len(t, exported, 2)
				exportJSON, err := common.Marshal(exported[0])
				require.NoError(t, err)
				assert.NotContains(t, string(exportJSON), "supplier-")
				assert.NotContains(t, string(exportJSON), "fixture-secret")
				assert.Equal(t, logs[0].RequestId, exported[0]["request_id"])
				assert.Equal(t, true, exported[1]["success"])
				publicErr := service.PublicRelayError(ctx, upstreamErr)
				publicJSON, err := common.Marshal(publicErr.ToOpenAIError())
				require.NoError(t, err)
				assert.NotContains(t, string(publicJSON), "supplier-")
				assert.Empty(t, publicErr.ToOpenAIError().Param)
				assert.Empty(t, publicErr.ToOpenAIError().Metadata)
				assert.Empty(t, publicErr.Metadata)
				assert.Contains(t, publicErr.ToClaudeError().Message, tc.publicMessage)
				assert.NotContains(t, publicErr.ToClaudeError().Type, "supplier-")
				for _, format := range []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude} {
					recorder := httptest.NewRecorder()
					streamCtx, _ := gin.CreateTestContext(recorder)
					streamCtx.Request = ctx.Request
					streamCtx.Set(common.RequestIdKey, "platform-stream-request")
					helper.SetEventStreamHeaders(streamCtx)
					require.NoError(t, helper.PingData(streamCtx))
					writeRelayStreamError(streamCtx, format, upstreamErr)
					assert.Equal(t, http.StatusOK, recorder.Code)
					assert.Contains(t, recorder.Body.String(), tc.publicMessage)
					assert.Contains(t, recorder.Body.String(), "platform-stream-request")
					assert.NotContains(t, recorder.Body.String(), "supplier-")
				}
			} else {
				assert.Contains(t, logs[0].Content, "supplier-message-marker")
				assert.Equal(t, string(originalCode), other["error_code"])
				assert.NotContains(t, admin, "reject_reason")
			}
			assert.Equal(t, originalMessage, upstreamErr.Error(), "logging must preserve internal diagnostics")
			assert.Equal(t, originalCode, upstreamErr.GetErrorCode())
			assert.Equal(t, originalStatus, upstreamErr.StatusCode)
		})
	}
}
