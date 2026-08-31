package controller

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesWebSocket_retries_another_channel_before_first_event_is_committed(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	constant.ErrorLogEnabled = true
	deadServer := httptest.NewServer(nil)
	deadURL := deadServer.URL
	deadServer.Close()

	upstreamRequests := make(chan []byte, 1)
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, payload, err := conn.ReadMessage()
		if err != nil {
			assert.NoError(t, err, "read fallback upstream request")
			return
		}
		upstreamRequests <- payload
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)); err != nil {
			assert.NoError(t, err, "write fallback completion")
		}
		_, _, _ = conn.ReadMessage()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 201, baseURL: deadURL, priority: 100})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 202, baseURL: upstream.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	require.Equal(t, "response.completed", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	closeResponsesWebSocketTestClient(client)

	select {
	case payload := <-upstreamRequests:
		require.Equal(t, "response.create", gjson.GetBytes(payload, "type").String())
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timeout waiting for fallback upstream request")
	}
	require.Equal(t, int32(1), upstream.connections.Load())

	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, model.LogTypeConsume, logs[0].Type)
}

func TestResponsesWebSocket_retries_valid_capacity_sideband_before_output(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	handshakeSent := make(chan struct{})
	releaseCapacity := make(chan struct{})
	primary := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read primary response.create")
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.created","response":{"output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`)); err != nil {
			assert.NoError(t, err, "write primary handshake")
			return
		}
		close(handshakeSent)
		<-releaseCapacity
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(responsesWebSocketCapacityCloseCode, responsesWebSocketCapacityCloseReason), time.Now().Add(time.Second))
	})
	backup := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read backup response.create")
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)); err != nil {
			assert.NoError(t, err, "write backup completion")
			return
		}
		if err := conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second)); err != nil {
			assert.NoError(t, err, "close backup upstream")
		}
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 401, baseURL: primary.server.URL, priority: 100})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 402, baseURL: backup.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	select {
	case <-handshakeSent:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timeout waiting for primary handshake")
	}
	close(releaseCapacity)
	require.Equal(t, "response.completed", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	closeErr := readResponsesWebSocketTestClose(t, client)
	require.Equal(t, websocket.CloseNormalClosure, closeErr.Code)
	closeResponsesWebSocketTestClient(client)
	require.Equal(t, int32(1), primary.connections.Load())
	require.Equal(t, int32(1), backup.connections.Load())

	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, model.LogTypeConsume, logs[0].Type)
}

func TestResponsesWebSocket_doesNotReplayAfterRetryAttemptStateEvent(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	common.RetryTimes = 2
	primary := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read primary response.create")
			return
		}
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(responsesWebSocketCapacityCloseCode, responsesWebSocketCapacityCloseReason), time.Now().Add(time.Second))
	})
	backup := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read backup response.create")
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.in_progress","response":{"status":"in_progress","output":[],"usage":{"total_tokens":0}}}`)); err != nil {
			assert.NoError(t, err, "write backup state event")
			return
		}
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(responsesWebSocketCapacityCloseCode, responsesWebSocketCapacityCloseReason), time.Now().Add(time.Second))
	})
	third := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 421, baseURL: primary.server.URL, priority: 100})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 422, baseURL: backup.server.URL, priority: 0})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 423, baseURL: third.server.URL, priority: -1})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	errorEvent := readResponsesWebSocketTestEvent(t, client)
	require.Equal(t, "server_is_overloaded", gjson.GetBytes(errorEvent, "error.code").String())
	closeErr := readResponsesWebSocketTestClose(t, client)
	require.Equal(t, websocket.CloseInternalServerErr, closeErr.Code)
	closeResponsesWebSocketTestClient(client)

	require.Equal(t, int32(1), primary.connections.Load())
	require.Equal(t, int32(1), backup.connections.Load())
	require.Zero(t, third.connections.Load(), "a retry-attempt state event must block a further replay")
}

func TestResponsesWebSocket_preservesCapacityAfterBackupUnknownDisconnect(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	primary := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, err := conn.ReadMessage()
		assert.NoError(t, err, "read primary response.create")
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(responsesWebSocketCapacityCloseCode, responsesWebSocketCapacityCloseReason), time.Now().Add(time.Second))
	})
	backup := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, err := conn.ReadMessage()
		assert.NoError(t, err, "read backup response.create")
		_ = conn.UnderlyingConn().Close()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 411, baseURL: primary.server.URL, priority: 100})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 412, baseURL: backup.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	errorEvent := readResponsesWebSocketTestEvent(t, client)
	require.Equal(t, "server_is_overloaded", gjson.GetBytes(errorEvent, "error.code").String())
	closeErr := readResponsesWebSocketTestClose(t, client)
	require.Equal(t, websocket.CloseInternalServerErr, closeErr.Code)
	require.Equal(t, int32(1), backup.connections.Load())
}

func TestResponsesWebSocket_preservesCapacityAfterBackupNormalClose(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	primary := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, err := conn.ReadMessage()
		assert.NoError(t, err, "read primary response.create")
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(responsesWebSocketCapacityCloseCode, responsesWebSocketCapacityCloseReason), time.Now().Add(time.Second))
	})
	backup := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, err := conn.ReadMessage()
		assert.NoError(t, err, "read backup response.create")
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second))
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 413, baseURL: primary.server.URL, priority: 100})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 414, baseURL: backup.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	errorEvent := readResponsesWebSocketTestEvent(t, client)
	require.Equal(t, "server_is_overloaded", gjson.GetBytes(errorEvent, "error.code").String())
	closeErr := readResponsesWebSocketTestClose(t, client)
	require.Equal(t, websocket.CloseInternalServerErr, closeErr.Code)
	require.Equal(t, int32(1), backup.connections.Load())
}

func TestResponsesWebSocket_preservesCapacityBeforeBackupTerminalError(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	primary := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, err := conn.ReadMessage()
		assert.NoError(t, err, "read primary response.create")
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(responsesWebSocketCapacityCloseCode, responsesWebSocketCapacityCloseReason), time.Now().Add(time.Second))
	})
	backup := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, err := conn.ReadMessage()
		assert.NoError(t, err, "read backup response.create")
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.failed","response":{"status":"failed","output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`))
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 419, baseURL: primary.server.URL, priority: 100})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 420, baseURL: backup.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	errorEvent := readResponsesWebSocketTestEvent(t, client)
	require.Equal(t, "server_is_overloaded", gjson.GetBytes(errorEvent, "error.code").String())
	closeErr := readResponsesWebSocketTestClose(t, client)
	require.Equal(t, websocket.CloseInternalServerErr, closeErr.Code)
	require.Equal(t, int32(1), backup.connections.Load())
}

func TestResponsesWebSocket_doesNotReplayAfterClientAppend(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	createReceived := make(chan struct{})
	appendReceived := make(chan struct{})
	primary := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, err := conn.ReadMessage()
		assert.NoError(t, err, "read primary response.create")
		close(createReceived)
		_, _, err = conn.ReadMessage()
		assert.NoError(t, err, "read primary response.append")
		close(appendReceived)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(responsesWebSocketCapacityCloseCode, responsesWebSocketCapacityCloseReason), time.Now().Add(time.Second))
	})
	backup := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 415, baseURL: primary.server.URL, priority: 100})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 416, baseURL: backup.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	select {
	case <-createReceived:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timeout waiting for primary response.create")
	}
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.append","input":[]}`)))
	select {
	case <-appendReceived:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timeout waiting for primary response.append")
	}
	errorEvent := readResponsesWebSocketTestEvent(t, client)
	require.Equal(t, "server_is_overloaded", gjson.GetBytes(errorEvent, "error.code").String())
	require.Equal(t, int32(0), backup.connections.Load())
}

func TestResponsesWebSocket_doesNotRetryCapacityDuringDrain(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	primaryReady := make(chan struct{})
	releaseCapacity := make(chan struct{})
	primary := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, err := conn.ReadMessage()
		assert.NoError(t, err, "read primary response.create")
		close(primaryReady)
		<-releaseCapacity
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(responsesWebSocketCapacityCloseCode, responsesWebSocketCapacityCloseReason), time.Now().Add(time.Second))
	})
	backup := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 417, baseURL: primary.server.URL, priority: 100})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 418, baseURL: backup.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	select {
	case <-primaryReady:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timeout waiting for primary response.create")
	}
	BeginResponsesWebSocketDrain()
	close(releaseCapacity)
	closeErr := readResponsesWebSocketTestClose(t, client)
	require.Equal(t, websocket.CloseServiceRestart, closeErr.Code)
	require.Equal(t, int32(0), backup.connections.Load())
}

func TestResponsesWebSocket_does_not_replay_malformed_capacity_sideband(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	primary := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read primary response.create")
			return
		}
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(responsesWebSocketCapacityCloseCode, "ai-cove-capacity/v2;state=rejected;phase=pre_output;code=server_is_overloaded"), time.Now().Add(time.Second))
	})
	backup := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 403, baseURL: primary.server.URL, priority: 100})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 404, baseURL: backup.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	errorEvent := readResponsesWebSocketTestEvent(t, client)
	require.Equal(t, "error", gjson.GetBytes(errorEvent, "type").String())
	require.NotEqual(t, "server_is_overloaded", gjson.GetBytes(errorEvent, "error.code").String())
	_, _, err := client.ReadMessage()
	require.Error(t, err)
	closeResponsesWebSocketTestClient(client)
	require.Equal(t, int32(0), backup.connections.Load())
}

func TestResponsesWebSocket_capacityRetryHonorsZeroBudget(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	common.RetryTimes = 0
	primary := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read primary response.create")
			return
		}
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(responsesWebSocketCapacityCloseCode, responsesWebSocketCapacityCloseReason), time.Now().Add(time.Second))
	})
	backup := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 409, baseURL: primary.server.URL, priority: 100})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 410, baseURL: backup.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	errorEvent := readResponsesWebSocketTestEvent(t, client)
	require.Equal(t, "server_is_overloaded", gjson.GetBytes(errorEvent, "error.code").String())
	closeResponsesWebSocketTestClient(client)
	require.Equal(t, int32(0), backup.connections.Load())
}

func TestResponsesWebSocket_does_not_replay_capacity_after_output(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	primary := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read primary response.create")
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.created","response":{"output":[{"type":"function_call","call_id":"call-1"}],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`)); err != nil {
			assert.NoError(t, err, "write output-bearing handshake")
			return
		}
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(responsesWebSocketCapacityCloseCode, responsesWebSocketCapacityCloseReason), time.Now().Add(time.Second))
	})
	backup := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 405, baseURL: primary.server.URL, priority: 100})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 406, baseURL: backup.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	_ = readResponsesWebSocketTestEvent(t, client)
	closeResponsesWebSocketTestClient(client)
	require.Equal(t, int32(0), backup.connections.Load())
}

func TestResponsesWebSocket_does_not_replay_capacity_after_client_cancel(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	createReceived := make(chan struct{})
	cancelReceived := make(chan struct{})
	primary := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read primary response.create")
			return
		}
		close(createReceived)
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read primary response.cancel")
			return
		}
		close(cancelReceived)
		_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(responsesWebSocketCapacityCloseCode, responsesWebSocketCapacityCloseReason), time.Now().Add(time.Second))
	})
	backup := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 407, baseURL: primary.server.URL, priority: 100})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 408, baseURL: backup.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	select {
	case <-createReceived:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timeout waiting for primary response.create")
	}
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.cancel"}`)))
	select {
	case <-cancelReceived:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timeout waiting for primary response.cancel")
	}
	_ = readResponsesWebSocketTestEvent(t, client)
	closeResponsesWebSocketTestClient(client)
	require.Equal(t, int32(0), backup.connections.Load())
}

func TestResponsesWebSocket_does_not_replay_after_upstream_received_create(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	primaryReceived := make(chan struct{})
	primary := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err != nil {
			assert.NoError(t, err, "read primary upstream request")
			return
		}
		close(primaryReceived)
		_ = conn.UnderlyingConn().Close()
	})
	backup := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 203, baseURL: primary.server.URL, priority: 100})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 204, baseURL: backup.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	select {
	case <-primaryReceived:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timeout waiting for primary upstream request")
	}
	errorEvent := readResponsesWebSocketTestEvent(t, client)
	require.Equal(t, "error", gjson.GetBytes(errorEvent, "type").String())
	_, _, err := client.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.CloseInternalServerErr, closeErr.Code)
	require.Equal(t, int32(0), backup.connections.Load())

	var primaryChannel model.Channel
	require.NoError(t, db.First(&primaryChannel, 203).Error)
	require.Equal(t, common.ChannelStatusEnabled, primaryChannel.Status)
	require.NotContains(t, string(errorEvent), "upstream-key")
}

func TestResponsesWebSocket_records_one_error_when_retry_candidates_are_exhausted(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	constant.ErrorLogEnabled = true
	deadServer := httptest.NewServer(nil)
	deadURL := deadServer.URL
	deadServer.Close()
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 205, baseURL: deadURL, priority: 0})
	client := dialResponsesWebSocketTestClient(t)

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	require.Equal(t, "error", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
	_, _, err := client.ReadMessage()
	require.Error(t, err)

	var logs []model.Log
	require.NoError(t, db.Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, model.LogTypeError, logs[0].Type)
}

func TestResponsesWebSocket_preconsumes_after_auto_group_selection(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	previousUsableGroups := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsableGroups))
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 42).Update("quota", 50).Error)

	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		_, _, _ = conn.ReadMessage()
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 206, baseURL: upstream.server.URL, group: "vip", priority: 0})
	client := dialResponsesWebSocketTestClientWithContext(t, func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
		common.SetContextKey(c, constant.ContextKeyTokenAutoGroups, []string{"vip"})
		common.SetContextKey(c, constant.ContextKeyUserQuota, 50)
	})

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	errorEvent := readResponsesWebSocketTestEvent(t, client)
	require.Equal(t, "error", gjson.GetBytes(errorEvent, "type").String())
	require.Contains(t, gjson.GetBytes(errorEvent, "error.message").String(), "预扣费额度失败")
	_, _, err := client.ReadMessage()
	require.Error(t, err)
	require.Zero(t, upstream.connections.Load())

	var user model.User
	require.NoError(t, db.First(&user, 42).Error)
	require.Equal(t, 50, user.Quota)
}

func TestResponsesWebSocket_reserves_higher_auto_group_before_retry_dial(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	previousUsableGroups := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(previousUsableGroups))
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, db.Model(&model.User{}).Where("id = ?", 42).Update("quota", 50).Error)

	deadServer := httptest.NewServer(nil)
	deadURL := deadServer.URL
	deadServer.Close()
	backup := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		if _, _, err := conn.ReadMessage(); err == nil {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`))
		}
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 207, baseURL: deadURL, group: "default", priority: 0})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 208, baseURL: backup.server.URL, group: "vip", priority: 0})
	client := dialResponsesWebSocketTestClientWithContext(t, func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
		common.SetContextKey(c, constant.ContextKeyTokenGroup, "auto")
		common.SetContextKey(c, constant.ContextKeyTokenAutoGroups, []string{"default", "vip"})
		common.SetContextKey(c, constant.ContextKeyUserQuota, 50)
	})

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)))
	errorEvent := readResponsesWebSocketTestEvent(t, client)
	require.Equal(t, "error", gjson.GetBytes(errorEvent, "type").String())
	require.Zero(t, backup.connections.Load())
}
