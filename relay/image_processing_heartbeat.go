package relay

import (
	"net/http"
	"strings"
	"sync"
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
	var stopOnce sync.Once

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
		stopOnce.Do(func() {
			close(done)
		})
		<-stopped
	}
}

func sendProcessingHeartbeat(writer http.ResponseWriter) {
	header := writer.Header()
	cacheControlValues, hadCacheControl := cloneHeaderValues(header, "Cache-Control")
	xAccelBufferingValues, hadXAccelBuffering := cloneHeaderValues(header, "X-Accel-Buffering")

	header.Set("Cache-Control", "no-store")
	header.Set("X-Accel-Buffering", "no")
	writer.WriteHeader(http.StatusProcessing)
	restoreHeaderValues(header, "Cache-Control", cacheControlValues, hadCacheControl)
	restoreHeaderValues(header, "X-Accel-Buffering", xAccelBufferingValues, hadXAccelBuffering)
}

func cloneHeaderValues(header http.Header, key string) ([]string, bool) {
	values, ok := header[http.CanonicalHeaderKey(key)]
	if !ok {
		return nil, false
	}
	return append([]string(nil), values...), true
}

func restoreHeaderValues(header http.Header, key string, values []string, ok bool) {
	canonicalKey := http.CanonicalHeaderKey(key)
	if !ok {
		delete(header, canonicalKey)
		return
	}
	header[canonicalKey] = values
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
