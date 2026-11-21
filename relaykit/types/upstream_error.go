package types

import (
	"fmt"
	"net/http"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

// IsUpstreamHTTP429 uses the original response status, before channel remapping.
func (e *NewAPIError) IsUpstreamHTTP429() bool {
	return e != nil && e.upstreamHTTPStatus == http.StatusTooManyRequests
}

func ErrOptionWithUpstreamHTTPStatusCode(statusCode int) NewAPIErrorOptions {
	return func(e *NewAPIError) {
		e.upstreamHTTPStatus = statusCode
	}
}

func ErrOptionWithUpstreamRejection(message string) NewAPIErrorOptions {
	return func(e *NewAPIError) {
		e.upstreamRejection = message
	}
}

func ErrOptionWithUpstreamError(source *NewAPIError) NewAPIErrorOptions {
	return func(e *NewAPIError) {
		e.upstreamHTTPStatus = source.upstreamHTTPStatus
		e.upstreamRejection = source.upstreamRejection
	}
}

func (e *NewAPIError) MaskSensitiveUpstreamErrorWithStatusCode(upstreamURLs ...string) string {
	if e == nil || !e.IsUpstreamHTTP429() {
		return e.MaskSensitiveErrorWithStatusCode(upstreamURLs...)
	}
	if e.upstreamRejection == "" {
		return fmt.Sprintf("status_code=%d", e.upstreamHTTPStatus)
	}
	return fmt.Sprintf("status_code=%d, %s", e.upstreamHTTPStatus, kitutil.MaskSensitiveInfo(e.upstreamRejection, upstreamURLs...))
}
