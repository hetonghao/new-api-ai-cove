package controller

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"

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
