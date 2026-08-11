// allow: SIZE_OK -- moderation execution is one request-scoped state machine and must remain traceable end to end.
package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/model"
)

const (
	// RiskModerationCloudflareFullReviewChunkRunes is AI Cove's conservative Cloudflare full-review chunk cap.
	RiskModerationCloudflareFullReviewChunkRunes = 16000
)

var (
	ErrInvalidRiskModerationInput        = errors.New("invalid risk moderation input")
	ErrRiskModerationCallerCanceled      = errors.New("risk moderation caller canceled")
	ErrRiskModerationProvider            = errors.New("risk moderation provider failed")
	ErrRiskModerationNoAvailableProvider = errors.New("no available risk moderation provider")
)

type RiskModerationInput struct {
	Provider   *model.RiskProvider
	Providers  []*model.RiskProvider
	Content    string
	ReviewMode model.RiskReviewMode
	// FullReviewChunkRunes overrides the provider adapter default when positive.
	FullReviewChunkRunes int
}

type RiskModerationOutcome struct {
	Result         RiskReviewResult
	Chunks         []RiskReviewChunkAudit
	Source         RiskReviewSource
	CacheHit       bool
	ProviderCalled bool
}

type RiskReviewChunkAudit struct {
	Index      int
	Status     RiskReviewStatus
	Categories []string
	LatencyMS  int64
	Usage      RiskReviewUsage
}

type riskProviderReviewer func(context.Context, *model.RiskProvider, string) (RiskReviewResult, error)

type riskModerationExecutorDeps struct {
	Cache    *RiskReviewCacheService
	Reviewer riskProviderReviewer
	Now      func() time.Time
}

type RiskModerationExecutor struct {
	cache    *RiskReviewCacheService
	reviewer riskProviderReviewer
	circuit  *riskModerationCircuit
}

func NewRiskModerationExecutor() *RiskModerationExecutor {
	executor := newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: NewRiskReviewCacheService(), Reviewer: ReviewRiskContentWithBudget, Now: time.Now,
	})
	executor.circuit = riskModerationProductionCircuit
	return executor
}

func newRiskModerationExecutor(deps riskModerationExecutorDeps) *RiskModerationExecutor {
	return &RiskModerationExecutor{
		cache: deps.Cache, reviewer: deps.Reviewer, circuit: newRiskModerationCircuit(deps.Now),
	}
}

func riskModerationChunkLimit(input RiskModerationInput) (int, error) {
	switch input.ReviewMode {
	case model.RiskReviewSelective:
		return riskExcerptLimit, nil
	case model.RiskReviewFull:
		if input.FullReviewChunkRunes > 0 {
			return input.FullReviewChunkRunes, nil
		}
		if input.FullReviewChunkRunes < 0 {
			return 0, fmt.Errorf("%w: full review chunk limit", ErrInvalidRiskModerationInput)
		}
		return RiskModerationCloudflareFullReviewChunkRunes, nil
	default:
		return 0, fmt.Errorf("%w: review mode", ErrInvalidRiskModerationInput)
	}
}

func (e *RiskModerationExecutor) Execute(ctx context.Context, input RiskModerationInput) (RiskModerationOutcome, error) {
	if e == nil || e.cache == nil || e.reviewer == nil || e.circuit == nil || NormalizeRiskText(input.Content) == "" {
		return RiskModerationOutcome{}, ErrInvalidRiskModerationInput
	}
	providers, providersErr := riskModerationProviders(input)
	if providersErr != nil {
		return RiskModerationOutcome{}, providersErr
	}
	for _, provider := range providers {
		if provider.TimeoutMs <= 0 || provider.FailureThreshold <= 0 || provider.CooldownSeconds <= 0 {
			return RiskModerationOutcome{}, ErrInvalidRiskModerationInput
		}
	}
	policyInput := input
	policyInput.Provider = nil
	policyInput.Providers = providers
	policyVersion, err := RiskModerationPolicyVersion(policyInput)
	if err != nil {
		return RiskModerationOutcome{}, err
	}
	chunkLimit, err := riskModerationChunkLimit(input)
	if err != nil {
		return RiskModerationOutcome{}, err
	}
	resolvedInput := input
	resolvedInput.FullReviewChunkRunes = chunkLimit
	var providerCalled atomic.Bool
	var providerResult atomic.Pointer[RiskReviewResult]
	var providerChunks atomic.Pointer[[]RiskReviewChunkAudit]
	cacheOutcome, reviewErr := e.cache.Review(ctx, RiskReviewCacheInput{
		Content: input.Content, PolicyVersion: policyVersion,
	}, func(reviewParent context.Context) (RiskReviewResult, error) {
		circuitUnavailable := false
		otherUnavailable := false
		for _, tier := range riskModerationProviderTiers(providers) {
			cursor, cursorErr := loadRiskModerationProviderCursor(reviewParent, policyVersion, tier.priority, len(tier.providers))
			if cursorErr != nil {
				cursor = riskModerationProviderCursor{}
			}
			startIndex := cursor.index(len(tier.providers))
			tried := 0
			for tried < len(tier.providers) {
				provider := tier.providers[(startIndex+tried)%len(tier.providers)]
				permit, allowErr := e.circuit.Allow(
					reviewParent,
					riskModerationProviderCircuitKey(provider),
					provider.FailureThreshold,
					time.Duration(provider.CooldownSeconds)*time.Second,
				)
				if errors.Is(allowErr, ErrRiskModerationCircuitOpen) {
					circuitUnavailable = true
					tried++
					continue
				}
				if allowErr != nil {
					return RiskReviewResult{}, allowErr
				}
				if err := reviewParent.Err(); err != nil {
					e.circuit.Abandon(context.Background(), permit)
					return RiskReviewResult{}, err
				}
				advanced, advanceErr := cursor.advance(reviewParent)
				if advanceErr != nil {
					e.circuit.Abandon(context.Background(), permit)
					if err := reviewParent.Err(); err != nil {
						return RiskReviewResult{}, err
					}
					cursor = riskModerationProviderCursor{}
					continue
				}
				if !advanced {
					e.circuit.Abandon(context.Background(), permit)
					cursor, cursorErr = loadRiskModerationProviderCursor(reviewParent, policyVersion, tier.priority, len(tier.providers))
					if cursorErr != nil {
						cursor = riskModerationProviderCursor{}
					}
					continue
				}
				selectedInput := resolvedInput
				selectedInput.Provider = provider
				selectedInput.Providers = nil
				result, chunks, providerErr := e.executeProviderReview(reviewParent, selectedInput, &providerCalled)
				called := providerCalled.Load()
				if called {
					result = riskReviewResultWithProvider(result, provider)
					providerResult.Store(&result)
					providerChunks.Store(&chunks)
				}
				if isRiskProviderLocalBudgetUnavailable(providerErr) {
					e.circuit.Abandon(context.Background(), permit)
					if called {
						return result, providerErr
					}
					otherUnavailable = true
					tried++
					continue
				}
				if providerErr != nil {
					if ctx.Err() != nil {
						e.circuit.Abandon(context.Background(), permit)
					} else {
						e.circuit.Failure(context.Background(), permit)
					}
					return result, providerErr
				}
				e.circuit.Success(context.Background(), permit)
				return result, nil
			}
		}
		if circuitUnavailable && !otherUnavailable {
			return RiskReviewResult{}, ErrRiskModerationCircuitOpen
		}
		return RiskReviewResult{}, ErrRiskModerationNoAvailableProvider
	})
	if reviewErr != nil && ctx.Err() != nil {
		reviewErr = fmt.Errorf("%w: %w", ErrRiskModerationCallerCanceled, reviewErr)
	}
	called := providerCalled.Load()
	source := cacheOutcome.Source
	if source == RiskReviewSourceProvider && !called {
		source = ""
	}
	if source == "" && (called || reviewErr != nil) {
		if called {
			source = RiskReviewSourceProvider
		}
	}
	chunks := []RiskReviewChunkAudit(nil)
	if captured := providerChunks.Load(); captured != nil {
		chunks = cloneRiskReviewChunkAudits(*captured)
	}
	result := cacheOutcome.Result
	if source == RiskReviewSourceProvider {
		if actual := providerResult.Load(); actual != nil {
			result = cloneRiskReviewResult(*actual)
		}
	}
	return RiskModerationOutcome{
		Result: result, Chunks: chunks, Source: source,
		CacheHit:       cacheOutcome.Source == RiskReviewSourceCache,
		ProviderCalled: called && source == RiskReviewSourceProvider,
	}, reviewErr
}

func (e *RiskModerationExecutor) executeProviderReview(
	ctx context.Context,
	input RiskModerationInput,
	providerCalled *atomic.Bool,
) (RiskReviewResult, []RiskReviewChunkAudit, error) {
	review := func(reviewCtx context.Context, content string) (RiskReviewResult, error) {
		providerCtx, cancel := context.WithTimeout(reviewCtx, time.Duration(input.Provider.TimeoutMs)*time.Millisecond)
		defer cancel()
		if err := providerCtx.Err(); err != nil {
			return RiskReviewResult{}, err
		}
		wasCalled := providerCalled.Load()
		providerCalled.Store(true)
		result, err := e.reviewer(providerCtx, input.Provider, content)
		if isRiskProviderLocalBudgetUnavailable(err) && !wasCalled {
			providerCalled.Store(false)
		}
		return result, err
	}
	if input.ReviewMode == model.RiskReviewSelective {
		result, err := review(ctx, input.Content)
		if err != nil {
			return result, nil, fmt.Errorf("%w: %w", ErrRiskModerationProvider, err)
		}
		if !cacheableRiskReview(result) {
			return result, nil, fmt.Errorf("%w: %w", ErrRiskModerationProvider, ErrInvalidFullReviewStatus)
		}
		return result, nil, nil
	}

	full, err := ReviewFullRiskText(ctx, input.Content, input.FullReviewChunkRunes, review)
	if err != nil {
		return RiskReviewResult{}, nil, err
	}
	result := RiskReviewResult{Status: full.Status, Categories: full.Categories, Usage: full.Usage}
	chunks := make([]RiskReviewChunkAudit, 0, len(full.Chunks))
	for _, chunk := range full.Chunks {
		chunks = append(chunks, RiskReviewChunkAudit{
			Index: chunk.Index, Status: chunk.Status,
			Categories: append([]string(nil), chunk.Categories...),
			LatencyMS:  chunk.LatencyMS, Usage: chunk.Usage,
		})
	}
	if full.Status != RiskReviewError {
		return result, chunks, nil
	}
	for _, chunk := range full.Chunks {
		if chunk.Err != nil {
			return result, chunks, fmt.Errorf("%w: %w", ErrRiskModerationProvider, chunk.Err)
		}
	}
	return result, chunks, ErrRiskModerationProvider
}

func cloneRiskReviewChunkAudits(chunks []RiskReviewChunkAudit) []RiskReviewChunkAudit {
	if chunks == nil {
		return nil
	}
	cloned := make([]RiskReviewChunkAudit, len(chunks))
	for index, chunk := range chunks {
		cloned[index] = chunk
		cloned[index].Categories = append([]string(nil), chunk.Categories...)
	}
	return cloned
}
