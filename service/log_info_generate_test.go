package service

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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
	other := model.NewLogOther()

	AppendRelayTransportLogInfo(ctx, other)

	require.Equal(t, map[string]interface{}{
		"transport":                     "websocket",
		"websocket_upstream_connect_ms": int64(12),
		"websocket_first_event_ms":      int64(23),
		"websocket_first_output_ms":     int64(34),
		"websocket_complete_ms":         int64(45),
		"websocket_close_reason":        "upstream disconnected",
	}, other.Snapshot())
}

func TestTurboCompressionLogIsPerRequest(t *testing.T) {
	ctx, _ := gin.CreateTestContext(nil)
	ctx.Request = &http.Request{Header: http.Header{}}
	ctx.Request.Header.Set("X-AI-Cove-Client", "turbo")
	for _, row := range []struct {
		value string
		count int
		zstd  bool
	}{{"v1/changed/20", 20, true}, {"", 0, false}, {"v1/kept", 0, true}, {"v1/changed/0", 0, true}, {"v1/changed/257", 0, false}} {
		common.SetTurboImageDecision(ctx, []string{row.value})
		ctx.Set(common.TurboWireZstdKey, row.zstd)
		other := model.NewLogOther()
		AppendRelayTransportLogInfo(ctx, other)
		data := other.Snapshot()
		if row.count > 0 {
			require.Equal(t, row.count, data["turbo_images_compressed"])
		} else {
			require.NotContains(t, data, "turbo_images_compressed")
		}
		if row.zstd {
			require.Equal(t, true, data["turbo_zstd"])
		} else {
			require.NotContains(t, data, "turbo_zstd")
		}
	}
}

func TestAppendRelayTransportLogInfoImagineSource(t *testing.T) {
	for _, source := range []string{"imagine", "unknown", ""} {
		t.Run(source, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(nil)
			ctx.Request = &http.Request{Header: http.Header{}}
			ctx.Request.Header.Set("X-AI-Cove-Client", source)
			other := model.NewLogOther()
			AppendRelayTransportLogInfo(ctx, other)
			if source == "imagine" {
				assert.Equal(t, "imagine", other.Snapshot()["client_source"])
			} else {
				assert.NotContains(t, other.Snapshot(), "client_source")
			}
		})
	}
}

func TestAppendRelayTransportLogInfo_appends_turbo_client_source(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(nil)
	ctx.Request = &http.Request{Header: http.Header{
		"X-Ai-Cove-Client":         []string{"turbo"},
		"X-Ai-Cove-Client-Version": []string{"mac/0.1.0-beta.4"},
	}}
	other := model.NewLogOther()

	AppendRelayTransportLogInfo(ctx, other)

	values := other.Snapshot()
	require.Equal(t, "turbo", values["client_source"])
	require.Equal(t, "mac/0.1.0-beta.4", values["client_version"])
}
