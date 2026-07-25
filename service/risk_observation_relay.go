package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
)

type riskModerationRunner interface {
	Execute(context.Context, RiskModerationInput) (RiskModerationOutcome, error)
}

type riskObservationRelayDeps struct {
	loadPolicy   func() (model.RiskPolicyState, error)
	executor     riskModerationRunner
	enqueueJob   func(RiskObservationJob) RiskObservationEnqueueResult
	enqueueEvent func(RiskObservationEvent) RiskObservationEnqueueResult
}

type RiskObservationDirectRecord struct {
	Job       *RiskObservationJob
	Event     *RiskObservationEvent
	ErrorCode string
}

type RiskObservationRelayDecision struct {
	Blocked      bool
	DirectRecord *RiskObservationDirectRecord
}

var riskObservationModerationExecutor = sync.OnceValue(func() riskModerationRunner {
	return NewRiskModerationExecutor()
})

func ProcessRiskObservationForRelay(ctx context.Context, job RiskObservationJob) RiskObservationRelayDecision {
	return processRiskObservationForRelay(ctx, job, riskObservationRelayDeps{
		loadPolicy:   model.GetRiskPolicyState,
		executor:     riskObservationModerationExecutor(),
		enqueueJob:   EnqueueRiskObservation,
		enqueueEvent: EnqueueRiskObservationEvent,
	})
}

func processRiskObservationForRelay(ctx context.Context, job RiskObservationJob, deps riskObservationRelayDeps) RiskObservationRelayDecision {
	if !riskChannelEnabled([]model.RiskChannel{model.RiskChannelCPAPro}, job.ChannelName) {
		return RiskObservationRelayDecision{}
	}
	loadPolicy := deps.loadPolicy
	if loadPolicy == nil {
		loadPolicy = model.GetRiskPolicyState
	}
	state, err := loadPolicy()
	if err != nil {
		event := riskObservationErrorEvent(job, riskObservationPolicyError)
		result := deps.enqueueEvent(event)
		decision := RiskObservationRelayDecision{}
		if result.Outcome == RiskObservationEnqueueDirectRecordRequired {
			decision.DirectRecord = &RiskObservationDirectRecord{Event: &event}
		}
		return decision
	}
	if !state.Enabled || !riskChannelEnabled(state.EnabledChannels, job.ChannelName) {
		return RiskObservationRelayDecision{}
	}
	if state.ProviderID == nil {
		event := riskObservationErrorEvent(job, riskObservationProviderConfigError)
		result := deps.enqueueEvent(event)
		decision := RiskObservationRelayDecision{}
		if result.Outcome == RiskObservationEnqueueDirectRecordRequired {
			decision.DirectRecord = &RiskObservationDirectRecord{Event: &event}
		}
		return decision
	}
	job.ProviderID = *state.ProviderID
	job.ReviewMode = state.ReviewMode
	job.ActionMode = state.ActionMode
	if state.ActionMode == model.RiskActionObserve {
		result := deps.enqueueJob(job)
		decision := RiskObservationRelayDecision{}
		if result.Outcome == RiskObservationEnqueueDirectRecordRequired {
			decision.DirectRecord = &RiskObservationDirectRecord{Job: &job, ErrorCode: result.ErrorCode}
		}
		return decision
	}

	event, ok := evaluateRiskObservation(ctx, job, deps.executor)
	if ok {
		result := deps.enqueueEvent(event)
		decision := RiskObservationRelayDecision{Blocked: event.Blocked}
		if result.Outcome == RiskObservationEnqueueDirectRecordRequired {
			decision.DirectRecord = &RiskObservationDirectRecord{Event: &event}
		}
		return decision
	}
	return RiskObservationRelayDecision{}
}

func evaluateRiskObservation(ctx context.Context, job RiskObservationJob, executor riskModerationRunner) (RiskObservationEvent, bool) {
	if job.Text == "" {
		event := newRiskObservationEvent(job)
		event.Result = RiskObservationNotReviewed
		event.Source = RiskObservationSourceLocal
		return event, true
	}
	if job.ProviderID < 1 || executor == nil {
		return RiskObservationEvent{}, false
	}
	event := newRiskObservationEvent(job)
	content := job.Text
	if job.ReviewMode == model.RiskReviewSelective {
		rules, err := model.GetRiskRules()
		if err != nil {
			event.Result = RiskObservationError
			event.ErrorCode = riskObservationRulesError
			event.Source = RiskObservationSourceLocal
			return event, true
		}
		content, event.RuleIDs = BuildSelectiveRiskExcerpt(job.Text, rules)
		if content == "" {
			event.Result = RiskObservationNotReviewed
			event.Source = RiskObservationSourceLocal
			return event, true
		}
	}

	provider, err := model.GetRiskProviderByID(job.ProviderID)
	if err != nil {
		event.Result = RiskObservationError
		event.ErrorCode = riskObservationProviderConfigError
		event.Source = RiskObservationSourceLocal
		return event, true
	}
	startedAt := time.Now()
	outcome, executeErr := executor.Execute(ctx, RiskModerationInput{
		Provider: provider, Content: content, ReviewMode: job.ReviewMode, FullReviewChunkRunes: 0,
	})
	event.ProviderID = provider.Id
	event.ProviderName = provider.Name
	event.Result = RiskObservationResult(outcome.Result.Status)
	event.Categories = append([]string(nil), outcome.Result.Categories...)
	event.LatencyMS = time.Since(startedAt).Milliseconds()
	event.PromptTokens = outcome.Result.Usage.PromptTokens
	event.CompletionTokens = outcome.Result.Usage.CompletionTokens
	event.TotalTokens = outcome.Result.Usage.TotalTokens
	event.Neurons = outcome.Result.Usage.Neurons
	event.Chunks = cloneRiskReviewChunkAudits(outcome.Chunks)
	event.Source = riskObservationSource(outcome, executeErr)
	event.CacheHit = outcome.CacheHit
	event.ProviderCalled = outcome.ProviderCalled
	if executeErr != nil {
		event.Result = RiskObservationError
		event.ErrorCode = riskObservationErrorCode(executeErr)
		if job.ActionMode == model.RiskActionObserve && ctx.Err() != nil {
			event.ErrorCode = RiskObservationErrorShutdown
		}
		return event, true
	}
	event.Blocked = job.ActionMode == model.RiskActionBlock && outcome.Result.Status == RiskReviewUnsafe
	return event, true
}

func riskObservationErrorCode(err error) string {
	if errors.Is(err, ErrRiskModerationCircuitOpen) {
		return riskObservationCircuitOpen
	}
	return riskObservationProviderError
}

func riskObservationSource(outcome RiskModerationOutcome, executeErr error) RiskObservationSource {
	switch outcome.Source {
	case RiskReviewSourceProvider:
		return RiskObservationSourceProvider
	case RiskReviewSourceCache:
		return RiskObservationSourceCache
	case RiskReviewSourceInflight:
		return RiskObservationSourceInflight
	default:
		if executeErr != nil {
			return RiskObservationSourceProvider
		}
		return RiskObservationSourceLocal
	}
}

func newRiskObservationEvent(job RiskObservationJob) RiskObservationEvent {
	metadata := BuildRiskRecordContentMetadata(job.Text)
	return RiskObservationEvent{
		RequestID: job.RequestID, ChannelID: job.ChannelID, UserID: job.UserID, TokenID: job.TokenID,
		Model: job.Model, Path: job.Path, Preview: metadata.Preview, ContentHash: metadata.ContentHash,
		ObservedAt: time.Now().UTC(),
	}
}
