package service

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRiskObservationModelSink_persists_event_contract(t *testing.T) {
	tests := []struct {
		name             string
		event            RiskObservationEvent
		wantSource       model.RiskRecordSource
		wantNeurons      int64
		wantChunkNeurons int64
	}{
		{
			name:        "unsafe result from cache",
			wantSource:  model.RiskRecordSourceCache,
			wantNeurons: 0,
			event: RiskObservationEvent{
				RequestID: "req-sink", ChannelID: 12, UserID: 34, TokenID: 55,
				Model: "gpt-5.6", Path: "/v1/responses", Preview: "masked preview",
				ContentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				Source:      RiskObservationSourceCache, CacheHit: true, Blocked: true, RuleIDs: []int{5, 8},
				Result:     RiskObservationUnsafe,
				Categories: []string{"violent crimes"}, LatencyMS: 93,
				ObservedAt: time.Date(2026, time.July, 25, 12, 30, 0, 0, time.UTC),
			},
		},
		{
			name:       "unsafe result allowed by category",
			wantSource: model.RiskRecordSourceCache,
			event: RiskObservationEvent{
				RequestID: "req-allowed", ChannelID: 12, UserID: 34,
				Result: RiskObservationUnsafe, Categories: []string{"S14"}, Source: RiskObservationSourceCache,
				CacheHit: true, NonBlockingMatched: true,
				ObservedAt: time.Date(2026, time.July, 25, 12, 30, 30, 0, time.UTC),
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
			name:       "provider error detail",
			wantSource: model.RiskRecordSourceProvider,
			event: RiskObservationEvent{
				RequestID: "req-error", ChannelID: 12, UserID: 34,
				ProviderID: 21, ProviderName: "Cloudflare", ProviderType: model.RiskProviderCloudflare, Result: RiskObservationError,
				Source: RiskObservationSourceProvider, ProviderCalled: true,
				ErrorCode: riskObservationProviderError, ErrorDetail: "Cloudflare returned HTTP 429",
				ObservedAt: time.Date(2026, time.July, 25, 12, 31, 30, 0, time.UTC),
			},
		},
		{
			name:             "actual provider call",
			wantSource:       model.RiskRecordSourceProvider,
			wantChunkNeurons: 6,
			event: RiskObservationEvent{
				RequestID: "req-provider", ChannelID: 12, UserID: 34,
				ProviderID: 21, ProviderName: "Cloudflare", ProviderType: model.RiskProviderCloudflare, Result: RiskObservationSafe,
				Source: RiskObservationSourceProvider, ProviderCalled: true,
				Chunks: []RiskReviewChunkAudit{{
					Index: 0, Status: RiskReviewSafe, Categories: []string{"clean"}, LatencyMS: 17,
					Usage: RiskReviewUsage{PromptTokens: 3, CompletionTokens: 1, TotalTokens: 4, Neurons: 5.625},
				}},
				ObservedAt: time.Date(2026, time.July, 25, 12, 32, 0, 0, time.UTC),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskObservationTest(t)
			require.NoError(t, model.DB.AutoMigrate(
				&model.RiskRecord{},
				&model.RiskRecordGovernance{},
				&model.Channel{},
				&model.User{},
				&model.Token{},
			))

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
			assert.Equal(t, test.event.NonBlockingMatched, record.NonBlockingMatched)
			if len(test.event.RuleIDs) == 0 {
				assert.Empty(t, record.RuleIDs)
			} else {
				assert.Equal(t, test.event.RuleIDs, record.RuleIDs)
			}
			assert.Equal(t, test.event.ProviderID, record.ProviderID)
			assert.Equal(t, test.event.ProviderName, record.ProviderName)
			assert.Equal(t, test.event.ProviderType, record.ProviderType)
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
			assert.Equal(t, test.wantNeurons, record.Neurons)
			if len(test.event.Chunks) == 0 {
				assert.Empty(t, record.Chunks)
			} else {
				require.Len(t, record.Chunks, len(test.event.Chunks))
				assert.Equal(t, test.event.Chunks[0].Index, record.Chunks[0].Index)
				assert.Equal(t, model.RiskRecordResult(test.event.Chunks[0].Status), record.Chunks[0].Result)
				assert.Equal(t, test.event.Chunks[0].Categories, record.Chunks[0].Categories)
				assert.Equal(t, test.event.Chunks[0].LatencyMS, record.Chunks[0].LatencyMS)
				assert.Equal(t, test.event.Chunks[0].Usage.TotalTokens, record.Chunks[0].TotalTokens)
				assert.Equal(t, test.wantChunkNeurons, record.Chunks[0].Neurons)
			}
			assert.Equal(t, test.event.ErrorCode, record.ErrorCode)
			assert.Equal(t, test.event.ErrorDetail, record.ErrorDetail)
			assert.Equal(t, test.event.ObservedAt, record.ObservedAt)
		})
	}
}

func TestRiskObservationNeuronsForStorage_roundsAndRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		neurons float64
		want    int64
		wantErr bool
	}{
		{name: "integer", neurons: 12, want: 12},
		{name: "fractional below half", neurons: 9.072817475858999, want: 9},
		{name: "fractional half rounds up", neurons: 9.5, want: 10},
		{name: "negative", neurons: -0.1, wantErr: true},
		{name: "not a number", neurons: math.NaN(), wantErr: true},
		{name: "infinity", neurons: math.Inf(1), wantErr: true},
		{name: "overflow", neurons: math.Exp2(63), wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			got, err := riskObservationNeuronsForStorage(test.neurons)

			// Then
			if test.wantErr {
				require.ErrorIs(t, err, errInvalidRiskObservationNeurons)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}
