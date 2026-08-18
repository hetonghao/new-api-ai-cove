package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupResponsesWebSocketPreflightTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := model.DB
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	model.DB = db
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func runResponsesWebSocketPreflight(t *testing.T, specificChannel any) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/responses", func(c *gin.Context) {
		if specificChannel != nil {
			common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, specificChannel)
		}
		ResponsesWebSocketPreflight()(c)
		if !c.IsAborted() {
			c.Status(http.StatusNoContent)
		}
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/responses", nil))
	return recorder
}

func TestResponsesWebSocketPreflight_returns_426_when_no_channel_supports_websocket(t *testing.T) {
	setupResponsesWebSocketPreflightTestDB(t)

	recorder := runResponsesWebSocketPreflight(t, nil)

	require.Equal(t, http.StatusUpgradeRequired, recorder.Code)
}

func TestResponsesWebSocketPreflight_returns_400_for_invalid_specific_channel(t *testing.T) {
	setupResponsesWebSocketPreflightTestDB(t)

	recorder := runResponsesWebSocketPreflight(t, "invalid")

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestResponsesWebSocketPreflight_accepts_supported_responses_websocket_channels(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
	}{
		{name: "OpenAI", channelType: constant.ChannelTypeOpenAI},
		{name: "Codex", channelType: constant.ChannelTypeCodex},
		{name: "Advanced Custom", channelType: constant.ChannelTypeAdvancedCustom},
		{name: "Sub2API", channelType: constant.ChannelTypeSub2API},
		{name: "New API", channelType: constant.ChannelTypeNewAPI},
		{name: "xAI", channelType: constant.ChannelTypeXai},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupResponsesWebSocketPreflightTestDB(t)
			channel := model.Channel{
				Id:     7,
				Name:   tt.name + " WebSocket",
				Type:   tt.channelType,
				Status: common.ChannelStatusEnabled,
				Key:    "test-key",
			}
			channel.SetOtherSettings(dto.ChannelOtherSettings{SupportsWebSockets: true})
			require.NoError(t, db.Create(&channel).Error)

			recorder := runResponsesWebSocketPreflight(t, nil)

			require.Equal(t, http.StatusNoContent, recorder.Code)
		})
	}
}

func TestResponsesWebSocketPreflight_returns_426_for_unsupported_specific_channel(t *testing.T) {
	db := setupResponsesWebSocketPreflightTestDB(t)
	channel := model.Channel{
		Id:     8,
		Name:   "OpenAI HTTP only",
		Type:   constant.ChannelTypeOpenAI,
		Status: common.ChannelStatusEnabled,
		Key:    "test-key",
	}
	require.NoError(t, db.Create(&channel).Error)

	recorder := runResponsesWebSocketPreflight(t, channel.Id)

	require.Equal(t, http.StatusUpgradeRequired, recorder.Code)
}
