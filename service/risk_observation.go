package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
)

const (
	RiskObservationSafe   RiskObservationResult = "safe"
	RiskObservationUnsafe RiskObservationResult = "unsafe"
	RiskObservationError  RiskObservationResult = "error"

	riskObservationQueueCapacity = 64
	riskObservationPolicyError   = "policy_error"
	riskObservationRulesError    = "rules_error"
	riskObservationProviderError = "provider_error"
)

type RiskObservationResult string

type RiskObservationEvent struct {
	RequestID        string
	ChannelID        int
	UserID           int
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

func CloseRiskObservationQueue(ctx context.Context) {
	riskObservationQueue.Close(ctx)
}

func processRiskObservation(ctx context.Context, job RiskObservationJob) {
	state, err := model.GetRiskPolicyState()
	if err != nil {
		recordRiskObservationEvent(ctx, riskObservationErrorEvent(job, riskObservationPolicyError))
		return
	}
	if !state.Enabled || state.ActionMode != model.RiskActionObserve || !riskChannelEnabled(state.EnabledChannels, job.ChannelName) {
		return
	}

	content := job.Text
	var ruleIDs []int
	if state.ReviewMode == model.RiskReviewSelective {
		rules, listErr := model.GetRiskRules()
		if listErr != nil {
			recordRiskObservationEvent(ctx, riskObservationErrorEvent(job, riskObservationRulesError))
			return
		}
		content, ruleIDs = BuildSelectiveRiskExcerpt(job.Text, rules)
		if content == "" {
			return
		}
	}
	if content == "" || state.ProviderID == nil {
		return
	}

	provider, err := model.GetRiskProviderByID(*state.ProviderID)
	if err != nil {
		event := riskObservationErrorEvent(job, riskObservationProviderError)
		event.ProviderID = *state.ProviderID
		event.RuleIDs = ruleIDs
		recordRiskObservationEvent(ctx, event)
		return
	}

	startedAt := time.Now()
	result, err := ReviewRiskContent(ctx, provider, content)
	event := RiskObservationEvent{
		RequestID: job.RequestID, ChannelID: job.ChannelID, UserID: job.UserID,
		RuleIDs: append([]int(nil), ruleIDs...), ProviderID: provider.Id, ProviderName: provider.Name,
		LatencyMS: time.Since(startedAt).Milliseconds(), ObservedAt: time.Now().UTC(),
	}
	if err != nil {
		event.Result = RiskObservationError
		event.ErrorCode = riskObservationProviderError
		if ctx.Err() != nil {
			event.ErrorCode = RiskObservationErrorShutdown
		}
		recordRiskObservationEvent(ctx, event)
		return
	}

	event.Result = RiskObservationResult(result.Status)
	event.Categories = append([]string(nil), result.Categories...)
	event.PromptTokens = result.Usage.PromptTokens
	event.CompletionTokens = result.Usage.CompletionTokens
	event.TotalTokens = result.Usage.TotalTokens
	event.Neurons = result.Usage.Neurons
	recordRiskObservationEvent(ctx, event)
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
	return RiskObservationEvent{
		RequestID: job.RequestID, ChannelID: job.ChannelID, UserID: job.UserID,
		Result: RiskObservationError, ErrorCode: code, ObservedAt: time.Now().UTC(),
	}
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
