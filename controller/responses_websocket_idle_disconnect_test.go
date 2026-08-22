package controller

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesWebSocket_idle_upstream_close_does_not_emit_request_error(t *testing.T) {
	// Given
	db := setupResponsesWebSocketHandlerTest(t)
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read upstream request")
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp-idle-close","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)); err != nil {
			assert.NoError(t, err, "write response.completed")
			return
		}
		assert.NoError(t, conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseServiceRestart, "service restart"),
			time.Now().Add(time.Second),
		))
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 306, baseURL: upstream.server.URL})
	client := dialResponsesWebSocketTestClient(t)

	// When
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))

	// Then
	require.Equal(t, "response.completed", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	require.Equal(t, websocket.CloseServiceRestart, readResponsesWebSocketTestClose(t, client).Code)
	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, model.LogTypeConsume, logs[0].Type)
}

func TestResponsesWebSocket_active_client_disconnect_does_not_record_generic_request_error(t *testing.T) {
	// Given
	db := setupResponsesWebSocketHandlerTest(t)
	constant.ErrorLogEnabled = true
	cleanupBefore := responsesWebSocketRuntime.cleanup.Load()
	requestReceived := make(chan struct{})
	upstreamDone := make(chan struct{})
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		defer close(upstreamDone)
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read upstream request")
			return
		}
		close(requestReceived)
		_, _, _ = conn.ReadMessage()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 307, baseURL: upstream.server.URL})
	client := dialResponsesWebSocketTestClient(t)

	// When
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	select {
	case <-requestReceived:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for upstream request")
	}
	require.NoError(t, client.Close())

	// Then
	select {
	case <-upstreamDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for upstream shutdown")
	}
	require.Eventually(t, func() bool {
		return responsesWebSocketRuntime.cleanup.Load() > cleanupBefore
	}, 5*time.Second, time.Millisecond)
	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Empty(t, logs)
}

type responsesWebSocketTestBilling struct {
	refunds int
}

func (*responsesWebSocketTestBilling) Settle(int) error         { return nil }
func (*responsesWebSocketTestBilling) NeedsRefund() bool        { return true }
func (*responsesWebSocketTestBilling) GetPreConsumedQuota() int { return 0 }
func (*responsesWebSocketTestBilling) Reserve(int) error        { return nil }
func (billing *responsesWebSocketTestBilling) Refund(*gin.Context) {
	billing.refunds++
}

func TestResponsesWebSocket_client_disconnect_cleanup_finalizes_once(t *testing.T) {
	// Given
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	billing := &responsesWebSocketTestBilling{}
	active := &responsesWebSocketRequestState{
		ctx:  ctx,
		info: &relaycommon.RelayInfo{Billing: billing},
	}
	var upstream *websocket.Conn

	// When
	cleanupResponsesWebSocketSession(&active, &upstream, responsesWebSocketCleanupClientDisconnected)
	cleanupResponsesWebSocketSession(&active, &upstream, responsesWebSocketCleanupClientDisconnected)

	// Then
	require.Nil(t, active)
	require.Equal(t, 1, billing.refunds)
}

func TestResponsesWebSocket_active_upstream_close_records_upstream_disconnect_once(t *testing.T) {
	// Given
	db := setupResponsesWebSocketHandlerTest(t)
	constant.ErrorLogEnabled = true
	cleanupBefore := responsesWebSocketRuntime.cleanup.Load()
	requestReceived := make(chan struct{})
	upstreamDone := make(chan struct{})
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		defer close(upstreamDone)
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read upstream request")
			return
		}
		close(requestReceived)
		assert.NoError(t, conn.Close())
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 308, baseURL: upstream.server.URL})
	client := dialResponsesWebSocketTestClient(t)

	// When
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	select {
	case <-requestReceived:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for upstream request")
	}

	// Then
	errorEvent := readResponsesWebSocketTestEvent(t, client)
	require.Equal(t, "error", gjson.GetBytes(errorEvent, "type").String())
	require.Contains(t, gjson.GetBytes(errorEvent, "error.message").String(), "upstream websocket disconnected")
	require.Equal(t, websocket.CloseInternalServerErr, readResponsesWebSocketTestClose(t, client).Code)
	select {
	case <-upstreamDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for upstream shutdown")
	}
	require.Eventually(t, func() bool {
		return responsesWebSocketRuntime.cleanup.Load() > cleanupBefore
	}, 5*time.Second, time.Millisecond)
	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, model.LogTypeError, logs[0].Type)
	require.Equal(t, "status_code=500, upstream disconnected", logs[0].Content)
	require.Equal(t, "upstream disconnected", gjson.Get(logs[0].Other, "websocket_close_reason").String())
}
