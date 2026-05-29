package controller

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	defaultProcessingProbeDelay = 130 * time.Second
	maxProcessingProbeDelay     = 180 * time.Second
)

type unwrapResponseWriter interface {
	Unwrap() http.ResponseWriter
}

func ProcessingProbe(c *gin.Context) {
	delay := processingProbeDelay(c.Query("delay_ms"))
	writer := c.Writer
	rawWriter := unwrapHTTPResponseWriter(writer)

	rawWriter.Header().Set("Cache-Control", "no-store")
	rawWriter.Header().Set("X-Accel-Buffering", "no")
	rawWriter.WriteHeader(http.StatusProcessing)
	_ = http.NewResponseController(rawWriter).Flush()

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-c.Request.Context().Done():
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":             true,
		"message":             "processing probe complete",
		"sent_interim_status": http.StatusProcessing,
		"delay_ms":            delay.Milliseconds(),
	})
}

func processingProbeDelay(value string) time.Duration {
	if value == "" {
		return defaultProcessingProbeDelay
	}

	delayMs, err := strconv.Atoi(value)
	if err != nil || delayMs < 0 {
		return defaultProcessingProbeDelay
	}

	delay := time.Duration(delayMs) * time.Millisecond
	if delay > maxProcessingProbeDelay {
		return maxProcessingProbeDelay
	}
	return delay
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
