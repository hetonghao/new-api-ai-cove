package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestChunkFullRiskText_preserves_exact_text_at_unicode_boundaries(t *testing.T) {
	tests := []struct {
		name string
		text string
		max  int
		want []string
	}{
		{name: "empty", text: "", max: 2, want: nil},
		{name: "exact boundary", text: "abcd", max: 4, want: []string{"abcd"}},
		{name: "one over", text: "abcde", max: 4, want: []string{"abcd", "e"}},
		{name: "unicode and whitespace", text: " 你\n好🙂 ", max: 3, want: []string{" 你\n", "好🙂 "}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			chunks, err := ChunkFullRiskText(test.text, test.max)
			require.NoError(t, err)
			require.Equal(t, test.want, chunks)
			require.Equal(t, test.text, strings.Join(chunks, ""))
			for _, chunk := range chunks {
				require.LessOrEqual(t, len([]rune(chunk)), test.max)
			}
		})
	}
}

func TestChunkFullRiskText_rejects_non_positive_limit(t *testing.T) {
	_, err := ChunkFullRiskText("text", 0)
	require.ErrorIs(t, err, ErrInvalidFullReviewChunkLimit)
}

func TestReviewFullRiskText_unsafe_dominates_errors_and_aggregates_trace(t *testing.T) {
	providerErr := errors.New("provider unavailable")
	results := []struct {
		result RiskReviewResult
		err    error
	}{
		{result: RiskReviewResult{Status: RiskReviewSafe, Categories: []string{"safe-note"}, Usage: RiskReviewUsage{PromptTokens: 3, TotalTokens: 3}}},
		{result: RiskReviewResult{Categories: []string{"partial"}, Usage: RiskReviewUsage{PromptTokens: 2}}, err: providerErr},
		{result: RiskReviewResult{Status: RiskReviewUnsafe, Categories: []string{"S1", "S1"}, Usage: RiskReviewUsage{PromptTokens: 5, CompletionTokens: 1, TotalTokens: 6, Neurons: 9.072817475858999}}},
	}
	call := 0
	reviewer := func(_ context.Context, _ string) (RiskReviewResult, error) {
		result := results[call]
		call++
		return result.result, result.err
	}

	got, err := ReviewFullRiskText(context.Background(), "abcdef", 2, reviewer)

	require.NoError(t, err)
	require.Equal(t, RiskReviewUnsafe, got.Status)
	require.Equal(t, []string{"safe-note", "partial", "S1"}, got.Categories)
	require.Equal(t, 10, got.Usage.PromptTokens)
	require.Equal(t, 1, got.Usage.CompletionTokens)
	require.Equal(t, 9, got.Usage.TotalTokens)
	require.InDelta(t, 9.072817475858999, got.Usage.Neurons, 1e-12)
	require.Len(t, got.Chunks, 3)
	require.Equal(t, RiskReviewError, got.Chunks[1].Status)
	require.ErrorIs(t, got.Chunks[1].Err, providerErr)
	require.Equal(t, []string{"partial"}, got.Chunks[1].Categories)
	for _, chunk := range got.Chunks {
		require.GreaterOrEqual(t, chunk.LatencyMS, int64(0))
	}
}

func TestReviewFullRiskText_errors_fail_open_when_no_chunk_is_unsafe(t *testing.T) {
	reviewer := func(_ context.Context, chunk string) (RiskReviewResult, error) {
		if chunk == "cd" {
			return RiskReviewResult{}, errors.New("rate limited")
		}
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}

	got, err := ReviewFullRiskText(context.Background(), "abcdef", 2, reviewer)

	require.NoError(t, err)
	require.Equal(t, RiskReviewError, got.Status)
	require.Len(t, got.Chunks, 3)
}

func TestReviewFullRiskText_reuses_one_caller_deadline_for_all_chunks(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	var seen []time.Time
	reviewer := func(chunkCtx context.Context, _ string) (RiskReviewResult, error) {
		gotDeadline, ok := chunkCtx.Deadline()
		require.True(t, ok)
		seen = append(seen, gotDeadline)
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}

	got, err := ReviewFullRiskText(ctx, "abcdef", 2, reviewer)

	require.NoError(t, err)
	require.Equal(t, RiskReviewSafe, got.Status)
	require.Equal(t, []time.Time{deadline, deadline, deadline}, seen)
}

func TestReviewFullRiskText_marks_unfinished_chunks_after_shared_context_cancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	reviewer := func(_ context.Context, _ string) (RiskReviewResult, error) {
		calls++
		cancel()
		return RiskReviewResult{Status: RiskReviewSafe}, nil
	}

	got, err := ReviewFullRiskText(ctx, "abcdef", 2, reviewer)

	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Equal(t, RiskReviewError, got.Status)
	require.Len(t, got.Chunks, 3)
	require.ErrorIs(t, got.Chunks[1].Err, context.Canceled)
	require.ErrorIs(t, got.Chunks[2].Err, context.Canceled)
}

func TestBuildSelectiveRiskExcerpt_keeps_existing_limit(t *testing.T) {
	text := strings.Repeat("safe ", 900) + "danger" + strings.Repeat(" tail", 900)
	excerpt, ruleIDs := BuildSelectiveRiskExcerpt(text, []*model.RiskRule{{
		Id: 7, RuleType: model.RiskRuleKeyword, Pattern: "danger", Enabled: true,
	}})

	require.Equal(t, []int{7}, ruleIDs)
	require.LessOrEqual(t, len([]rune(excerpt)), riskExcerptLimit)
	require.Contains(t, excerpt, "danger")
}
