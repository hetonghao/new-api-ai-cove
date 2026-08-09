package service

import (
	"crypto/tls"
	"fmt"
	"net/http/httptrace"
	"sync/atomic"
	"time"

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

func (t *cloudflareRequestTrace) timeoutMessage(provider *model.RiskProvider, contentRunes int) string {
	return fmt.Sprintf(
		"risk provider timeout: provider_type=%s provider_id=%d timeout_ms=%d content_runes=%d elapsed_ms=%d get_conn_ms=%d got_conn_ms=%d dns_ms=%d connect_ms=%d tls_ms=%d wrote_request_ms=%d first_response_byte_ms=%d",
		provider.ProviderType,
		provider.Id,
		provider.TimeoutMs,
		contentRunes,
		time.Since(t.startedAt).Milliseconds(),
		traceOffsetMillis(&t.getConn),
		traceOffsetMillis(&t.gotConn),
		traceDurationMillis(&t.dnsStart, &t.dnsDone),
		traceDurationMillis(&t.connectStart, &t.connectDone),
		traceDurationMillis(&t.tlsStart, &t.tlsDone),
		traceOffsetMillis(&t.wroteRequest),
		traceOffsetMillis(&t.firstResponseByte),
	)
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
