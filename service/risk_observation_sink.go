package service

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/QuantumNous/new-api/model"
)

var errInvalidRiskObservationNeurons = errors.New("invalid risk observation neurons")

type riskObservationModelSink struct{}

func (riskObservationModelSink) RecordRiskObservation(ctx context.Context, event RiskObservationEvent) error {
	neurons, err := riskObservationNeuronsForStorage(event.Neurons)
	if err != nil {
		return fmt.Errorf("normalize risk observation neurons: %w", err)
	}
	chunks := make([]model.RiskRecordChunk, 0, len(event.Chunks))
	for _, chunk := range event.Chunks {
		chunkNeurons, err := riskObservationNeuronsForStorage(chunk.Usage.Neurons)
		if err != nil {
			return fmt.Errorf("normalize risk observation chunk %d neurons: %w", chunk.Index, err)
		}
		chunks = append(chunks, model.RiskRecordChunk{
			Index: chunk.Index, Result: model.RiskRecordResult(chunk.Status),
			Categories: append([]string(nil), chunk.Categories...), Summary: chunk.Summary, LatencyMS: chunk.LatencyMS,
			PromptTokens: chunk.Usage.PromptTokens, CompletionTokens: chunk.Usage.CompletionTokens,
			TotalTokens: chunk.Usage.TotalTokens, Neurons: chunkNeurons,
		})
	}
	return model.RecordRiskObservation(ctx, model.RiskRecordInput{
		RequestID:          event.RequestID,
		ChannelID:          event.ChannelID,
		UserID:             event.UserID,
		TokenID:            event.TokenID,
		Model:              event.Model,
		Path:               event.Path,
		Preview:            event.Preview,
		ContentHash:        event.ContentHash,
		RuleIDs:            event.RuleIDs,
		ProviderID:         event.ProviderID,
		ProviderName:       event.ProviderName,
		ProviderType:       event.ProviderType,
		Result:             model.RiskRecordResult(event.Result),
		Categories:         event.Categories,
		LatencyMS:          event.LatencyMS,
		PromptTokens:       event.PromptTokens,
		CompletionTokens:   event.CompletionTokens,
		TotalTokens:        event.TotalTokens,
		Neurons:            neurons,
		Chunks:             chunks,
		ErrorCode:          event.ErrorCode,
		ErrorDetail:        event.ErrorDetail,
		Source:             model.RiskRecordSource(event.Source),
		CacheHit:           event.CacheHit,
		ProviderCalled:     event.ProviderCalled,
		Blocked:            event.Blocked,
		NonBlockingMatched: event.NonBlockingMatched,
		ObservedAt:         event.ObservedAt,
	})
}

func riskObservationNeuronsForStorage(neurons float64) (int64, error) {
	if math.IsNaN(neurons) || math.IsInf(neurons, 0) || neurons < 0 {
		return 0, errInvalidRiskObservationNeurons
	}
	rounded := math.Round(neurons)
	if rounded >= math.Exp2(63) {
		return 0, errInvalidRiskObservationNeurons
	}
	return int64(rounded), nil
}
