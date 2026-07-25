package service

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const RiskReviewError RiskReviewStatus = "error"

var (
	ErrInvalidFullReviewChunkLimit = errors.New("full risk review chunk limit must be positive")
	ErrNilFullReviewReviewer       = errors.New("full risk review reviewer is required")
	ErrInvalidFullReviewStatus     = errors.New("invalid full risk review status")
)

type FullRiskReviewChunkResult struct {
	Index      int
	Status     RiskReviewStatus
	Categories []string
	LatencyMS  int64
	Usage      RiskReviewUsage
	Err        error
}

type FullRiskReviewResult struct {
	Status     RiskReviewStatus
	Categories []string
	Usage      RiskReviewUsage
	Chunks     []FullRiskReviewChunkResult
}

type FullRiskReviewer func(context.Context, string) (RiskReviewResult, error)

func ChunkFullRiskText(text string, maxChunkRunes int) ([]string, error) {
	if maxChunkRunes <= 0 {
		return nil, ErrInvalidFullReviewChunkLimit
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return nil, nil
	}
	chunks := make([]string, 0, (len(runes)+maxChunkRunes-1)/maxChunkRunes)
	for start := 0; start < len(runes); start += maxChunkRunes {
		end := min(start+maxChunkRunes, len(runes))
		chunks = append(chunks, string(runes[start:end]))
	}
	return chunks, nil
}

func ReviewFullRiskText(
	ctx context.Context,
	text string,
	maxChunkRunes int,
	reviewer FullRiskReviewer,
) (FullRiskReviewResult, error) {
	chunks, err := ChunkFullRiskText(text, maxChunkRunes)
	if err != nil {
		return FullRiskReviewResult{}, err
	}
	if reviewer == nil {
		return FullRiskReviewResult{}, ErrNilFullReviewReviewer
	}

	result := FullRiskReviewResult{
		Status: RiskReviewSafe,
		Chunks: make([]FullRiskReviewChunkResult, 0, len(chunks)),
	}
	seenCategories := make(map[string]struct{})
	unsafeFound := false
	errorFound := false
	for index, chunk := range chunks {
		chunkResult := FullRiskReviewChunkResult{Index: index}
		if ctxErr := ctx.Err(); ctxErr != nil {
			chunkResult.Status = RiskReviewError
			chunkResult.Err = ctxErr
			errorFound = true
		} else {
			startedAt := time.Now()
			review, reviewErr := reviewer(ctx, chunk)
			chunkResult.LatencyMS = time.Since(startedAt).Milliseconds()
			chunkResult.Categories = append([]string(nil), review.Categories...)
			chunkResult.Usage = review.Usage
			switch {
			case reviewErr != nil:
				chunkResult.Status = RiskReviewError
				chunkResult.Err = reviewErr
				errorFound = true
				unsafeFound = unsafeFound || review.Status == RiskReviewUnsafe
			case review.Status == RiskReviewSafe || review.Status == RiskReviewUnsafe:
				chunkResult.Status = review.Status
				unsafeFound = unsafeFound || review.Status == RiskReviewUnsafe
			default:
				chunkResult.Status = RiskReviewError
				chunkResult.Err = fmt.Errorf("%w: %q", ErrInvalidFullReviewStatus, review.Status)
				errorFound = true
			}
		}

		result.Usage.PromptTokens += chunkResult.Usage.PromptTokens
		result.Usage.CompletionTokens += chunkResult.Usage.CompletionTokens
		result.Usage.TotalTokens += chunkResult.Usage.TotalTokens
		result.Usage.Neurons += chunkResult.Usage.Neurons
		for _, category := range chunkResult.Categories {
			if _, exists := seenCategories[category]; exists {
				continue
			}
			seenCategories[category] = struct{}{}
			result.Categories = append(result.Categories, category)
		}
		result.Chunks = append(result.Chunks, chunkResult)
	}

	if unsafeFound {
		result.Status = RiskReviewUnsafe
	} else if errorFound {
		result.Status = RiskReviewError
	}
	return result, nil
}
