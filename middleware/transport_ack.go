package middleware

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

type transportAckCountingBody struct {
	io.ReadCloser
	metrics *common.TransportAckReadMetrics
}

func (b *transportAckCountingBody) Read(p []byte) (int, error) {
	started := time.Now()
	n, err := b.ReadCloser.Read(p)
	b.metrics.RawBytes += n
	b.metrics.RawReadDuration += time.Since(started)
	return n, err
}

func TransportAckRequestBodyLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path != constant.TransportAckPath || c.Request.Method != http.MethodPost || c.Request.Body == nil {
			c.Next()
			return
		}
		encoding := strings.ToLower(strings.TrimSpace(c.GetHeader("Content-Encoding")))
		switch encoding {
		case "", "identity", "gzip", "br", "zstd":
		default:
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		c.Request.Header.Set("Content-Encoding", encoding)

		metrics := &common.TransportAckReadMetrics{}
		c.Set(common.KeyTransportAckReadMetrics, metrics)
		countingBody := &transportAckCountingBody{ReadCloser: c.Request.Body, metrics: metrics}
		c.Request.Body = http.MaxBytesReader(c.Writer, countingBody, constant.TransportAckMaxBytes)
		c.Next()
	}
}
