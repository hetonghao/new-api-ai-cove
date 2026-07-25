package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
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

func EnqueueRiskObservation(job RiskObservationJob) bool {
	return riskObservationQueue.Enqueue(job)
}

func EnqueueRiskObservationEvent(event RiskObservationEvent) bool {
	return riskObservationQueue.EnqueueEvent(event)
}

func CloseRiskObservationQueue(ctx context.Context) {
	riskObservationQueue.Close(ctx)
}

func processRiskObservation(ctx context.Context, job RiskObservationJob) {
	event, ok := evaluateRiskObservation(ctx, job, riskObservationModerationExecutor())
	if ok {
		recordRiskObservationEvent(ctx, event)
	}
}

func riskChannelEnabled(channels []model.RiskChannel, selectedChannel string) bool {
	selectedChannel = strings.TrimSpace(selectedChannel)
	for _, channel := range channels {
		if strings.EqualFold(string(channel), selectedChannel) {
			return true
		}
	}
	return false
}

func recordRiskObservationDegradation(ctx context.Context, job RiskObservationJob, code string) {
	recordRiskObservationEvent(ctx, riskObservationErrorEvent(job, code))
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
	_ = sink.RecordRiskObservation(ctx, event)
}
