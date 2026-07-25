package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRiskObservationModelSink_persists_event_contract(t *testing.T) {
	tests := []struct {
		name       string
		event      RiskObservationEvent
		wantSource model.RiskRecordSource
	}{
		{
			name:       "unsafe result with provider",
			wantSource: model.RiskRecordSourceCache,
			event: RiskObservationEvent{
				RequestID: "req-sink", ChannelID: 12, UserID: 34, TokenID: 55,
				Model: "gpt-5.6", Path: "/v1/responses", Preview: "masked preview",
				ContentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Source:      RiskObservationSourceCache, CacheHit: true, Blocked: true, RuleIDs: []int{5, 8},
				ProviderID: 21, ProviderName: "Cloudflare", Result: RiskObservationUnsafe,
				Categories: []string{"violent crimes"}, LatencyMS: 93, PromptTokens: 11,
				CompletionTokens: 2, TotalTokens: 13, Neurons: 7,
				ObservedAt: time.Date(2026, time.July, 25, 12, 30, 0, 0, time.UTC),
			},
		},
		{
			name:       "queue degradation before provider selection",
			wantSource: model.RiskRecordSourceLocal,
			event: RiskObservationEvent{
				RequestID: "req-degraded", ChannelID: 12, UserID: 34,
				Result: RiskObservationError, ErrorCode: RiskObservationErrorQueueFull,
				ObservedAt: time.Date(2026, time.July, 25, 12, 31, 0, 0, time.UTC),
			},
		},
		{
			name:       "actual provider call",
			wantSource: model.RiskRecordSourceProvider,
			event: RiskObservationEvent{
				RequestID: "req-provider", ChannelID: 12, UserID: 34,
				ProviderID: 21, ProviderName: "Cloudflare", Result: RiskObservationSafe,
				Source: RiskObservationSourceProvider, ProviderCalled: true,
				ObservedAt: time.Date(2026, time.July, 25, 12, 32, 0, 0, time.UTC),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskObservationTest(t)
			require.NoError(t, model.DB.AutoMigrate(&model.RiskRecord{}, &model.RiskRecordGovernance{}))

			// When
			err := (riskObservationModelSink{}).RecordRiskObservation(context.Background(), test.event)

			// Then
			require.NoError(t, err)
			records, total, err := model.ListRiskRecords(context.Background(), 0, 10)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, records, 1)
			record := records[0]
			assert.Equal(t, test.event.RequestID, record.RequestID)
			assert.Equal(t, test.event.ChannelID, record.ChannelID)
			assert.Equal(t, test.event.UserID, record.UserID)
			assert.Equal(t, test.event.TokenID, record.TokenID)
			assert.Equal(t, test.event.Model, record.Model)
			assert.Equal(t, test.event.Path, record.Path)
			assert.Equal(t, test.event.Preview, record.Preview)
			assert.Equal(t, test.event.ContentHash, record.ContentHash)
			assert.Equal(t, test.wantSource, record.Source)
			assert.Equal(t, test.event.CacheHit, record.CacheHit)
			assert.Equal(t, test.event.ProviderCalled, record.ProviderCalled)
			assert.Equal(t, test.event.Blocked, record.Blocked)
			if len(test.event.RuleIDs) == 0 {
				assert.Empty(t, record.RuleIDs)
			} else {
				assert.Equal(t, test.event.RuleIDs, record.RuleIDs)
			}
			assert.Equal(t, test.event.ProviderID, record.ProviderID)
			assert.Equal(t, test.event.ProviderName, record.ProviderName)
			assert.Equal(t, model.RiskRecordResult(test.event.Result), record.Result)
			if len(test.event.Categories) == 0 {
				assert.Empty(t, record.Categories)
			} else {
				assert.Equal(t, test.event.Categories, record.Categories)
			}
			assert.Equal(t, test.event.LatencyMS, record.LatencyMS)
			assert.Equal(t, test.event.PromptTokens, record.PromptTokens)
			assert.Equal(t, test.event.CompletionTokens, record.CompletionTokens)
			assert.Equal(t, test.event.TotalTokens, record.TotalTokens)
			assert.Equal(t, test.event.Neurons, record.Neurons)
			assert.Equal(t, test.event.ErrorCode, record.ErrorCode)
			assert.Equal(t, test.event.ObservedAt, record.ObservedAt)
		})
	}
}
