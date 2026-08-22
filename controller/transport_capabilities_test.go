package controller

import (
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestParseTransportCapabilityModelsDeduplicatesInRequestOrder(t *testing.T) {
	models, ok := parseTransportCapabilityModels([]string{" model-a,model-b", "model-a", "model-c "})
	require.True(t, ok)
	require.Equal(t, []string{"model-a", "model-b", "model-c"}, models)
}

func TestParseTransportCapabilityModelsRejectsEmptyInput(t *testing.T) {
	models, ok := parseTransportCapabilityModels([]string{" , ", ""})
	require.False(t, ok)
	require.Empty(t, models)
}

func TestTransportCapabilitiesRejectsTooManyModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/v1/transport/capabilities", TransportCapabilities)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/v1/transport/capabilities?models="+repeatedTransportCapabilityModels(101), nil)
	engine.ServeHTTP(recorder, request)
	require.Equal(t, 400, recorder.Code)
	require.Contains(t, recorder.Body.String(), "too_many_models")
}

func TestTransportCapabilitiesReturnsOrderedTTLAndLocalHints(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	common.MemoryCacheEnabled = false
	user := model.User{Id: 991, Username: "capability-user", Password: "password", Group: "default", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)
	for _, channel := range []*model.Channel{
		{Id: 992, Type: constant.ChannelTypeCodex, Key: "local-http", Status: common.ChannelStatusEnabled, Name: "http", Models: "cap-http", Group: "default"},
		{Id: 993, Type: constant.ChannelTypeCodex, Key: "local-ws", Status: common.ChannelStatusEnabled, Name: "ws", Models: "cap-http", Group: "default"},
		{Id: 996, Type: constant.ChannelTypeCodex, Key: "http-only", Status: common.ChannelStatusEnabled, Name: "http-only", Models: "cap-http-only", Group: "default"},
	} {
		channel.SetOtherSettings(dto.ChannelOtherSettings{SupportsWebSockets: channel.Id == 993})
		require.NoError(t, db.Create(channel).Error)
		abilityModel := "cap-http"
		if channel.Id == 996 {
			abilityModel = "cap-http-only"
		}
		require.NoError(t, db.Create(&model.Ability{Group: "default", Model: abilityModel, ChannelId: channel.Id, Enabled: true}).Error)
	}
	chatChannel := &model.Channel{Id: 994, Type: constant.ChannelTypeOpenAI, Key: "chat-only", Status: common.ChannelStatusEnabled, Name: "chat", Models: "cap-chat", Group: "default"}
	require.NoError(t, db.Create(chatChannel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "cap-chat", ChannelId: chatChannel.Id, Enabled: true}).Error)
	customChannel := &model.Channel{Id: 995, Type: constant.ChannelTypeAdvancedCustom, Key: "custom-chat", Status: common.ChannelStatusEnabled, Name: "custom-chat", Models: "cap-custom-chat", Group: "default"}
	customChannel.SetOtherSettings(dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/chat/completions", UpstreamPath: "/v1/chat/completions", Models: []string{"cap-custom-chat"}}}}})
	require.NoError(t, db.Create(customChannel).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: "cap-custom-chat", ChannelId: customChannel.Id, Enabled: true}).Error)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest("GET", "/v1/transport/capabilities?models=missing,cap-http,cap-chat,cap-custom-chat,cap-http-only,missing", nil)
	context.Set("id", user.Id)
	TransportCapabilities(context)
	require.Equal(t, 200, recorder.Code)
	payload := gjson.Parse(recorder.Body.String())
	require.True(t, payload.Get("success").Bool())
	require.Equal(t, float64(1), payload.Get("version").Float())
	require.Equal(t, float64(5), payload.Get("data.#").Float())
	require.Equal(t, "missing", payload.Get("data.0.model").String())
	require.False(t, payload.Get("data.0.allowed").Bool())
	require.Equal(t, "model_not_allowed", payload.Get("data.0.reason_code").String())
	require.Equal(t, "cap-http", payload.Get("data.1.model").String())
	require.True(t, payload.Get("data.1.allowed").Bool())
	require.True(t, payload.Get("data.1.http").Bool())
	require.True(t, payload.Get("data.1.responses_websocket").Bool())
	require.Equal(t, "ok", payload.Get("data.1.reason_code").String())
	require.Equal(t, "cap-chat", payload.Get("data.2.model").String())
	require.True(t, payload.Get("data.2.allowed").Bool())
	require.False(t, payload.Get("data.2.http").Bool())
	require.Equal(t, "no_http_channel", payload.Get("data.2.reason_code").String())
	require.Equal(t, "cap-custom-chat", payload.Get("data.3.model").String())
	require.True(t, payload.Get("data.3.allowed").Bool())
	require.False(t, payload.Get("data.3.http").Bool())
	require.Equal(t, "no_http_channel", payload.Get("data.3.reason_code").String())
	require.Equal(t, "cap-http-only", payload.Get("data.4.model").String())
	require.True(t, payload.Get("data.4.allowed").Bool())
	require.True(t, payload.Get("data.4.http").Bool())
	require.False(t, payload.Get("data.4.responses_websocket").Bool())
	require.Equal(t, "no_responses_websocket_channel", payload.Get("data.4.reason_code").String())
	require.NotEmpty(t, payload.Get("generated_at").String())
	require.NotEmpty(t, payload.Get("expires_at").String())
	require.NotContains(t, recorder.Body.String(), "992")
}

func repeatedTransportCapabilityModels(count int) string {
	models := make([]byte, 0, count*9)
	for i := 0; i < count; i++ {
		if i > 0 {
			models = append(models, ',')
		}
		models = append(models, "model-"...)
		models = strconv.AppendInt(models, int64(i), 10)
	}
	return string(models)
}
