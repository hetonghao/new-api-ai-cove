package controller

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesWebSocketValidationPayload_inherits_session_model(t *testing.T) {
	t.Parallel()

	payload, modelName, err := responsesWebSocketValidationPayload([]byte(`{"type":"response.create","input":[]}`), "gpt-5.4")

	require.NoError(t, err)
	require.Equal(t, "gpt-5.4", modelName)
	require.JSONEq(t, `{"type":"response.create","model":"gpt-5.4","input":[]}`, string(payload))
}

func TestResponsesWebSocketValidationPayload_rejects_missing_initial_model(t *testing.T) {
	t.Parallel()

	_, _, err := responsesWebSocketValidationPayload([]byte(`{"type":"response.create","input":[]}`), "")

	require.ErrorContains(t, err, "model")
}

func TestResponsesWebSocketValidationPayload_rejects_model_switch(t *testing.T) {
	t.Parallel()

	_, _, err := responsesWebSocketValidationPayload([]byte(`{"type":"response.create","model":"gpt-5.5"}`), "gpt-5.4")

	require.ErrorContains(t, err, "不能切换")
}

func TestResponsesWebSocketValidationPayload_rejects_non_string_model(t *testing.T) {
	t.Parallel()

	_, _, err := responsesWebSocketValidationPayload([]byte(`{"type":"response.create","model":42}`), "")

	require.ErrorContains(t, err, "must be a string")
}

func TestResponsesWebSocket_rejects_non_create_first_event_after_upgrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/responses", ResponsesWebSocket)
	server := httptest.NewServer(router)
	defer server.Close()

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil)
	require.NoError(t, err)
	defer client.Close()

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.cancel"}`)))
	_, payload, err := client.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "error", gjson.GetBytes(payload, "type").String())
	require.Equal(t, "invalid_request", gjson.GetBytes(payload, "error.code").String())

	_, _, err = client.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.ClosePolicyViolation, closeErr.Code)
}

func TestResponsesWebSocket_negotiates_permessage_deflate_with_client(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/v1/responses", ResponsesWebSocket)
	server := httptest.NewServer(router)
	defer server.Close()

	dialer := websocket.Dialer{EnableCompression: true}
	client, response, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/v1/responses", nil)
	require.NoError(t, err)
	defer client.Close()

	require.Contains(t, response.Header.Get("Sec-WebSocket-Extensions"), "permessage-deflate")
}

func TestTruncateResponsesWebSocketCloseReason_preserves_utf8_byte_limit(t *testing.T) {
	t.Parallel()

	got := truncateResponsesWebSocketCloseReason(strings.Repeat("界", 100))

	require.LessOrEqual(t, len(got), responsesWebSocketCloseMax)
	require.True(t, utf8.ValidString(got))
}

func TestResponsesWebSocketCloseCode_filters_reserved_codes(t *testing.T) {
	t.Parallel()

	for _, code := range []int{999, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure, websocket.CloseTLSHandshake, 5000} {
		require.Equal(t, websocket.CloseInternalServerErr, responsesWebSocketCloseCode(code), "code %d", code)
	}
	require.Equal(t, websocket.CloseNormalClosure, responsesWebSocketCloseCode(websocket.CloseNormalClosure))
	require.Equal(t, 4000, responsesWebSocketCloseCode(4000))
}

func TestResponsesWebSocketCloseClassification(t *testing.T) {
	t.Parallel()

	require.True(t, isNormalResponsesWebSocketClose(&websocket.CloseError{Code: websocket.CloseNormalClosure}))
	require.True(t, isNormalResponsesWebSocketClose(context.Canceled))
	require.False(t, isNormalResponsesWebSocketClose(errors.New("connection reset")))
}

func TestResponsesWebSocketFailureChannel_skips_peer_and_context_closes(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{Id: 7}
	require.Nil(t, responsesWebSocketFailureChannel(channel, context.Canceled))
	require.Nil(t, responsesWebSocketFailureChannel(channel, &websocket.CloseError{Code: websocket.CloseNormalClosure}))
	require.Nil(t, responsesWebSocketFailureChannel(channel, &websocket.CloseError{Code: 4001}))
	require.Same(t, channel, responsesWebSocketFailureChannel(channel, &websocket.CloseError{Code: websocket.CloseAbnormalClosure}))
	require.Same(t, channel, responsesWebSocketFailureChannel(channel, errors.New("connection reset")))
}
