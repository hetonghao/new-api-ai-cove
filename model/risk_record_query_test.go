package model

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestQueryRiskRecords_filtersByTimeChannelUserResultSourceAndProvider(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)
	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, db.Create(&User{Id: 99, Username: "alice", Password: "password"}).Error)
	baseTime := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)
	inputs := []RiskRecordInput{
		validRiskRecordInput(RiskRecordResultSafe),
		validRiskRecordInput(RiskRecordResultUnsafe),
		validRiskRecordInput(RiskRecordResultUnsafe),
	}
	inputs[0].RequestID = "req-before"
	inputs[0].ObservedAt = baseTime.Add(-time.Minute)
	inputs[1].RequestID = "req-match"
	inputs[1].ObservedAt = baseTime
	inputs[1].ChannelID = 88
	inputs[1].UserID = 99
	inputs[1].ProviderID = 77
	inputs[1].ProviderName = "Matched"
	inputs[1].ProviderType = RiskProviderPlatformInternal
	inputs[1].Source = RiskRecordSourceProvider
	inputs[1].ProviderCalled = true
	inputs[2].RequestID = "req-after"
	inputs[2].ObservedAt = baseTime.Add(time.Minute)
	for _, input := range inputs {
		require.NoError(t, RecordRiskObservation(context.Background(), input))
	}
	providerID := 77

	// When
	records, total, err := QueryRiskRecords(context.Background(), RiskRecordQuery{
		Offset: 0, Limit: 20,
		StartTimestamp: baseTime.Unix(), EndTimestamp: baseTime.Unix(),
		ChannelID: 88, Username: "alice", ProviderID: &providerID,
		ProviderType: RiskProviderPlatformInternal,
		Result:       RiskRecordResultUnsafe, Source: RiskRecordSourceProvider,
	})

	// Then
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	assert.Equal(t, "req-match", records[0].RequestID)
}

func TestQueryRiskRecords_filtersByUsername(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)
	require.NoError(t, db.AutoMigrate(&User{}))
	require.NoError(t, db.Create(&User{Id: 34, Username: "alice", Password: "password", AffCode: "alice"}).Error)
	require.NoError(t, db.Create(&User{Id: 35, Username: "bob", Password: "password", AffCode: "bob"}).Error)
	observedAt := time.Date(2026, time.July, 26, 2, 0, 0, 0, time.UTC)
	for _, input := range []RiskRecordInput{
		{RequestID: "req-alice", ChannelID: 12, UserID: 34, ProviderID: 21, ProviderName: "Cloudflare", Result: RiskRecordResultSafe, ObservedAt: observedAt},
		{RequestID: "req-bob", ChannelID: 12, UserID: 35, ProviderID: 21, ProviderName: "Cloudflare", Result: RiskRecordResultSafe, ObservedAt: observedAt},
	} {
		require.NoError(t, RecordRiskObservation(context.Background(), input))
	}

	// When
	records, total, err := QueryRiskRecords(context.Background(), RiskRecordQuery{
		Offset: 0, Limit: 20, Username: "alice",
	})

	// Then
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	assert.Equal(t, "req-alice", records[0].RequestID)
}

func TestQueryRiskRecords_filtersByUserIDWhenUsernameIsMissing(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	input := validRiskRecordInput(RiskRecordResultSafe)
	input.RequestID = "req-orphaned-user"
	input.UserID = 404
	require.NoError(t, RecordRiskObservation(context.Background(), input))

	// When
	records, total, err := QueryRiskRecords(context.Background(), RiskRecordQuery{
		Offset: 0, Limit: 20, UserID: 404,
	})

	// Then
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	assert.Equal(t, "req-orphaned-user", records[0].RequestID)
}

func TestQueryRiskRecords_enrichesChannelUserAndTokenNames(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)
	require.NoError(t, db.AutoMigrate(&Channel{}, &User{}, &Token{}))
	require.NoError(t, db.Create(&Channel{Id: 12, Name: "CPA Pro", Key: "secret"}).Error)
	require.NoError(t, db.Create(&User{Id: 34, Username: "alice", Password: "password", AffCode: "alice"}).Error)
	require.NoError(t, db.Create(&Token{Id: 56, UserId: 34, Name: "Codex", Key: "token-key"}).Error)
	input := validRiskRecordInput(RiskRecordResultSafe)
	input.TokenID = 56
	require.NoError(t, RecordRiskObservation(context.Background(), input))

	// When
	records, total, err := QueryRiskRecords(context.Background(), RiskRecordQuery{Offset: 0, Limit: 20})

	// Then
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	assert.Equal(t, "CPA Pro", records[0].ChannelName)
	assert.Equal(t, "alice", records[0].Username)
	assert.Equal(t, "Codex", records[0].TokenName)
}

func TestQueryRiskRecords_keepsRecordsWhenRelatedEntitiesAreMissing(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)
	require.NoError(t, db.AutoMigrate(&Channel{}, &User{}, &Token{}))
	input := validRiskRecordInput(RiskRecordResultSafe)
	input.RequestID = "req-orphaned-relations"
	input.TokenID = 56
	require.NoError(t, RecordRiskObservation(context.Background(), input))

	// When
	records, total, err := QueryRiskRecords(context.Background(), RiskRecordQuery{Offset: 0, Limit: 20})

	// Then
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	assert.Equal(t, "req-orphaned-relations", records[0].RequestID)
	assert.Empty(t, records[0].ChannelName)
	assert.Empty(t, records[0].Username)
	assert.Empty(t, records[0].TokenName)
}

func TestQueryRiskRecords_filtersByUsernameWildcard(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)
	require.NoError(t, db.AutoMigrate(&User{}))
	users := []User{
		{Id: 34, Username: "alice", Password: "password", AffCode: "alice"},
		{Id: 35, Username: "alicia", Password: "password", AffCode: "alicia"},
		{Id: 36, Username: "bob", Password: "password", AffCode: "bob"},
	}
	require.NoError(t, db.Create(&users).Error)
	observedAt := time.Date(2026, time.July, 26, 2, 0, 0, 0, time.UTC)
	for _, input := range []RiskRecordInput{
		{RequestID: "req-alice", ChannelID: 12, UserID: 34, ProviderID: 21, ProviderName: "Cloudflare", Result: RiskRecordResultSafe, ObservedAt: observedAt},
		{RequestID: "req-alicia", ChannelID: 12, UserID: 35, ProviderID: 21, ProviderName: "Cloudflare", Result: RiskRecordResultSafe, ObservedAt: observedAt},
		{RequestID: "req-bob", ChannelID: 12, UserID: 36, ProviderID: 21, ProviderName: "Cloudflare", Result: RiskRecordResultSafe, ObservedAt: observedAt},
	} {
		require.NoError(t, RecordRiskObservation(context.Background(), input))
	}

	// When
	records, total, err := QueryRiskRecords(context.Background(), RiskRecordQuery{
		Offset: 0, Limit: 20, Username: "ali%",
	})

	// Then
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, records, 2)
	assert.ElementsMatch(t, []string{"req-alice", "req-alicia"}, []string{records[0].RequestID, records[1].RequestID})
}

func TestQueryRiskRecords_materializesUsernameIDsBeforeRiskRecordQueryAcrossSeparateHandles(t *testing.T) {
	// Given
	mainDB := setupRiskRecordModelTest(t)
	require.NoError(t, mainDB.AutoMigrate(&User{}))
	require.NoError(t, mainDB.Create(&User{Id: 34, Username: "alice", Password: "password", AffCode: "alice"}).Error)
	originalLogDB := LOG_DB
	dsn := fmt.Sprintf("file:%s_log?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	logDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	LOG_DB = logDB
	t.Cleanup(func() {
		LOG_DB = originalLogDB
		if sqlDB, dbErr := logDB.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, logDB.AutoMigrate(&RiskRecord{}))

	observedAt := time.Date(2026, time.July, 26, 2, 0, 0, 0, time.UTC)
	require.NoError(t, RecordRiskObservation(context.Background(), RiskRecordInput{
		RequestID: "req-alice", ChannelID: 12, UserID: 34, ProviderID: 21,
		ProviderName: "Cloudflare", Result: RiskRecordResultSafe, ObservedAt: observedAt,
	}))

	executedSQL := make([]string, 0, 3)
	require.NoError(t, mainDB.Callback().Query().After("gorm:query").Register("test:capture-risk-record-query", func(tx *gorm.DB) {
		executedSQL = append(executedSQL, tx.Dialector.Explain(tx.Statement.SQL.String(), tx.Statement.Vars...))
	}))

	// When
	records, total, err := QueryRiskRecords(context.Background(), RiskRecordQuery{
		Offset: 0, Limit: 20, Username: "alice",
	})

	// Then
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	assert.Equal(t, "req-alice", records[0].RequestID)
	var sawUserLookup bool
	var sawRiskRecordLookup bool
	for _, statement := range executedSQL {
		if strings.Contains(statement, "FROM `users`") {
			sawUserLookup = true
		}
		if strings.Contains(statement, "FROM `risk_records`") {
			sawRiskRecordLookup = true
			assert.NotContains(t, statement, "FROM `users`")
		}
	}
	assert.True(t, sawUserLookup)
	assert.True(t, sawRiskRecordLookup)
}

func TestQueryRiskRecords_distinguishesMissingProviderFilterFromExplicitZero(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	observedAt := time.Date(2026, time.July, 26, 1, 0, 0, 0, time.UTC)
	local := validRiskRecordInput(RiskRecordResultNotReviewed)
	local.RequestID = "provider-zero"
	local.ProviderID = 0
	local.ProviderName = ""
	local.ProviderType = ""
	local.Source = RiskRecordSourceLocal
	local.ObservedAt = observedAt
	provider := validRiskRecordInput(RiskRecordResultSafe)
	provider.RequestID = "provider-positive"
	provider.ProviderID = 21
	provider.ObservedAt = observedAt.Add(time.Second)
	require.NoError(t, RecordRiskObservation(context.Background(), local))
	require.NoError(t, RecordRiskObservation(context.Background(), provider))
	zero := 0
	tests := []struct {
		name       string
		providerID *int
		wantTotal  int64
		wantIDs    []string
	}{
		{name: "missing provider filter", providerID: nil, wantTotal: 2, wantIDs: []string{"provider-positive", "provider-zero"}},
		{name: "explicit zero provider", providerID: &zero, wantTotal: 1, wantIDs: []string{"provider-zero"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			records, total, err := QueryRiskRecords(context.Background(), RiskRecordQuery{
				Offset: 0, Limit: 20, ProviderID: test.providerID,
			})

			// Then
			require.NoError(t, err)
			assert.Equal(t, test.wantTotal, total)
			require.Len(t, records, len(test.wantIDs))
			for index, requestID := range test.wantIDs {
				assert.Equal(t, requestID, records[index].RequestID)
			}
		})
	}
}

func TestQueryRiskRecords_rejectsInvalidFilters(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	negativeProviderID := -1
	tests := []RiskRecordQuery{
		{Offset: -1, Limit: 20},
		{Offset: 0, Limit: 101},
		{Offset: 0, Limit: 20, StartTimestamp: 20, EndTimestamp: 10},
		{Offset: 0, Limit: 20, ChannelID: -1},
		{Offset: 0, Limit: 20, Username: "%a%"},
		{Offset: 0, Limit: 20, ProviderID: &negativeProviderID},
		{Offset: 0, Limit: 20, Result: "maybe"},
		{Offset: 0, Limit: 20, Source: "remote"},
	}
	for _, query := range tests {
		// When
		_, _, err := QueryRiskRecords(context.Background(), query)

		// Then
		require.ErrorIs(t, err, ErrInvalidRiskRecordPage)
	}
}
