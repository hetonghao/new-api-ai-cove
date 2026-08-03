package controller

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const responsesWebSocketTestModel = "gpt-4o-mini"

type responsesWebSocketTestChannel struct {
	id       int
	baseURL  string
	group    string
	priority int64
}

type responsesWebSocketTestUpstream struct {
	server      *httptest.Server
	connections atomic.Int32
}

func setupResponsesWebSocketHandlerTest(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousMainDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	previousRetryTimes := common.RetryTimes
	previousLogConsumeEnabled := common.LogConsumeEnabled
	previousDataExportEnabled := common.DataExportEnabled
	previousBatchUpdateEnabled := common.BatchUpdateEnabled
	previousErrorLogEnabled := constant.ErrorLogEnabled
	previousModelRequestRateLimitEnabled := setting.ModelRequestRateLimitEnabled
	previousModelRequestRateLimitDurationMinutes := setting.ModelRequestRateLimitDurationMinutes
	previousModelRequestRateLimitCount := setting.ModelRequestRateLimitCount
	previousModelRequestRateLimitSuccessCount := setting.ModelRequestRateLimitSuccessCount
	previousGroupRatios := ratio_setting.GroupRatio2JSONString()
	previousModelRatios := ratio_setting.ModelRatio2JSONString()

	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	common.MemoryCacheEnabled = false
	common.RetryTimes = 1
	common.LogConsumeEnabled = true
	common.DataExportEnabled = false
	common.BatchUpdateEnabled = false
	constant.ErrorLogEnabled = false
	setting.ModelRequestRateLimitEnabled = false
	setting.ModelRequestRateLimitDurationMinutes = 1
	setting.ModelRequestRateLimitCount = 0
	setting.ModelRequestRateLimitSuccessCount = 1000
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":0}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o-mini":0.075}`))
	service.InitTokenEncoders()

	user := model.User{
		Id:       42,
		Username: "responses-ws-test",
		Password: "responses-ws-test-password",
		Status:   common.UserStatusEnabled,
		Quota:    1_000_000,
		Group:    "default",
		Setting:  `{"billing_preference":"wallet_only"}`,
	}
	require.NoError(t, db.Create(&user).Error)

	t.Cleanup(func() {
		deadline := time.Now().Add(5 * time.Second)
		for gopool.WorkerCount() > 0 {
			if time.Now().After(deadline) {
				t.Fatal("timeout waiting for background relay metrics")
			}
			runtime.Gosched()
		}
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SetDatabaseTypes(previousMainDatabaseType, previousLogDatabaseType)
		common.RedisEnabled = previousRedisEnabled
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
		common.RetryTimes = previousRetryTimes
		common.LogConsumeEnabled = previousLogConsumeEnabled
		common.DataExportEnabled = previousDataExportEnabled
		common.BatchUpdateEnabled = previousBatchUpdateEnabled
		constant.ErrorLogEnabled = previousErrorLogEnabled
		setting.ModelRequestRateLimitEnabled = previousModelRequestRateLimitEnabled
		setting.ModelRequestRateLimitDurationMinutes = previousModelRequestRateLimitDurationMinutes
		setting.ModelRequestRateLimitCount = previousModelRequestRateLimitCount
		setting.ModelRequestRateLimitSuccessCount = previousModelRequestRateLimitSuccessCount
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatios))
	})

	return db
}

func insertResponsesWebSocketTestChannel(t *testing.T, db *gorm.DB, spec responsesWebSocketTestChannel) {
	t.Helper()

	group := spec.group
	if group == "" {
		group = "default"
	}
	weight := uint(100)
	channel := model.Channel{
		Id:       spec.id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      "upstream-key",
		Status:   common.ChannelStatusEnabled,
		Name:     "responses websocket test",
		Weight:   &weight,
		BaseURL:  &spec.baseURL,
		Models:   responsesWebSocketTestModel,
		Group:    group,
		Priority: &spec.priority,
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{SupportsWebSockets: true})
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     responsesWebSocketTestModel,
		ChannelId: spec.id,
		Enabled:   true,
		Priority:  &spec.priority,
		Weight:    100,
	}).Error)
}

func newResponsesWebSocketTestUpstream(t *testing.T, serve func(*websocket.Conn)) *responsesWebSocketTestUpstream {
	t.Helper()

	upstream := &responsesWebSocketTestUpstream{}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	upstream.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade upstream websocket: %v", err)
			return
		}
		upstream.connections.Add(1)
		defer conn.Close()
		serve(conn)
	}))
	t.Cleanup(upstream.server.Close)
	return upstream
}

func dialResponsesWebSocketTestClient(t *testing.T) *websocket.Conn {
	return dialResponsesWebSocketTestClientWithContext(t, nil)
}

func dialResponsesWebSocketTestClientWithContext(t *testing.T, customize func(*gin.Context)) *websocket.Conn {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUserId, 42)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserQuota, 1_000_000)
		common.SetContextKey(c, constant.ContextKeyUserEmail, "responses-ws@example.com")
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
		common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
		c.Set("username", "responses-ws-test")
		c.Set("token_name", "responses-ws-token")
		if customize != nil {
			customize(c)
		}
		c.Next()
	})
	router.GET("/v1/responses", middleware.ResponsesWebSocketPreflight(), ResponsesWebSocket)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func readResponsesWebSocketTestEvent(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	messageType, payload, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, messageType)
	return payload
}

func closeResponsesWebSocketTestClient(conn *websocket.Conn) {
	_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second))
	_ = conn.Close()
}
