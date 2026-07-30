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
	ErrInvalidRiskModerationInput = errors.New("invalid risk moderation input")
	ErrRiskModerationProvider     = errors.New("risk moderation provider failed")
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
	return newRiskModerationExecutor(riskModerationExecutorDeps{
		Cache: NewRiskReviewCacheService(), Reviewer: ReviewRiskContent, Now: time.Now,
	})
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
	providers, providersErr := riskModerationProviders(input)
	if e == nil || e.cache == nil || e.reviewer == nil || e.circuit == nil || providersErr != nil ||
		NormalizeRiskText(input.Content) == "" {
		return RiskModerationOutcome{}, ErrInvalidRiskModerationInput
	}
	maxTimeout := 0
	for _, provider := range providers {
		if provider.TimeoutMs <= 0 || provider.FailureThreshold <= 0 || provider.CooldownSeconds <= 0 {
			return RiskModerationOutcome{}, ErrInvalidRiskModerationInput
		}
		if provider.TimeoutMs > maxTimeout {
			maxTimeout = provider.TimeoutMs
		}
	}
	policyVersion, err := RiskModerationPolicyVersion(input)
	if err != nil {
		return RiskModerationOutcome{}, err
	}
	chunkLimit, err := riskModerationChunkLimit(input)
	if err != nil {
		return RiskModerationOutcome{}, err
	}
	resolvedInput := input
	resolvedInput.FullReviewChunkRunes = chunkLimit
	startedAt := time.Now()
	reviewCtx, cancel := context.WithDeadline(ctx, startedAt.Add(time.Duration(maxTimeout)*time.Millisecond))
	defer cancel()
	var providerCalled atomic.Bool
	var providerChunks atomic.Pointer[[]RiskReviewChunkAudit]
	cacheOutcome, reviewErr := e.cache.Review(reviewCtx, RiskReviewCacheInput{
		Content: input.Content, PolicyVersion: policyVersion,
	}, func(reviewParent context.Context) (RiskReviewResult, error) {
		provider := providers[nextRiskModerationProviderIndex(reviewParent, policyVersion, len(providers))]
		providerVersion, versionErr := RiskModerationPolicyVersion(RiskModerationInput{
			Provider: provider, ReviewMode: input.ReviewMode, FullReviewChunkRunes: chunkLimit,
		})
		if versionErr != nil {
			return RiskReviewResult{}, versionErr
		}
		permit, allowErr := e.circuit.Allow(
			providerVersion,
			provider.FailureThreshold,
			time.Duration(provider.CooldownSeconds)*time.Second,
		)
		if allowErr != nil {
			return riskReviewResultWithProvider(RiskReviewResult{}, provider), allowErr
		}
		providerCtx, providerCancel := context.WithDeadline(reviewParent, startedAt.Add(time.Duration(provider.TimeoutMs)*time.Millisecond))
		defer providerCancel()
		selectedInput := resolvedInput
		selectedInput.Provider = provider
		selectedInput.Providers = nil
		result, chunks, providerErr := e.executeProviderReview(providerCtx, selectedInput, &providerCalled)
		result = riskReviewResultWithProvider(result, provider)
		providerChunks.Store(&chunks)
		if providerErr != nil {
			if ctx.Err() != nil {
				e.circuit.Abandon(permit)
			} else {
				e.circuit.Failure(permit)
			}
			return result, providerErr
		}
		e.circuit.Success(permit)
		return result, nil
	})
	called := providerCalled.Load()
	source := cacheOutcome.Source
	if source == "" && called {
		source = RiskReviewSourceProvider
	}
	chunks := []RiskReviewChunkAudit(nil)
	if captured := providerChunks.Load(); captured != nil {
		chunks = cloneRiskReviewChunkAudits(*captured)
	}
	return RiskModerationOutcome{
		Result: cacheOutcome.Result, Chunks: chunks, Source: source,
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
		if err := reviewCtx.Err(); err != nil {
			return RiskReviewResult{}, err
		}
		providerCalled.Store(true)
		return e.reviewer(reviewCtx, input.Provider, content)
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
