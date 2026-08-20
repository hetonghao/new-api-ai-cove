package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRecordSevereRiskEvent_isIdempotentByRequestID(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)
	record := SevereRiskRecordInput{
		RequestID: "severe-req-1", ChannelID: 37, ChannelName: "codex", UserID: 56,
		Username: "user@example.com", TokenID: 9, TokenName: "token", Model: "gpt-5.6-sol",
		Path: "/v1/responses", ErrorCode: "invalid_prompt", ErrorDetail: "Invalid prompt",
		ContextHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", ContextEncrypted: "ciphertext", ChannelScope: SevereRiskChannelScopeAll,
		TriggeredAt: time.Date(2026, time.August, 21, 0, 0, 0, 0, time.UTC),
	}

	// When
	require.NoError(t, RecordSevereRiskEvent(context.Background(), record))
	require.NoError(t, RecordSevereRiskEvent(context.Background(), record))

	// Then
	var count int64
	require.NoError(t, db.Model(&SevereRiskRecord{}).Where("request_id = ?", record.RequestID).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestClaimSevereRiskAction_isSingleUse(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	record := SevereRiskRecordInput{
		RequestID: "severe-claim-1", ChannelID: 37, ChannelName: "codex", UserID: 56,
		Username: "user@example.com", TokenID: 9, TokenName: "token", Model: "gpt-5.6-sol",
		Path: "/v1/responses", ErrorCode: "invalid_prompt", ErrorDetail: "Invalid prompt",
		ContextHash: "hash", ContextEncrypted: "cipher", ChannelScope: SevereRiskChannelScopeAll,
		TriggeredAt: time.Now().UTC(),
	}
	require.NoError(t, RecordSevereRiskEvent(context.Background(), record))

	// When
	first, err := ClaimSevereRiskAction(context.Background(), record.RequestID)
	require.NoError(t, err)
	second, err := ClaimSevereRiskAction(context.Background(), record.RequestID)

	// Then
	require.NoError(t, err)
	require.True(t, first)
	require.False(t, second)
}
