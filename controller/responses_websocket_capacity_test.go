package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

type responsesWebSocketRetryBillingProbe struct {
	pending atomic.Bool
	refunds atomic.Int32
}

func (p *responsesWebSocketRetryBillingProbe) Settle(int) error { return nil }

func (p *responsesWebSocketRetryBillingProbe) Refund(*gin.Context) {
	if p.pending.Swap(false) {
		p.refunds.Add(1)
	}
}

func (p *responsesWebSocketRetryBillingProbe) NeedsRefund() bool {
	return p.pending.Load()
}

func (*responsesWebSocketRetryBillingProbe) GetPreConsumedQuota() int { return 1 }

func (*responsesWebSocketRetryBillingProbe) Reserve(int) error { return nil }

var _ relaycommon.BillingSettler = (*responsesWebSocketRetryBillingProbe)(nil)

func TestResponsesWebSocketCapacityCodeRequiresVersionedBoundedReason(t *testing.T) {
	tests := []struct {
		name   string
		code   int
		reason string
		want   string
	}{
		{name: "server overloaded", code: responsesWebSocketCapacityCloseCode, reason: responsesWebSocketCapacityCloseReason, want: "server_is_overloaded"},
		{name: "slow down", code: responsesWebSocketCapacityCloseCode, reason: responsesWebSocketCapacityReasonPrefix + "slow_down", want: "slow_down"},
		{name: "wrong close code", code: websocket.CloseInternalServerErr, reason: responsesWebSocketCapacityCloseReason},
		{name: "ordinary internal close", code: websocket.CloseInternalServerErr, reason: "ordinary"},
		{name: "ordinary service unavailable close", code: websocket.CloseServiceRestart, reason: "ordinary"},
		{name: "ordinary try again close", code: websocket.CloseTryAgainLater, reason: "ordinary"},
		{name: "unknown version", code: responsesWebSocketCapacityCloseCode, reason: "ai-cove-capacity/v2;state=rejected;phase=pre_output;code=server_is_overloaded"},
		{name: "unknown code", code: responsesWebSocketCapacityCloseCode, reason: responsesWebSocketCapacityReasonPrefix + "quota_exhausted"},
		{name: "oversized reason", code: responsesWebSocketCapacityCloseCode, reason: responsesWebSocketCapacityCloseReason + "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, ok := responsesWebSocketCapacityCode(&websocket.CloseError{Code: tt.code, Text: tt.reason})
			if tt.want == "" {
				require.False(t, ok)
				return
			}
			require.True(t, ok)
			require.Equal(t, tt.want, code)
		})
	}
}

func TestPropagateResponsesWebSocketCapacityCloseMapsToStandardInternalError(t *testing.T) {
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		propagateResponsesWebSocketClose(conn, &websocket.CloseError{Code: responsesWebSocketCapacityCloseCode, Text: responsesWebSocketCapacityCloseReason})
	})
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(upstream.server.URL, "http"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	closeErr := readResponsesWebSocketTestClose(t, client)
	require.Equal(t, websocket.CloseInternalServerErr, closeErr.Code)
	require.Equal(t, "upstream websocket disconnected", closeErr.Text)
}

func TestResponsesWebSocketEventAllowsCapacityRetryOnlyForEmptyHandshake(t *testing.T) {
	require.True(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"response.created","response":{"output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}}`)))
	require.True(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"response.created","response":{"status":"in_progress","output":[],"usage":{"total_tokens":0}}}`)))
	require.True(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"codex.rate_limits","rate_limits":{}}`)))
	require.True(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"codex.response.metadata","metadata":{"provider":"codex"}}`)))
	require.True(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"response.created","response":{"output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0,"input_tokens_details":{"cached_tokens":0},"audio_tokens":{"input":0}}}}`)))
	require.False(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"response.created","response":{"output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0,"input_tokens_details":{"cached_tokens":2}}}}`)))
	require.False(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"response.in_progress","usage":{"reasoning_tokens":1}}`)))
	require.False(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"response.created","response":{"output":[{"type":"function_call"}]}}`)))
	require.False(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"response.in_progress","response":{"usage":{"input_tokens":1}}}`)))
	require.False(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"response.output_text.delta","delta":"hello"}`)))
	require.False(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"unknown.event"}`)))
}

func TestResponsesWebSocketCapacityRetryFailsClosedForUnknownEnvelopeFields(t *testing.T) {
	for _, payload := range []string{
		`{"type":"response.created","response":{"output":[],"server_tool_use":{}}}`,
		`{"type":"response.in_progress","response":{"output":[],"computer_call":{}}}`,
		`{"type":"response.created","unknown":0}`,
		`{"type":"response.created","unknown":null}`,
		`{"type":"response.created","response":{"future_field":{"value":0}}}`,
		`{"type":"response.created","response":{"metadata":{"provider":"codex"}}}`,
		`{"type":"codex.rate_limits","metadata":{"provider":"codex"}}`,
		`{"type":"codex.response.metadata","rate_limits":{"remaining":0}}`,
	} {
		require.False(t, responsesWebSocketEventAllowsCapacityRetry([]byte(payload)), payload)
	}
	require.True(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"codex.rate_limits","rate_limits":{"primary":{"remaining":0}}}`)))
}

func TestResponsesWebSocketApplicationOutputFailsClosedForUnknownEnvelopeFields(t *testing.T) {
	for _, payload := range []string{
		`{"type":"response.failed","response":{"status":"failed","server_tool_use":{}}}`,
		`{"type":"error","computer_call":{}}`,
		`{"type":"response.failed","unknown":0}`,
		`{"type":"response.failed","unknown":null}`,
		`{"type":"response.failed","response":{"future_field":{"value":0}}}`,
		`{"type":"response.failed","response":{"status":"pending"}}`,
		`{"type":"response.failed","response":{"metadata":{"provider":"codex"}}}`,
	} {
		require.True(t, responsesWebSocketEventHasApplicationOutput([]byte(payload)), payload)
	}
	require.False(t, responsesWebSocketEventHasApplicationOutput([]byte(`{"type":"response.failed","response":{"status":"failed"}}`)))
	require.False(t, responsesWebSocketEventHasApplicationOutput([]byte(`{"type":"codex.response.metadata","metadata":{"provider":"codex"}}`)))
	require.False(t, responsesWebSocketEventHasApplicationOutput([]byte(`{"type":"codex.rate_limits","rate_limits":{"primary":{"remaining":0}}}`)))
}

func TestResponsesWebSocketCapacityCodeRejectsNonCloseErrors(t *testing.T) {
	_, ok := responsesWebSocketCapacityCode(errors.New("unexpected EOF"))
	require.False(t, ok)
}

func TestResponsesWebSocketCapacityErrorSkipsRetry(t *testing.T) {
	err := newResponsesWebSocketCapacityError("server_is_overloaded")

	require.True(t, types.IsSkipRetryError(err))
}

func TestResponsesWebSocketCapacityFallbackPreservesCapacityForUpstream429(t *testing.T) {
	for _, code := range []types.ErrorCode{
		types.ErrorCode("usage_limit_reached"),
		types.ErrorCode("rate_limit_exceeded"),
		types.ErrorCodeAuthUnavailable,
		types.ErrorCodeChannelNoAvailableKey,
	} {
		fallback := types.NewErrorWithStatusCode(errors.New(string(code)), code, http.StatusTooManyRequests)
		got := responsesWebSocketCapacityFallbackError("server_is_overloaded", fallback)

		require.Equal(t, types.ErrorCodeServerIsOverloaded, got.GetErrorCode(), code)
	}
}

func TestResponsesWebSocketCapacityFallbackPreservesClient400Error(t *testing.T) {
	fallback := types.NewErrorWithStatusCode(errors.New("invalid request"), types.ErrorCodeInvalidRequest, http.StatusBadRequest)

	got := responsesWebSocketCapacityFallbackError("server_is_overloaded", fallback)

	require.Same(t, fallback, got)
}

func TestResponsesWebSocketRetryPreparationFailureRefundsInheritedBillingOnce(t *testing.T) {
	setupResponsesWebSocketHandlerTest(t)
	gin.SetMode(gin.TestMode)
	baseCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	baseCtx.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	billing := &responsesWebSocketRetryBillingProbe{}
	billing.pending.Store(true)

	_, _, _, _, _, err := prepareFirstResponsesWebSocketRequestWithBilling(
		baseCtx,
		[]byte(`{"type":"not_response.create","model":"gpt-4o-mini"}`),
		time.Now(),
		billing,
		nil,
		nil,
		nil,
		nil,
		0,
		nil,
		true,
	)

	require.Error(t, err)
	require.Equal(t, int32(1), billing.refunds.Load())
	refundResponsesWebSocketBillingIfPending(baseCtx, billing)
	require.Equal(t, int32(1), billing.refunds.Load())
}

func TestResponsesWebSocketRetryIntermediatePreparationKeepsGenericBillingPending(t *testing.T) {
	setupResponsesWebSocketHandlerTest(t)
	gin.SetMode(gin.TestMode)
	baseCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	baseCtx.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	billing := &responsesWebSocketRetryBillingProbe{}
	billing.pending.Store(true)

	_, _, _, _, _, err := prepareFirstResponsesWebSocketRequestWithBilling(
		baseCtx,
		[]byte(`{"type":"not_response.create","model":"gpt-4o-mini"}`),
		time.Now(),
		billing,
		nil,
		nil,
		nil,
		nil,
		0,
		nil,
		false,
	)

	require.Error(t, err)
	require.Zero(t, billing.refunds.Load())
	refundResponsesWebSocketBillingIfPending(baseCtx, billing)
	require.Equal(t, int32(1), billing.refunds.Load())
}
