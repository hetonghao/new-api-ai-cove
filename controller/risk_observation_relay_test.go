package controller

import (
	"bytes"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRelayWithRiskObservation_queues_current_turn_before_relay(t *testing.T) {
	// Given
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-test","messages":[{"role":"user","content":"old"},{"role":"assistant","content":"answer"},{"role":"user","content":"current"}]}`)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(common.RequestIdKey, "req-1")
	ctx.Set("channel_id", 7)
	ctx.Set("channel_name", "CPA-pro")
	ctx.Set("id", 42)
	var job service.RiskObservationJob
	enqueue := func(got service.RiskObservationJob) bool {
		job = got
		return true
	}
	relayCalled := false
	handler := relayWithRiskObservation(types.RelayFormatOpenAI, func(_ *gin.Context, format types.RelayFormat) {
		relayCalled = format == types.RelayFormatOpenAI
	}, enqueue)

	// When
	handler(ctx)

	// Then
	require.True(t, relayCalled)
	require.Equal(t, "current", job.Text)
	require.Equal(t, "req-1", job.RequestID)
	require.Equal(t, 7, job.ChannelID)
	require.Equal(t, "CPA-pro", job.ChannelName)
	require.Equal(t, 42, job.UserID)
}

func TestRelayWithRiskObservation_keeps_invalid_request_on_relay_path(t *testing.T) {
	// Given
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(`{"model":`)))
	ctx.Request.Header.Set("Content-Type", "application/json")
	enqueued := false
	enqueue := func(service.RiskObservationJob) bool {
		enqueued = true
		return true
	}
	relayCalled := false
	handler := relayWithRiskObservation(types.RelayFormatOpenAI, func(_ *gin.Context, _ types.RelayFormat) {
		relayCalled = true
	}, enqueue)

	// When
	handler(ctx)

	// Then
	require.True(t, relayCalled)
	require.False(t, enqueued)
}
