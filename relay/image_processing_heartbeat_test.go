package relay

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/textproto"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
)

func TestShouldSendImageProcessingHeartbeatForGPTImage2(t *testing.T) {
	tests := []struct {
		name    string
		info    *relaycommon.RelayInfo
		request *dto.ImageRequest
		want    bool
	}{
		{
			name: "origin model",
			info: &relaycommon.RelayInfo{
				RelayMode:       relayconstant.RelayModeImagesGenerations,
				OriginModelName: "gpt-image-2",
			},
			request: &dto.ImageRequest{Model: "gpt-image-2"},
			want:    true,
		},
		{
			name: "mapped upstream model",
			info: &relaycommon.RelayInfo{
				RelayMode:       relayconstant.RelayModeImagesEdits,
				OriginModelName: "customer-image-model",
				ChannelMeta: &relaycommon.ChannelMeta{
					UpstreamModelName: "gpt-image-2",
				},
			},
			request: &dto.ImageRequest{Model: "gpt-image-2"},
			want:    true,
		},
		{
			name: "other image model",
			info: &relaycommon.RelayInfo{
				RelayMode:       relayconstant.RelayModeImagesGenerations,
				OriginModelName: "gpt-image-1",
			},
			request: &dto.ImageRequest{Model: "gpt-image-1"},
			want:    false,
		},
		{
			name: "non image relay mode",
			info: &relaycommon.RelayInfo{
				RelayMode:       relayconstant.RelayModeChatCompletions,
				OriginModelName: "gpt-image-2",
			},
			request: &dto.ImageRequest{Model: "gpt-image-2"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSendImageProcessingHeartbeat(tt.info, tt.request); got != tt.want {
				t.Fatalf("shouldSendImageProcessingHeartbeat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStartProcessingHeartbeatSendsRepeated102UntilStopped(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.GET("/heartbeat", func(c *gin.Context) {
		c.Writer.Header().Set("Cache-Control", "private")
		stop := startProcessingHeartbeat(c, 10*time.Millisecond)
		time.Sleep(25 * time.Millisecond)
		stop()
		stop()
		c.String(http.StatusOK, "ok")
	})

	server := httptest.NewServer(engine)
	defer server.Close()

	interimCount := 0
	trace := &httptrace.ClientTrace{
		Got1xxResponse: func(code int, _ textproto.MIMEHeader) error {
			if code == http.StatusProcessing {
				interimCount++
			}
			return nil
		},
	}

	request, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(context.Background(), trace),
		http.MethodGet,
		server.URL+"/heartbeat",
		nil,
	)
	if err != nil {
		t.Fatalf("NewRequestWithContext returned error: %v", err)
	}

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}

	if response.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, want %d; body=%s", response.StatusCode, http.StatusOK, body)
	}
	if got := response.Header.Get("Cache-Control"); got != "private" {
		t.Fatalf("Cache-Control = %q, want private", got)
	}
	if got := response.Header.Get("X-Accel-Buffering"); got != "" {
		t.Fatalf("X-Accel-Buffering = %q, want empty final header", got)
	}
	if interimCount < 2 {
		t.Fatalf("observed %d interim responses, want at least 2", interimCount)
	}
}
