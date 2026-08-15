package service

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAppendRelayTransportLogInfo_appends_websocket_lifecycle_fields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	common.SetContextKey(ctx, constant.ContextKeyRelayTransport, "websocket")
	common.SetContextKey(ctx, constant.ContextKeyWebSocketUpstreamConnectMs, int64(12))
	common.SetContextKey(ctx, constant.ContextKeyWebSocketFirstEventMs, int64(23))
	common.SetContextKey(ctx, constant.ContextKeyWebSocketFirstOutputMs, int64(34))
	common.SetContextKey(ctx, constant.ContextKeyWebSocketCompleteMs, int64(45))
	common.SetContextKey(ctx, constant.ContextKeyWebSocketCloseReason, "upstream disconnected")
	other := map[string]interface{}{}

	AppendRelayTransportLogInfo(ctx, other)

	require.Equal(t, map[string]interface{}{
		"transport":                     "websocket",
		"websocket_upstream_connect_ms": int64(12),
		"websocket_first_event_ms":      int64(23),
		"websocket_first_output_ms":     int64(34),
		"websocket_complete_ms":         int64(45),
		"websocket_close_reason":        "upstream disconnected",
	}, other)
}

func TestAppendRelayTransportLogInfo_appends_turbo_client_source(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	ctx.Request = &http.Request{Header: http.Header{
		"X-Ai-Cove-Client":         []string{"turbo"},
		"X-Ai-Cove-Client-Version": []string{"mac/0.1.0-beta.4"},
	}}
	other := map[string]interface{}{}

	AppendRelayTransportLogInfo(ctx, other)

	require.Equal(t, "turbo", other["client_source"])
	require.Equal(t, "mac/0.1.0-beta.4", other["client_version"])
}
