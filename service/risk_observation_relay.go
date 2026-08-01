package service

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/model"
)

type riskModerationRunner interface {
	Execute(context.Context, RiskModerationInput) (RiskModerationOutcome, error)
}

type riskObservationEvaluation struct {
	executor riskModerationRunner
	rules    []*model.RiskRule
}

type riskObservationRelayDeps struct {
	loadPolicy   func() (model.RiskPolicyState, error)
	loadRules    func() ([]*model.RiskRule, error)
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
		loadPolicy: func() (model.RiskPolicyState, error) {
			return model.GetRiskPolicyStateForRelay(job.UserID, job.Model)
		},
		loadRules:    model.GetRiskRules,
		executor:     riskObservationModerationExecutor(),
		enqueueJob:   EnqueueRiskObservation,
		enqueueEvent: EnqueueRiskObservationEvent,
	})
}

func processRiskObservationForRelay(ctx context.Context, job RiskObservationJob, deps riskObservationRelayDeps) RiskObservationRelayDecision {
	if job.Text == "" {
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
	if !state.Enabled || !riskChannelEnabled(state.EnabledChannels, job.ChannelID) || slices.Contains(state.ExcludedUserIDs, job.UserID) || slices.Contains(state.ExcludedModels, job.Model) {
		return RiskObservationRelayDecision{}
	}
	if len(state.ProviderIDs) == 0 {
		event := riskObservationErrorEvent(job, riskObservationProviderConfigError)
		result := deps.enqueueEvent(event)
		decision := RiskObservationRelayDecision{}
		if result.Outcome == RiskObservationEnqueueDirectRecordRequired {
			decision.DirectRecord = &RiskObservationDirectRecord{Event: &event}
		}
		return decision
	}
	loadRules := deps.loadRules
	if loadRules == nil {
		loadRules = model.GetRiskRules
	}
	rules, err := loadRules()
	if err != nil {
		event := riskObservationErrorEvent(job, riskObservationRulesError)
		result := deps.enqueueEvent(event)
		decision := RiskObservationRelayDecision{}
		if result.Outcome == RiskObservationEnqueueDirectRecordRequired {
			decision.DirectRecord = &RiskObservationDirectRecord{Event: &event}
		}
		return decision
	}
	if skipRuleIDs := matchingRiskSkipRuleIDs(job.Text, rules); len(skipRuleIDs) > 0 {
		event := newRiskObservationEvent(job)
		event.Result = RiskObservationNotReviewed
		event.Source = RiskObservationSourceLocal
		event.RuleIDs = skipRuleIDs
		result := deps.enqueueEvent(event)
		decision := RiskObservationRelayDecision{}
		if result.Outcome == RiskObservationEnqueueDirectRecordRequired {
			decision.DirectRecord = &RiskObservationDirectRecord{Event: &event}
		}
		return decision
	}
	job.ProviderIDs = append([]int(nil), state.ProviderIDs...)
	job.ProviderID = 0
	job.ReviewMode = state.ReviewMode
	job.ActionMode = state.ActionMode
	job.NonBlockingCategories = append([]string(nil), state.NonBlockingCategories...)
	if state.ActionMode == model.RiskActionObserve {
		result := deps.enqueueJob(job)
		decision := RiskObservationRelayDecision{}
		if result.Outcome == RiskObservationEnqueueDirectRecordRequired {
			decision.DirectRecord = &RiskObservationDirectRecord{Job: &job, ErrorCode: result.ErrorCode}
		}
		return decision
	}

	event, ok := evaluateRiskObservationWithRules(ctx, job, riskObservationEvaluation{executor: deps.executor, rules: rules})
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
	if len(riskObservationProviderIDs(job)) == 0 || executor == nil {
		return RiskObservationEvent{}, false
	}
	var rules []*model.RiskRule
	if job.ReviewMode == model.RiskReviewSelective {
		var err error
		rules, err = model.GetRiskRules()
		if err != nil {
			event := newRiskObservationEvent(job)
			event.Result = RiskObservationError
			event.ErrorCode = riskObservationRulesError
			event.Source = RiskObservationSourceLocal
			return event, true
		}
	}
	return evaluateRiskObservationWithRules(ctx, job, riskObservationEvaluation{executor: executor, rules: rules})
}

func evaluateRiskObservationWithRules(ctx context.Context, job RiskObservationJob, evaluation riskObservationEvaluation) (RiskObservationEvent, bool) {
	providerIDs := riskObservationProviderIDs(job)
	if len(providerIDs) == 0 || evaluation.executor == nil {
		return RiskObservationEvent{}, false
	}
	event := newRiskObservationEvent(job)
	content := job.Text
	if job.ReviewMode == model.RiskReviewSelective {
		content, event.RuleIDs = BuildSelectiveRiskExcerpt(job.Text, evaluation.rules)
		if content == "" {
			event.Result = RiskObservationNotReviewed
			event.Source = RiskObservationSourceLocal
			return event, true
		}
	}

	providers := make([]*model.RiskProvider, 0, len(providerIDs))
	for _, providerID := range providerIDs {
		provider, err := model.GetRiskProviderByID(providerID)
		if err != nil {
			event.Result = RiskObservationError
			event.ErrorCode = riskObservationProviderConfigError
			event.Source = RiskObservationSourceLocal
			return event, true
		}
		providers = append(providers, provider)
	}
	startedAt := time.Now()
	outcome, executeErr := evaluation.executor.Execute(ctx, RiskModerationInput{
		Providers: providers, Content: content, ReviewMode: job.ReviewMode, FullReviewChunkRunes: 0,
	})
	event.ProviderID = outcome.Result.ProviderID
	event.ProviderName = outcome.Result.ProviderName
	event.ProviderType = outcome.Result.ProviderType
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
		event.ErrorCode, event.ErrorDetail = RiskObservationErrorInfo(executeErr)
		if job.ActionMode == model.RiskActionObserve && ctx.Err() != nil {
			event.ErrorCode = RiskObservationErrorShutdown
			event.ErrorDetail = ""
		}
		return event, true
	}
	event.Blocked, event.NonBlockingMatched = riskObservationDecision(
		job.ActionMode, outcome.Result.Status, outcome.Result.Categories, job.NonBlockingCategories,
	)
	return event, true
}

func riskObservationDecision(
	actionMode model.RiskActionMode,
	status RiskReviewStatus,
	categories []string,
	nonBlockingCategories []string,
) (blocked bool, nonBlockingMatched bool) {
	if actionMode != model.RiskActionBlock || status != RiskReviewUnsafe {
		return false, false
	}
	allowed := make(map[string]struct{}, len(nonBlockingCategories))
	for _, category := range nonBlockingCategories {
		category = normalizeRiskCategory(category)
		if category != "" {
			allowed[category] = struct{}{}
		}
	}
	if len(categories) == 0 {
		return true, false
	}
	for _, category := range categories {
		category = normalizeRiskCategory(category)
		if category == "" {
			return true, false
		}
		if _, ok := allowed[category]; !ok {
			return true, false
		}
	}
	return false, true
}

func normalizeRiskCategory(category string) string {
	return strings.ToLower(strings.TrimSpace(category))
}

func riskObservationProviderIDs(job RiskObservationJob) []int {
	if len(job.ProviderIDs) > 0 {
		return job.ProviderIDs
	}
	if job.ProviderID > 0 {
		return []int{job.ProviderID}
	}
	return nil
}

func matchingRiskSkipRuleIDs(text string, rules []*model.RiskRule) []int {
	normalized := NormalizeRiskText(text)
	if text == "" {
		return nil
	}
	ruleIDs := make([]int, 0)
	for _, rule := range rules {
		if rule == nil || !rule.Enabled || rule.Action != model.RiskRuleActionSkip {
			continue
		}
		matchText := normalized
		if rule.RuleType == model.RiskRuleRegex {
			matchText = text
		}
		if len(riskRuleMatches(matchText, rule)) > 0 {
			ruleIDs = append(ruleIDs, rule.Id)
		}
	}
	return ruleIDs
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
		if executeErr != nil && (outcome.ProviderCalled || outcome.Result.ProviderID > 0) {
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
