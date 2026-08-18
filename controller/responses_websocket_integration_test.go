package controller

import (
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesWebSocket_drains_idle_and_new_sessions_with_service_restart(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 300, baseURL: "http://127.0.0.1:1", priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	BeginResponsesWebSocketDrain()
	require.Equal(t, websocket.CloseServiceRestart, readResponsesWebSocketTestClose(t, client).Code)

	newClient := dialResponsesWebSocketTestClient(t)
	require.Equal(t, websocket.CloseServiceRestart, readResponsesWebSocketTestClose(t, newClient).Code)
}

func TestResponsesWebSocket_drain_rejects_queued_create_when_idle(t *testing.T) {
	state := newResponsesWebSocketDrainState()
	state.draining.Store(true)

	require.True(t, state.shouldRejectNewResponse(nil, "response.create"))
	require.False(t, state.shouldRejectNewResponse(&responsesWebSocketRequestState{}, "response.create"))
	require.False(t, state.shouldRejectNewResponse(nil, "response.cancel"))
}

func TestResponsesWebSocket_drains_active_session_after_terminal_frame(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	requestReceived := make(chan struct{})
	release := make(chan struct{})
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read upstream request")
			return
		}
		close(requestReceived)
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.created","response":{"id":"resp-drain"}}`)); err != nil {
			assert.NoError(t, err, "write response.created")
			return
		}
		<-release
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)); err != nil {
			assert.NoError(t, err, "write response.completed")
		}
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 301, baseURL: upstream.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	require.Equal(t, "response.created", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	select {
	case <-requestReceived:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timeout waiting for upstream request")
	}

	BeginResponsesWebSocketDrain()
	close(release)
	require.Equal(t, "response.completed", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	require.Equal(t, websocket.CloseServiceRestart, readResponsesWebSocketTestClose(t, client).Code)
}

func TestResponsesWebSocket_drains_cancelled_active_session_after_cancel_event(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	requestReceived := make(chan struct{})
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read upstream create")
			return
		}
		close(requestReceived)
		_, payload, err := conn.ReadMessage()
		if err != nil {
			assert.NoError(t, err, "read upstream cancel")
			return
		}
		if !assert.Equal(t, "response.cancel", gjson.GetBytes(payload, "type").String(), "unexpected upstream event: %s", payload) {
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.cancelled","response":{"status":"cancelled"}}`)); err != nil {
			assert.NoError(t, err, "write response.cancelled")
		}
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 302, baseURL: upstream.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	select {
	case <-requestReceived:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timeout waiting for upstream request")
	}

	BeginResponsesWebSocketDrain()
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.cancel"}`)))
	require.Equal(t, "response.cancelled", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	require.Equal(t, websocket.CloseServiceRestart, readResponsesWebSocketTestClose(t, client).Code)
}

func TestResponsesWebSocket_forwards_two_sequential_creates_on_one_upstream_connection(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	upstreamRequests := make(chan []byte, 2)
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		for i := 1; i <= 2; i++ {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				t.Errorf("read upstream request %d: %v", i, err)
				return
			}
			upstreamRequests <- payload
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.created","response":{"id":"resp-test"}}`)); err != nil {
				t.Errorf("write response.created: %v", err)
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp-test","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)); err != nil {
				t.Errorf("write response.completed: %v", err)
				return
			}
		}
		_, _, _ = conn.ReadMessage()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 101, baseURL: upstream.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	requests := []string{
		`{"type":"response.create","model":"gpt-4o-mini","input":[]}`,
		`{"type":"response.create","input":[]}`,
	}
	for _, request := range requests {
		require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(request)))
		require.Equal(t, "response.created", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
		require.Equal(t, "response.completed", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	}
	closeResponsesWebSocketTestClient(client)

	require.Equal(t, int32(1), upstream.connections.Load())
	for i := 0; i < 2; i++ {
		select {
		case payload := <-upstreamRequests:
			require.Equal(t, "response.create", gjson.GetBytes(payload, "type").String())
			require.Equal(t, responsesWebSocketTestModel, gjson.GetBytes(payload, "model").String())
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for upstream request %d", i+1)
		}
	}

	var user model.User
	require.NoError(t, db.First(&user, 42).Error)
	require.Equal(t, 2, user.RequestCount)

	var logs []model.Log
	require.NoError(t, db.Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 2)
	for _, log := range logs {
		require.Equal(t, "websocket", gjson.Get(log.Other, "transport").String())
		require.True(t, gjson.Get(log.Other, "websocket_first_output_ms").Exists())
		require.True(t, gjson.Get(log.Other, "websocket_complete_ms").Exists())
	}
	require.True(t, gjson.Get(logs[0].Other, "websocket_upstream_connect_ms").Exists())
	require.True(t, gjson.Get(logs[0].Other, "websocket_first_event_ms").Exists())
	require.False(t, gjson.Get(logs[1].Other, "websocket_upstream_connect_ms").Exists())
	require.False(t, gjson.Get(logs[1].Other, "websocket_first_event_ms").Exists())
}

func TestResponsesWebSocket_rejects_concurrent_create_without_forwarding_it(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	firstRequest := make(chan struct{})
	release := make(chan struct{})
	unexpected := make(chan []byte, 1)
	upstreamDone := make(chan struct{})
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		defer close(upstreamDone)
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Errorf("read first upstream request: %v", err)
			return
		}
		close(firstRequest)
		<-release
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)); err != nil {
			t.Errorf("write terminal response: %v", err)
			return
		}
		for {
			messageType, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if messageType == websocket.TextMessage {
				unexpected <- payload
			}
		}
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 102, baseURL: upstream.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	create := []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, create))
	select {
	case <-firstRequest:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for first upstream request")
	}
	require.NoError(t, client.WriteMessage(websocket.TextMessage, create))
	errorEvent := readResponsesWebSocketTestEvent(t, client)
	require.Equal(t, "error", gjson.GetBytes(errorEvent, "type").String())
	require.Equal(t, "invalid_request", gjson.GetBytes(errorEvent, "error.code").String())
	close(release)
	require.Equal(t, "response.completed", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	closeResponsesWebSocketTestClient(client)

	select {
	case <-upstreamDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for upstream shutdown")
	}
	select {
	case payload := <-unexpected:
		t.Fatalf("concurrent create reached upstream: %s", payload)
	default:
	}
}

func TestResponsesWebSocket_rejects_idle_event_without_forwarding_it(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	upstreamEvents := make(chan []byte, 2)
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		for i := 0; i < 2; i++ {
			_, payload, err := conn.ReadMessage()
			if err != nil {
				t.Errorf("read upstream request %d: %v", i+1, err)
				return
			}
			upstreamEvents <- payload
			if gjson.GetBytes(payload, "type").String() != "response.create" {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)); err != nil {
				t.Errorf("write terminal response: %v", err)
				return
			}
		}
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 103, baseURL: upstream.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	create := []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, create))
	require.Equal(t, "response.completed", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.cancel"}`)))
	require.Equal(t, "error", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	require.NoError(t, client.WriteMessage(websocket.TextMessage, create))
	require.Equal(t, "response.completed", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	closeResponsesWebSocketTestClient(client)

	for i := 0; i < 2; i++ {
		select {
		case payload := <-upstreamEvents:
			require.Equal(t, "response.create", gjson.GetBytes(payload, "type").String())
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for upstream request %d", i+1)
		}
	}
}

func TestResponsesWebSocket_applies_model_request_rate_limit_per_create(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	setting.ModelRequestRateLimitEnabled = true
	setting.ModelRequestRateLimitCount = 1
	setting.ModelRequestRateLimitSuccessCount = 1000

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
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 104, baseURL: upstream.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	create := []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, create))
	require.Equal(t, "response.completed", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	require.NoError(t, client.WriteMessage(websocket.TextMessage, create))
	require.Equal(t, "error", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())

	select {
	case payload := <-upstreamRequests:
		require.Equal(t, "response.create", gjson.GetBytes(payload, "type").String())
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for first upstream request")
	}
	select {
	case payload := <-upstreamRequests:
		t.Fatalf("rate-limited request reached upstream: %s", payload)
	default:
	}
}

func TestResponsesWebSocket_records_pre_channel_validation_error(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	constant.ErrorLogEnabled = true
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 105, baseURL: upstream.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","input":[]}`)))
	require.Equal(t, "error", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	_, _, err := client.ReadMessage()
	require.Error(t, err)

	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, model.LogTypeError, logs[0].Type)
	require.Zero(t, logs[0].ChannelId)
	require.NotEmpty(t, logs[0].RequestId)
	require.Equal(t, "websocket", gjson.Get(logs[0].Other, "transport").String())
	require.Equal(t, "/v1/responses", gjson.Get(logs[0].Other, "request_path").String())
}

func TestResponsesWebSocket_signals_http_fallback_when_no_websocket_channel_before_submission(t *testing.T) {
	// Given: the model has an HTTP channel, but no Responses WebSocket channel.
	db := setupResponsesWebSocketHandlerTest(t)
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 106, baseURL: upstream.server.URL, priority: 0})
	var httpOnlyChannel model.Channel
	require.NoError(t, db.First(&httpOnlyChannel, 106).Error)
	httpOnlyChannel.SetOtherSettings(dto.ChannelOtherSettings{SupportsWebSockets: false})
	require.NoError(t, db.Save(&httpOnlyChannel).Error)
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 107, baseURL: upstream.server.URL, priority: 0, models: []string{"other-model"}})
	client := dialResponsesWebSocketTestClient(t)

	// When: the client submits the first response.create.
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[{"role":"user","content":"hello"}]}`)))
	errorEvent := readResponsesWebSocketTestEvent(t, client)

	// Then: New API proves the request was not submitted and permits HTTP transport fallback.
	require.Equal(t, "error", gjson.GetBytes(errorEvent, "type").String())
	require.Equal(t, "responses_websocket_unavailable", gjson.GetBytes(errorEvent, "error.code").String())
	require.Equal(t, int64(http.StatusServiceUnavailable), gjson.GetBytes(errorEvent, "status").Int())
	require.Equal(t, "http", gjson.GetBytes(errorEvent, "transport").String())
	require.Equal(t, "not_submitted", gjson.GetBytes(errorEvent, "request_state").String())
	require.Zero(t, upstream.connections.Load())
}
