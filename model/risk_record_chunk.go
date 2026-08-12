package model

import "strings"

const RiskRecordChunkSummaryMaxRunes = 500

type RiskRecordChunk struct {
	Index            int              `json:"index"`
	Result           RiskRecordResult `json:"result"`
	Categories       []string         `json:"categories"`
	Summary          string           `json:"summary"`
	LatencyMS        int64            `json:"latency_ms"`
	PromptTokens     int              `json:"prompt_tokens"`
	CompletionTokens int              `json:"completion_tokens"`
	TotalTokens      int              `json:"total_tokens"`
	Neurons          int64            `json:"neurons"`
}

func normalizeRiskRecordChunks(
	input []RiskRecordChunk,
	source RiskRecordSource,
	providerCalled bool,
) ([]RiskRecordChunk, error) {
	if len(input) > 0 && (source != RiskRecordSourceProvider || !providerCalled) {
		return nil, ErrInvalidRiskRecord
	}
	chunks := make([]RiskRecordChunk, 0, len(input))
	for index, chunk := range input {
		chunk.Summary = strings.TrimSpace(chunk.Summary)
		if chunk.Index != index || chunk.LatencyMS < 0 || chunk.PromptTokens < 0 ||
			chunk.CompletionTokens < 0 || chunk.TotalTokens < 0 || chunk.Neurons < 0 ||
			len([]rune(chunk.Summary)) > RiskRecordChunkSummaryMaxRunes {
			return nil, ErrInvalidRiskRecord
		}
		switch chunk.Result {
		case RiskRecordResultUnsafe:
		case RiskRecordResultSafe, RiskRecordResultError:
			if chunk.Summary != "" {
				return nil, ErrInvalidRiskRecord
			}
		default:
			return nil, ErrInvalidRiskRecord
		}
		if len(chunk.Categories) > 64 {
			return nil, ErrInvalidRiskRecord
		}
		categories := make([]string, 0, len(chunk.Categories))
		for _, category := range chunk.Categories {
			category = strings.TrimSpace(category)
			if category == "" || len(category) > 128 {
				return nil, ErrInvalidRiskRecord
			}
			categories = append(categories, category)
		}
		chunk.Categories = categories
		chunks = append(chunks, chunk)
	}
	return chunks, nil
}
