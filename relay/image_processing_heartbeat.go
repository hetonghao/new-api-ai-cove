package relay

import (
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
)

const imageProcessingHeartbeatInterval = 60 * time.Second

type unwrapResponseWriter interface {
	Unwrap() http.ResponseWriter
}

func shouldSendImageProcessingHeartbeat(info *relaycommon.RelayInfo, request *dto.ImageRequest) bool {
	if info == nil || request == nil {
		return false
	}
	if info.RelayMode != relayconstant.RelayModeImagesGenerations && info.RelayMode != relayconstant.RelayModeImagesEdits {
		return false
	}
	upstreamModelName := ""
	if info.ChannelMeta != nil {
		upstreamModelName = info.ChannelMeta.UpstreamModelName
	}
	return isGPTImage2Model(info.OriginModelName) ||
		isGPTImage2Model(upstreamModelName) ||
		isGPTImage2Model(request.Model)
}

func isGPTImage2Model(model string) bool {
	return strings.EqualFold(strings.TrimSpace(model), "gpt-image-2")
}

func startProcessingHeartbeat(c *gin.Context, interval time.Duration) func() {
	rawWriter := unwrapHTTPResponseWriter(c.Writer)
	sendProcessingHeartbeat(rawWriter)

	done := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				sendProcessingHeartbeat(rawWriter)
			case <-done:
				return
			case <-c.Request.Context().Done():
				return
			}
		}
	}()

	return func() {
		close(done)
		<-stopped
	}
}

func sendProcessingHeartbeat(writer http.ResponseWriter) {
	writer.Header().Set("Cache-Control", "no-store")
	writer.Header().Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusProcessing)
}

func unwrapHTTPResponseWriter(writer http.ResponseWriter) http.ResponseWriter {
	for {
		unwrapper, ok := writer.(unwrapResponseWriter)
		if !ok {
			return writer
		}
		writer = unwrapper.Unwrap()
	}
}
