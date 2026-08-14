package channel_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

func TestDoWssRequestRealtimeIgnoresChannelProxyConfiguration(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeRealtime,
		RelayFormat:    types.RelayFormatOpenAIRealtime,
		RequestURLPath: "/v1/realtime",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelBaseUrl:    server.URL,
			ApiKey:            "test-key",
			UpstreamModelName: "gpt-4o-realtime-preview",
			ChannelSetting: dto.ChannelSettings{
				Proxy: "://invalid-proxy",
			},
		},
	}

	conn, err := channel.DoWssRequest(&openai.Adaptor{}, ctx, info, nil)
	require.NoError(t, err)
	require.NotNil(t, conn)
	require.NoError(t, conn.Close())
	require.True(t, strings.HasPrefix(info.ChannelBaseUrl, "ws://"))
}

func TestDoWssRequestResponsesRequestsPerMessageDeflate(t *testing.T) {
	requestHeaders := make(chan http.Header, 1)
	upgrader := websocket.Upgrader{
		EnableCompression: true,
		CheckOrigin:       func(*http.Request) bool { return true },
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestHeaders <- r.Header.Clone()
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:            relayconstant.RelayModeResponses,
		RelayFormat:          types.RelayFormatOpenAIResponses,
		RequestURLPath:       "/v1/responses",
		IsResponsesWebSocket: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelBaseUrl:    server.URL,
			ApiKey:            "test-key",
			UpstreamModelName: "gpt-4o-mini",
		},
	}

	conn, err := channel.DoWssRequest(&openai.Adaptor{}, ctx, info, nil)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	require.Contains(t, (<-requestHeaders).Get("Sec-WebSocket-Extensions"), "permessage-deflate")
}

func TestDoWssRequestResponsesCapturesServerTraceWithoutForwardingRequestTrace(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get(common.ResponsesWebSocketTraceHeader))
		responseHeader := http.Header{}
		responseHeader.Set(common.ResponsesWebSocketTraceHeader, "0123456789abcdef0123456789abcdef")
		conn, err := upgrader.Upgrade(w, r, responseHeader)
		if err != nil {
			require.NoError(t, err, "upgrade websocket")
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	ctx.Request.Header.Set(common.ResponsesWebSocketTraceHeader, "client-controlled")
	info := &relaycommon.RelayInfo{
		RelayMode:            relayconstant.RelayModeResponses,
		RelayFormat:          types.RelayFormatOpenAIResponses,
		RequestURLPath:       "/v1/responses",
		IsResponsesWebSocket: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelBaseUrl:    server.URL,
			ApiKey:            "test-key",
			UpstreamModelName: "gpt-4o-mini",
		},
	}

	conn, err := channel.DoWssRequest(&openai.Adaptor{}, ctx, info, nil)
	require.NoError(t, err)
	require.Equal(t, "0123456789abcdef0123456789abcdef", ctx.GetString(common.ResponsesWebSocketUpstreamTraceKey))
	require.NoError(t, conn.Close())
}

func TestDoWssRequestResponsesIgnoresInvalidServerTrace(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseHeader := http.Header{}
		responseHeader.Set(common.ResponsesWebSocketTraceHeader, "cpa-trace")
		conn, err := upgrader.Upgrade(w, r, responseHeader)
		if err != nil {
			require.NoError(t, err, "upgrade websocket")
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:            relayconstant.RelayModeResponses,
		RelayFormat:          types.RelayFormatOpenAIResponses,
		RequestURLPath:       "/v1/responses",
		IsResponsesWebSocket: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelBaseUrl:    server.URL,
			ApiKey:            "test-key",
			UpstreamModelName: "gpt-4o-mini",
		},
	}

	conn, err := channel.DoWssRequest(&openai.Adaptor{}, ctx, info, nil)
	require.NoError(t, err)
	require.Empty(t, ctx.GetString(common.ResponsesWebSocketUpstreamTraceKey))
	require.NoError(t, conn.Close())
}
