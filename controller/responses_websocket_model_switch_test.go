package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesWebSocket_rebuilds_upstream_before_model_switch_without_previous_response(t *testing.T) {
	// Given
	db := setupResponsesWebSocketHandlerTest(t)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o-mini":0.075,"gpt-4o-mini-alt":0.075}`))
	upstreamRequests := make(chan []byte, 2)
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			upstreamRequests <- payload
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)); err != nil {
				return
			}
		}
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{
		id:      106,
		baseURL: upstream.server.URL,
		models:  []string{"gpt-4o-mini", "gpt-4o-mini-alt"},
	})
	client := dialResponsesWebSocketTestClient(t)

	// When
	for _, request := range []string{
		`{"type":"response.create","model":"gpt-4o-mini","input":[]}`,
		`{"type":"response.create","model":"gpt-4o-mini-alt","input":[]}`,
	} {
		require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(request)))
		require.Equal(t, "response.completed", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	}

	// Then
	require.Equal(t, "gpt-4o-mini", gjson.GetBytes(<-upstreamRequests, "model").String())
	require.Equal(t, "gpt-4o-mini-alt", gjson.GetBytes(<-upstreamRequests, "model").String())
	require.Equal(t, int32(2), upstream.connections.Load())
	closeResponsesWebSocketTestClient(client)

	var user model.User
	require.NoError(t, db.First(&user, 42).Error)
	require.Equal(t, 2, user.RequestCount)
	var logs []model.Log
	require.NoError(t, db.Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 2)
}

func TestResponsesWebSocket_reports_state_missing_before_model_switch_with_previous_response(t *testing.T) {
	// Given
	db := setupResponsesWebSocketHandlerTest(t)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o-mini":0.075,"gpt-4o-mini-alt":0.075}`))
	upstreamRequests := make(chan []byte, 2)
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		for {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			upstreamRequests <- payload
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp-1","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)); err != nil {
				return
			}
		}
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{
		id:      107,
		baseURL: upstream.server.URL,
		models:  []string{"gpt-4o-mini", "gpt-4o-mini-alt"},
	})
	client := dialResponsesWebSocketTestClient(t)

	// When
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	require.Equal(t, "response.completed", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	firstUpstreamRequest := <-upstreamRequests
	require.Equal(t, "gpt-4o-mini", gjson.GetBytes(firstUpstreamRequest, "model").String())
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini-alt","previous_response_id":"resp-1","input":[]}`)))
	stateMissing := readResponsesWebSocketTestEvent(t, client)
	require.Equal(t, "error", gjson.GetBytes(stateMissing, "type").String())
	require.Equal(t, int64(409), gjson.GetBytes(stateMissing, "status").Int())
	require.Equal(t, "previous_response_not_found", gjson.GetBytes(stateMissing, "error.code").String())
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini-alt","input":[{"role":"user","content":"full context"}]}`)))
	require.Equal(t, "response.completed", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())

	// Then
	secondUpstreamRequest := <-upstreamRequests
	require.Equal(t, "gpt-4o-mini-alt", gjson.GetBytes(secondUpstreamRequest, "model").String())
	require.Equal(t, int32(2), upstream.connections.Load())
	closeResponsesWebSocketTestClient(client)

	var logs []model.Log
	require.NoError(t, db.Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 2)
}
