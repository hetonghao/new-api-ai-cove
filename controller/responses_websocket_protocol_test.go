package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesWebSocketPayloadRepairsOfficialOpenAIItemIDs(t *testing.T) {
	const raw = `{"type":"response.create","model":"gpt-6-luna","input":[{"type":"message","id":"item_client","role":"assistant","content":[]}],"vendor_extension":9007199254740993}`
	for _, passthrough := range []bool{false, true} {
		c, _ := gin.CreateTestContext(nil)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(raw))
		t.Cleanup(func() { common.CleanupBodyStorage(c) })
		var request dto.OpenAIResponsesRequest
		require.NoError(t, common.Unmarshal([]byte(raw), &request))
		info := &relaycommon.RelayInfo{
			RelayMode:      relayconstant.RelayModeResponses,
			RequestURLPath: "/v1/responses",
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType: constant.ChannelTypeOpenAI, ApiType: constant.APITypeOpenAI,
				ChannelBaseUrl: "https://api.openai.com", UpstreamModelName: request.Model,
				ChannelSetting: dto.ChannelSettings{PassThroughBodyEnabled: passthrough},
				ParamOverride:  map[string]any{"operations": []any{map[string]any{"path": "input.0.id", "mode": "set", "value": "item_override"}}},
			},
		}
		info.Request, info.OriginModelName = &request, request.Model
		outgoing, _, apiErr := prepareResponsesWebSocketPayload(c, info, &request, []byte(raw))
		require.Nil(t, apiErr)
		expected := "msg_item_override"
		if passthrough {
			expected = "msg_item_client"
		}
		require.Equal(t, expected, gjson.GetBytes(outgoing, "input.0.id").String())
		require.Equal(t, "9007199254740993", gjson.GetBytes(outgoing, "vendor_extension").Raw)
		require.Equal(t, "item_client", gjson.GetBytes(request.Input, "0.id").String())
	}
}

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
	webSocketURL, done := newResponsesWebSocketProtocolTestServer(t)

	client, _, err := websocket.DefaultDialer.Dial(webSocketURL, nil)
	require.NoError(t, err)
	defer closeResponsesWebSocketProtocolTestClient(t, client, done)

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
	webSocketURL, done := newResponsesWebSocketProtocolTestServer(t)

	dialer := websocket.Dialer{EnableCompression: true}
	client, response, err := dialer.Dial(webSocketURL, nil)
	require.NoError(t, err)
	defer closeResponsesWebSocketProtocolTestClient(t, client, done)

	require.Contains(t, response.Header.Get("Sec-WebSocket-Extensions"), "permessage-deflate")
}

func TestResponsesWebSocket_returns_server_trace_only_in_upgrade_response(t *testing.T) {
	webSocketURL, done := newResponsesWebSocketProtocolTestServer(t)
	dialer := websocket.Dialer{}
	client, response, err := dialer.Dial(webSocketURL, http.Header{
		"X-AI-Cove-WS-Trace": []string{"client-controlled"},
	})
	require.NoError(t, err)
	trace := response.Header.Get("X-AI-Cove-WS-Trace")
	require.NotEmpty(t, trace)
	require.Regexp(t, `^[0-9a-f]{32}$`, trace)
	require.NotEqual(t, "client-controlled", trace)
	closeResponsesWebSocketProtocolTestClient(t, client, done)

	secondURL, secondDone := newResponsesWebSocketProtocolTestServer(t)
	secondClient, secondResponse, err := dialer.Dial(secondURL, http.Header{
		"X-AI-Cove-WS-Trace": []string{"another-client-value"},
	})
	require.NoError(t, err)
	secondTrace := secondResponse.Header.Get("X-AI-Cove-WS-Trace")
	require.NotEmpty(t, secondTrace)
	require.Regexp(t, `^[0-9a-f]{32}$`, secondTrace)
	require.NotEqual(t, "another-client-value", secondTrace)
	require.NotEqual(t, trace, secondTrace)
	closeResponsesWebSocketProtocolTestClient(t, secondClient, secondDone)
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

func TestResponsesWebSocketClientDisconnectErrorClassification(t *testing.T) {
	t.Parallel()

	require.True(t, isResponsesWebSocketClientDisconnectError(&websocket.CloseError{Code: websocket.CloseAbnormalClosure}))
	require.True(t, isResponsesWebSocketClientDisconnectError(&websocket.CloseError{Code: websocket.CloseNormalClosure}))
	require.False(t, isResponsesWebSocketClientDisconnectError(errors.New("connection reset")))
	require.False(t, isResponsesWebSocketClientDisconnectError(&responsesWebSocketPrivateProtocolError{
		closeCode: websocket.CloseMessageTooBig,
		reason:    "private message exceeds configured limit",
	}))
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
