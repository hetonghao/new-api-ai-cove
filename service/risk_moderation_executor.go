package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	riskModerationPromptSemantics         = "cloudflare-user-message-max16-temp0-v1"
	riskModerationClassificationSemantics = "safe-unsafe-error-unsafe-first-v1"
	// RiskModerationCloudflareFullReviewChunkRunes is AI Cove's conservative Cloudflare full-review chunk cap.
	RiskModerationCloudflareFullReviewChunkRunes = 16000
)

var (
	ErrInvalidRiskModerationInput = errors.New("invalid risk moderation input")
	ErrRiskModerationProvider     = errors.New("risk moderation provider failed")
)

type RiskModerationInput struct {
	Provider   *model.RiskProvider
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

func RiskModerationPolicyVersion(input RiskModerationInput) (string, error) {
	if input.Provider == nil {
		return "", ErrInvalidRiskModerationInput
	}
	chunkLimit, err := riskModerationChunkLimit(input)
	if err != nil {
		return "", err
	}
	provider := input.Provider
	accountID := ""
	channelID := 0
	promptSemantics := riskModerationPromptSemantics
	switch provider.ProviderType {
	case model.RiskProviderCloudflare:
		accountID, err = provider.CloudflareAccountID()
		if err != nil {
			return "", fmt.Errorf("resolve risk provider account ID: %w", err)
		}
	case model.RiskProviderPlatformInternal:
		channelID = provider.ChannelID
		promptSemantics = platformInternalRiskPromptSemantics
	default:
		return "", fmt.Errorf("%w: provider type", ErrInvalidRiskModerationInput)
	}
	material, err := common.Marshal(struct {
		ProviderID              int                    `json:"provider_id"`
		ProviderType            model.RiskProviderType `json:"provider_type"`
		AccountID               string                 `json:"account_id"`
		ChannelID               int                    `json:"channel_id"`
		Model                   string                 `json:"model"`
		ReviewMode              model.RiskReviewMode   `json:"review_mode"`
		ChunkLimit              int                    `json:"chunk_limit"`
		PromptSemantics         string                 `json:"prompt_semantics"`
		ClassificationSemantics string                 `json:"classification_semantics"`
	}{
		ProviderID: provider.Id, ProviderType: provider.ProviderType,
		AccountID: accountID, ChannelID: channelID, Model: provider.Model,
		ReviewMode: input.ReviewMode, ChunkLimit: chunkLimit,
		PromptSemantics:         promptSemantics,
		ClassificationSemantics: riskModerationClassificationSemantics,
	})
	if err != nil {
		return "", fmt.Errorf("encode risk moderation policy: %w", err)
	}
	return hex.EncodeToString(common.Sha256Raw(material)), nil
}

func riskModerationChunkLimit(input RiskModerationInput) (int, error) {
	switch input.ReviewMode {
	case model.RiskReviewSelective:
		return riskExcerptLimit, nil
	case model.RiskReviewFull:
		if input.FullReviewChunkRunes > 0 {
			return input.FullReviewChunkRunes, nil
		}
		if input.FullReviewChunkRunes < 0 || input.Provider == nil ||
			(input.Provider.ProviderType != model.RiskProviderCloudflare && input.Provider.ProviderType != model.RiskProviderPlatformInternal) {
			return 0, fmt.Errorf("%w: full review chunk limit", ErrInvalidRiskModerationInput)
		}
		return RiskModerationCloudflareFullReviewChunkRunes, nil
	default:
		return 0, fmt.Errorf("%w: review mode", ErrInvalidRiskModerationInput)
	}
}

func (e *RiskModerationExecutor) Execute(ctx context.Context, input RiskModerationInput) (RiskModerationOutcome, error) {
	if e == nil || e.cache == nil || e.reviewer == nil || e.circuit == nil || input.Provider == nil ||
		NormalizeRiskText(input.Content) == "" || input.Provider.TimeoutMs <= 0 ||
		input.Provider.FailureThreshold <= 0 || input.Provider.CooldownSeconds <= 0 {
		return RiskModerationOutcome{}, ErrInvalidRiskModerationInput
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
	reviewCtx, cancel := context.WithTimeout(ctx, time.Duration(input.Provider.TimeoutMs)*time.Millisecond)
	defer cancel()
	var providerCalled atomic.Bool
	var providerChunks atomic.Pointer[[]RiskReviewChunkAudit]
	cacheOutcome, reviewErr := e.cache.Review(reviewCtx, RiskReviewCacheInput{
		Content: input.Content, PolicyVersion: policyVersion,
	}, func(reviewParent context.Context) (RiskReviewResult, error) {
		permit, allowErr := e.circuit.Allow(
			policyVersion,
			input.Provider.FailureThreshold,
			time.Duration(input.Provider.CooldownSeconds)*time.Second,
		)
		if allowErr != nil {
			return RiskReviewResult{}, allowErr
		}
		result, chunks, providerErr := e.executeProviderReview(reviewParent, resolvedInput, &providerCalled)
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
