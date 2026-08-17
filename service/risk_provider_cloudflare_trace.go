package service

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http/httptrace"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type cloudflareRequestTrace struct {
	startedAt         time.Time
	getConn           atomic.Int64
	gotConn           atomic.Int64
	dnsStart          atomic.Int64
	dnsDone           atomic.Int64
	connectStart      atomic.Int64
	connectDone       atomic.Int64
	tlsStart          atomic.Int64
	tlsDone           atomic.Int64
	wroteRequest      atomic.Int64
	firstResponseByte atomic.Int64
}

func (t *cloudflareRequestTrace) clientTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GetConn: func(string) {
			t.mark(&t.getConn)
		},
		GotConn: func(httptrace.GotConnInfo) {
			t.mark(&t.gotConn)
		},
		DNSStart: func(httptrace.DNSStartInfo) {
			t.mark(&t.dnsStart)
		},
		DNSDone: func(httptrace.DNSDoneInfo) {
			t.mark(&t.dnsDone)
		},
		ConnectStart: func(string, string) {
			t.mark(&t.connectStart)
		},
		ConnectDone: func(string, string, error) {
			t.mark(&t.connectDone)
		},
		TLSHandshakeStart: func() {
			t.mark(&t.tlsStart)
		},
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			t.mark(&t.tlsDone)
		},
		WroteRequest: func(httptrace.WroteRequestInfo) {
			t.mark(&t.wroteRequest)
		},
		GotFirstResponseByte: func() {
			t.mark(&t.firstResponseByte)
		},
	}
}

func (t *cloudflareRequestTrace) mark(event *atomic.Int64) {
	event.CompareAndSwap(0, time.Since(t.startedAt).Milliseconds()+1)
}

func (t *cloudflareRequestTrace) timeoutFields(ctx context.Context, provider *model.RiskProvider, contentRunes int, err error) map[string]any {
	requestID := ""
	if ctx != nil {
		if value := ctx.Value(common.RequestIdKey); value != nil {
			requestID, _ = value.(string)
		}
	}
	finalError := "provider request failed"
	if errors.Is(err, context.DeadlineExceeded) {
		finalError = context.DeadlineExceeded.Error()
	} else if errors.Is(err, context.Canceled) {
		finalError = context.Canceled.Error()
	}
	return map[string]any{
		"request_id":             requestID,
		"provider_type":          provider.ProviderType,
		"provider_id":            provider.Id,
		"timeout_ms":             provider.TimeoutMs,
		"content_runes":          contentRunes,
		"elapsed_ms":             time.Since(t.startedAt).Milliseconds(),
		"get_conn_ms":            traceOffsetMillis(&t.getConn),
		"got_conn_ms":            traceOffsetMillis(&t.gotConn),
		"dns_ms":                 traceDurationMillis(&t.dnsStart, &t.dnsDone),
		"connect_ms":             traceDurationMillis(&t.connectStart, &t.connectDone),
		"tls_ms":                 traceDurationMillis(&t.tlsStart, &t.tlsDone),
		"wrote_request_ms":       traceOffsetMillis(&t.wroteRequest),
		"first_response_byte_ms": traceOffsetMillis(&t.firstResponseByte),
		"final_error":            finalError,
	}
}

func traceOffsetMillis(event *atomic.Int64) int64 {
	value := event.Load()
	if value == 0 {
		return -1
	}
	return value - 1
}

func traceDurationMillis(start, done *atomic.Int64) int64 {
	startMillis := traceOffsetMillis(start)
	doneMillis := traceOffsetMillis(done)
	if startMillis < 0 || doneMillis < 0 {
		return -1
	}
	return max(0, doneMillis-startMillis)
}
