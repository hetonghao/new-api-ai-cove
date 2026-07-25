package service

import (
	"context"

	"github.com/QuantumNous/new-api/model"
)

type riskObservationModelSink struct{}

func (riskObservationModelSink) RecordRiskObservation(ctx context.Context, event RiskObservationEvent) error {
	chunks := make([]model.RiskRecordChunk, 0, len(event.Chunks))
	for _, chunk := range event.Chunks {
		chunks = append(chunks, model.RiskRecordChunk{
			Index: chunk.Index, Result: model.RiskRecordResult(chunk.Status),
			Categories: append([]string(nil), chunk.Categories...), LatencyMS: chunk.LatencyMS,
			PromptTokens: chunk.Usage.PromptTokens, CompletionTokens: chunk.Usage.CompletionTokens,
			TotalTokens: chunk.Usage.TotalTokens, Neurons: chunk.Usage.Neurons,
		})
	}
	return model.RecordRiskObservation(ctx, model.RiskRecordInput{
		RequestID:        event.RequestID,
		ChannelID:        event.ChannelID,
		UserID:           event.UserID,
		TokenID:          event.TokenID,
		Model:            event.Model,
		Path:             event.Path,
		Preview:          event.Preview,
		ContentHash:      event.ContentHash,
		RuleIDs:          event.RuleIDs,
		ProviderID:       event.ProviderID,
		ProviderName:     event.ProviderName,
		Result:           model.RiskRecordResult(event.Result),
		Categories:       event.Categories,
		LatencyMS:        event.LatencyMS,
		PromptTokens:     event.PromptTokens,
		CompletionTokens: event.CompletionTokens,
		TotalTokens:      event.TotalTokens,
		Neurons:          event.Neurons,
		Chunks:           chunks,
		ErrorCode:        event.ErrorCode,
		Source:           model.RiskRecordSource(event.Source),
		CacheHit:         event.CacheHit,
		ProviderCalled:   event.ProviderCalled,
		Blocked:          event.Blocked,
		ObservedAt:       event.ObservedAt,
	})
}
