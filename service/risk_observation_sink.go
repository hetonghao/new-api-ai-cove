package service

import (
	"context"

	"github.com/QuantumNous/new-api/model"
)

type riskObservationModelSink struct{}

func (riskObservationModelSink) RecordRiskObservation(ctx context.Context, event RiskObservationEvent) error {
	return model.RecordRiskObservation(ctx, model.RiskRecordInput{
		RequestID:        event.RequestID,
		ChannelID:        event.ChannelID,
		UserID:           event.UserID,
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
		ErrorCode:        event.ErrorCode,
		ObservedAt:       event.ObservedAt,
	})
}
