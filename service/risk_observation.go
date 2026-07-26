package service

import (
	"context"
	"sync"
	"time"
)

const (
	RiskObservationNotReviewed RiskObservationResult = "not_reviewed"
	RiskObservationSafe        RiskObservationResult = "safe"
	RiskObservationUnsafe      RiskObservationResult = "unsafe"
	RiskObservationError       RiskObservationResult = "error"

	RiskObservationSourceLocal    RiskObservationSource = "local"
	RiskObservationSourceProvider RiskObservationSource = "provider"
	RiskObservationSourceCache    RiskObservationSource = "cache"
	RiskObservationSourceInflight RiskObservationSource = "inflight"

	riskObservationQueueCapacity       = 64
	riskObservationDirectRecordTimeout = 3 * time.Second
	riskObservationPolicyError         = "policy_error"
	riskObservationRulesError          = "rules_error"
	riskObservationProviderError       = "provider_error"
	riskObservationProviderConfigError = "provider_config_error"
	riskObservationCircuitOpen         = "circuit_open"
)

type (
	RiskObservationResult string
	RiskObservationSource string
)

type RiskObservationEvent struct {
	RequestID        string
	ChannelID        int
	UserID           int
	TokenID          int
	Model            string
	Path             string
	Preview          string
	ContentHash      string
	RuleIDs          []int
	ProviderID       int
	ProviderName     string
	Result           RiskObservationResult
	Categories       []string
	LatencyMS        int64
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Neurons          int64
	Chunks           []RiskReviewChunkAudit
	ErrorCode        string
	Source           RiskObservationSource
	CacheHit         bool
	ProviderCalled   bool
	Blocked          bool
	ObservedAt       time.Time
}

type RiskObservationSink interface {
	RecordRiskObservation(context.Context, RiskObservationEvent) error
}

var (
	riskObservationSinkMu sync.RWMutex
	riskObservationSink   RiskObservationSink = riskObservationModelSink{}
	riskObservationQueue                      = NewRiskObservationQueue(context.Background(), RiskObservationQueueConfig{
		Capacity: riskObservationQueueCapacity,
		Process:  processRiskObservation,
		Record:   recordRiskObservationEvent,
		Degrade:  recordRiskObservationDegradation,
	})
)

func SetRiskObservationSink(sink RiskObservationSink) {
	riskObservationSinkMu.Lock()
	riskObservationSink = sink
	riskObservationSinkMu.Unlock()
}

func EnqueueRiskObservation(job RiskObservationJob) RiskObservationEnqueueResult {
	return riskObservationQueue.Enqueue(job)
}

func EnqueueRiskObservationEvent(event RiskObservationEvent) RiskObservationEnqueueResult {
	return riskObservationQueue.EnqueueEvent(event)
}

func CloseRiskObservationQueue(ctx context.Context) {
	riskObservationQueue.Close(ctx)
}

func processRiskObservation(ctx context.Context, job RiskObservationJob) {
	event, ok := evaluateRiskObservation(ctx, job, riskObservationModerationExecutor())
	if ok {
		RecordRiskObservationEventDirect(ctx, event)
	}
}

func riskChannelEnabled(channels []int, selectedChannel int) bool {
	for _, channel := range channels {
		if channel == selectedChannel {
			return true
		}
	}
	return false
}

func recordRiskObservationDegradation(ctx context.Context, job RiskObservationJob, code string) {
	recordRiskObservationEvent(ctx, riskObservationErrorEvent(job, code))
}

// RecordRiskObservationDegradationDirect persists a caller-owned degradation
// after the upstream path completes, using a live context with a strict deadline.
func RecordRiskObservationDegradationDirect(ctx context.Context, job RiskObservationJob, code string) {
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), riskObservationDirectRecordTimeout)
	defer cancel()
	recordRiskObservationDegradation(recordCtx, job, code)
}

// RecordRiskObservationEventDirect persists a caller-owned completed event
// after the upstream path completes, using a live context with a strict deadline.
func RecordRiskObservationEventDirect(ctx context.Context, event RiskObservationEvent) {
	recordCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), riskObservationDirectRecordTimeout)
	defer cancel()
	recordRiskObservationEvent(recordCtx, event)
}

func riskObservationErrorEvent(job RiskObservationJob, code string) RiskObservationEvent {
	event := newRiskObservationEvent(job)
	event.Result = RiskObservationError
	event.ErrorCode = code
	event.Source = RiskObservationSourceLocal
	return event
}

func recordRiskObservationEvent(ctx context.Context, event RiskObservationEvent) {
	riskObservationSinkMu.RLock()
	sink := riskObservationSink
	riskObservationSinkMu.RUnlock()
	if sink == nil {
		return
	}
	event.RuleIDs = append([]int(nil), event.RuleIDs...)
	event.Categories = append([]string(nil), event.Categories...)
	event.Chunks = cloneRiskReviewChunkAudits(event.Chunks)
	_ = sink.RecordRiskObservation(ctx, event)
}
