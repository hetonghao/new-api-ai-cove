package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
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

func TestResponsesWebSocketEventAllowsCapacityRetryOnlyForEmptyHandshake(t *testing.T) {
	require.True(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"response.created","response":{"output":[],"usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`)))
	require.True(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"codex.rate_limits","rate_limits":{}}`)))
	require.False(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"response.created","response":{"output":[{"type":"function_call"}]}}`)))
	require.False(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"response.in_progress","response":{"usage":{"input_tokens":1}}}`)))
	require.False(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"response.output_text.delta","delta":"hello"}`)))
	require.False(t, responsesWebSocketEventAllowsCapacityRetry([]byte(`{"type":"unknown.event"}`)))
}

func TestResponsesWebSocketCapacityCodeRejectsNonCloseErrors(t *testing.T) {
	_, ok := responsesWebSocketCapacityCode(errors.New("unexpected EOF"))
	require.False(t, ok)
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
	)

	require.Error(t, err)
	require.Equal(t, int32(1), billing.refunds.Load())
	refundResponsesWebSocketBillingIfPending(baseCtx, billing)
	require.Equal(t, int32(1), billing.refunds.Load())
}
