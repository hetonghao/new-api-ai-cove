package controller

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/logger"
)

const responsesWebSocketObservabilityKey = "responses_websocket_observability"

type responsesWebSocketQueueStats struct {
	messages atomic.Int64
	bytes    atomic.Int64
	capacity int
}

func newResponsesWebSocketQueueStats(capacity int) *responsesWebSocketQueueStats {
	return &responsesWebSocketQueueStats{capacity: capacity}
}

func (stats *responsesWebSocketQueueStats) enqueue(payloadBytes int) {
	stats.messages.Add(1)
	stats.bytes.Add(int64(payloadBytes))
}

func (stats *responsesWebSocketQueueStats) dequeue(payloadBytes int) {
	stats.messages.Add(-1)
	stats.bytes.Add(-int64(payloadBytes))
}

type responsesWebSocketRuntimeCounters struct {
	opened  atomic.Uint64
	closed  atomic.Uint64
	cleanup atomic.Uint64
}

var responsesWebSocketRuntime responsesWebSocketRuntimeCounters

type responsesWebSocketObservability struct {
	mu sync.Mutex

	trace          string
	requestState   string
	downstreamOrd  uint64
	upstreamTrace  string
	upstreamGen    uint64
	upstreamOrd    uint64
	terminalSeen   bool
	failureAccount bool
	cleanupDone    bool
	failureReason  string

	lastApplicationRx time.Time
	upstreamQueue     *responsesWebSocketQueueStats
}

type responsesWebSocketObservabilitySnapshot struct {
	Trace                     string
	DownstreamOrdinal         uint64
	UpstreamTrace             string
	UpstreamGeneration        uint64
	UpstreamOrdinal           uint64
	RequestState              string
	TerminalSeen              bool
	CleanupOrFailureAccounted bool
	FailureReason             string
	LastApplicationRxAgeMs    int64
	UpstreamQueueMessages     int64
	UpstreamQueueBytes        int64
	UpstreamQueueCapacity     int
	Opened                    uint64
	Closed                    uint64
	Cleanup                   uint64
}

func newResponsesWebSocketObservability(trace string) *responsesWebSocketObservability {
	responsesWebSocketRuntime.opened.Add(1)
	return &responsesWebSocketObservability{
		trace:         trace,
		requestState:  "idle",
		upstreamQueue: newResponsesWebSocketQueueStats(responsesWebSocketQueueSize),
	}
}

func (obs *responsesWebSocketObservability) acceptResponseCreate() {
	obs.mu.Lock()
	defer obs.mu.Unlock()
	obs.downstreamOrd++
	obs.requestState = "active"
	obs.terminalSeen = false
	obs.failureAccount = false
	obs.failureReason = ""
}

func (obs *responsesWebSocketObservability) upstreamDial(trace string) {
	obs.mu.Lock()
	defer obs.mu.Unlock()
	obs.upstreamGen++
	obs.upstreamOrd = 0
	obs.upstreamTrace = trace
	obs.upstreamQueue = newResponsesWebSocketQueueStats(responsesWebSocketQueueSize)
}

func (obs *responsesWebSocketObservability) upstreamQueueStats() *responsesWebSocketQueueStats {
	obs.mu.Lock()
	defer obs.mu.Unlock()
	return obs.upstreamQueue
}

func (obs *responsesWebSocketObservability) commitUpstreamRequest() {
	obs.mu.Lock()
	defer obs.mu.Unlock()
	obs.upstreamOrd++
}

func (obs *responsesWebSocketObservability) markApplicationRx() {
	obs.mu.Lock()
	obs.lastApplicationRx = time.Now()
	obs.mu.Unlock()
}

func (obs *responsesWebSocketObservability) markTerminal() {
	obs.mu.Lock()
	obs.terminalSeen = true
	obs.requestState = "idle"
	obs.mu.Unlock()
}

func (obs *responsesWebSocketObservability) markFailure(reason string) {
	obs.mu.Lock()
	if !obs.failureAccount {
		obs.failureReason = reason
	}
	obs.failureAccount = true
	obs.requestState = "idle"
	obs.mu.Unlock()
}

func (obs *responsesWebSocketObservability) markCleanup() {
	obs.mu.Lock()
	if obs.cleanupDone {
		obs.mu.Unlock()
		return
	}
	obs.cleanupDone = true
	obs.failureAccount = true
	obs.requestState = "idle"
	obs.mu.Unlock()
	responsesWebSocketRuntime.closed.Add(1)
	responsesWebSocketRuntime.cleanup.Add(1)
}

func (obs *responsesWebSocketObservability) snapshot(now time.Time) responsesWebSocketObservabilitySnapshot {
	obs.mu.Lock()
	defer obs.mu.Unlock()
	queueMessages := int64(0)
	queueBytes := int64(0)
	queueCapacity := 0
	if obs.upstreamQueue != nil {
		queueMessages = obs.upstreamQueue.messages.Load()
		queueBytes = obs.upstreamQueue.bytes.Load()
		queueCapacity = obs.upstreamQueue.capacity
	}
	return responsesWebSocketObservabilitySnapshot{
		Trace:                     obs.trace,
		DownstreamOrdinal:         obs.downstreamOrd,
		UpstreamTrace:             obs.upstreamTrace,
		UpstreamGeneration:        obs.upstreamGen,
		UpstreamOrdinal:           obs.upstreamOrd,
		RequestState:              obs.requestState,
		TerminalSeen:              obs.terminalSeen,
		CleanupOrFailureAccounted: obs.failureAccount || obs.cleanupDone,
		FailureReason:             obs.failureReason,
		LastApplicationRxAgeMs:    responsesWebSocketAgeMs(now, obs.lastApplicationRx),
		UpstreamQueueMessages:     queueMessages,
		UpstreamQueueBytes:        queueBytes,
		UpstreamQueueCapacity:     queueCapacity,
		Opened:                    responsesWebSocketRuntime.opened.Load(),
		Closed:                    responsesWebSocketRuntime.closed.Load(),
		Cleanup:                   responsesWebSocketRuntime.cleanup.Load(),
	}
}

func responsesWebSocketAgeMs(now, instant time.Time) int64 {
	if instant.IsZero() {
		return -1
	}
	return now.Sub(instant).Milliseconds()
}

func (obs *responsesWebSocketObservability) log(ctx context.Context, event string) {
	snapshot := obs.snapshot(time.Now())
	logger.LogInfo(ctx, "responses websocket observability", map[string]any{
		"event":                        event,
		"downstream_trace":             snapshot.Trace,
		"downstream_ordinal":           snapshot.DownstreamOrdinal,
		"upstream_trace":               snapshot.UpstreamTrace,
		"upstream_generation":          snapshot.UpstreamGeneration,
		"upstream_ordinal":             snapshot.UpstreamOrdinal,
		"request_state":                snapshot.RequestState,
		"terminal_seen":                snapshot.TerminalSeen,
		"cleanup_or_failure_accounted": snapshot.CleanupOrFailureAccounted,
		"failure_reason":               snapshot.FailureReason,
		"last_application_rx_age_ms":   snapshot.LastApplicationRxAgeMs,
		"upstream_queue_messages":      snapshot.UpstreamQueueMessages,
		"upstream_queue_bytes":         snapshot.UpstreamQueueBytes,
		"upstream_queue_capacity":      snapshot.UpstreamQueueCapacity,
		"opened":                       snapshot.Opened,
		"closed":                       snapshot.Closed,
		"cleanup":                      snapshot.Cleanup,
	})
}
