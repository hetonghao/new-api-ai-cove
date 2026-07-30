package model

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRiskRecordModelTest(t *testing.T) *gorm.DB {
	t.Helper()
	originalDB := DB
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	require.NoError(t, db.AutoMigrate(
		&RiskRecord{},
		&RiskRecordGovernance{},
		&Channel{},
		&User{},
		&Token{},
	))
	t.Cleanup(func() {
		DB = originalDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func validRiskRecordInput(result RiskRecordResult) RiskRecordInput {
	input := RiskRecordInput{
		RequestID: "req-1", ChannelID: 12, UserID: 34, RuleIDs: []int{5, 8},
		ProviderID: 21, ProviderName: "Cloudflare", ProviderType: RiskProviderCloudflare,
		Result: result, Categories: []string{"violent crimes"},
		LatencyMS: 93, PromptTokens: 11, CompletionTokens: 2, TotalTokens: 13, Neurons: 7,
		ObservedAt: time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC),
	}
	if result == RiskRecordResultError {
		input.ErrorCode = "timeout"
		input.ErrorDetail = "云审核超过配置的总时间预算"
	}
	return input
}

func TestRecordRiskObservation_persistsSafeUnsafeAndErrorMetadata(t *testing.T) {
	results := []RiskRecordResult{RiskRecordResultSafe, RiskRecordResultUnsafe, RiskRecordResultError}
	for _, result := range results {
		t.Run(string(result), func(t *testing.T) {
			// Given
			setupRiskRecordModelTest(t)
			input := validRiskRecordInput(result)

			// When
			err := RecordRiskObservation(context.Background(), input)

			// Then
			require.NoError(t, err)
			records, total, err := ListRiskRecords(context.Background(), 0, 10)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, records, 1)
			assert.Equal(t, input.RequestID, records[0].RequestID)
			assert.Equal(t, input.RuleIDs, records[0].RuleIDs)
			assert.Equal(t, input.Result, records[0].Result)
			assert.Equal(t, input.Categories, records[0].Categories)
			assert.Equal(t, input.ProviderType, records[0].ProviderType)
			assert.Equal(t, input.Neurons, records[0].Neurons)
			assert.Equal(t, input.ErrorCode, records[0].ErrorCode)
			assert.Equal(t, input.ErrorDetail, records[0].ErrorDetail)
			assert.Equal(t, input.ObservedAt, records[0].ObservedAt)
		})
	}
}

func TestRecordRiskObservation_persistsPreProviderErrorsWithoutProvider(t *testing.T) {
	errorCodes := []string{"queue_full", "service_shutdown", "policy_error", "rules_error"}
	for _, errorCode := range errorCodes {
		t.Run(errorCode, func(t *testing.T) {
			// Given
			setupRiskRecordModelTest(t)
			input := validRiskRecordInput(RiskRecordResultError)
			input.ProviderID = 0
			input.ProviderName = ""
			input.ErrorCode = errorCode

			// When
			err := RecordRiskObservation(context.Background(), input)

			// Then
			require.NoError(t, err)
			records, total, err := ListRiskRecords(context.Background(), 0, 1)
			require.NoError(t, err)
			require.EqualValues(t, 1, total)
			require.Len(t, records, 1)
			assert.Zero(t, records[0].ProviderID)
			assert.Empty(t, records[0].ProviderName)
			assert.Equal(t, errorCode, records[0].ErrorCode)
		})
	}
}

func TestRecordRiskObservation_truncatesErrorDetail(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	input := validRiskRecordInput(RiskRecordResultError)
	input.ErrorDetail = strings.Repeat("诊", riskRecordErrorDetailMaxRunes+50)

	// When
	err := RecordRiskObservation(context.Background(), input)

	// Then
	require.NoError(t, err)
	records, _, err := ListRiskRecords(context.Background(), 0, 1)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Len(t, []rune(records[0].ErrorDetail), riskRecordErrorDetailMaxRunes)
}

func TestRecordRiskObservation_rejectsMissingProviderOutsidePreProviderErrors(t *testing.T) {
	tests := []struct {
		name      string
		result    RiskRecordResult
		errorCode string
	}{
		{name: "safe", result: RiskRecordResultSafe},
		{name: "unsafe", result: RiskRecordResultUnsafe},
		{name: "provider error", result: RiskRecordResultError, errorCode: "provider_error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskRecordModelTest(t)
			input := validRiskRecordInput(test.result)
			input.ProviderID = 0
			input.ProviderName = ""
			input.ErrorCode = test.errorCode

			// When
			err := RecordRiskObservation(context.Background(), input)

			// Then
			require.ErrorIs(t, err, ErrInvalidRiskRecord)
		})
	}
}

func TestRecordRiskObservation_rejectsInvalidMetadata(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	tests := []struct {
		name   string
		mutate func(*RiskRecordInput)
	}{
		{name: "unknown result", mutate: func(input *RiskRecordInput) { input.Result = "maybe" }},
		{name: "missing request id", mutate: func(input *RiskRecordInput) { input.RequestID = "" }},
		{name: "negative latency", mutate: func(input *RiskRecordInput) { input.LatencyMS = -1 }},
		{name: "error without code", mutate: func(input *RiskRecordInput) { input.Result = RiskRecordResultError }},
		{name: "safe with error code", mutate: func(input *RiskRecordInput) { input.ErrorCode = "unexpected" }},
		{name: "safe with error detail", mutate: func(input *RiskRecordInput) { input.ErrorDetail = "unexpected" }},
		{name: "zero observation time", mutate: func(input *RiskRecordInput) { input.ObservedAt = time.Time{} }},
		{name: "chunk audit on cache hit", mutate: func(input *RiskRecordInput) {
			input.Source = RiskRecordSourceCache
			input.CacheHit = true
			input.Chunks = []RiskRecordChunk{{Index: 0, Result: RiskRecordResultSafe}}
		}},
		{name: "non-contiguous chunk index", mutate: func(input *RiskRecordInput) {
			input.Source = RiskRecordSourceProvider
			input.ProviderCalled = true
			input.Chunks = []RiskRecordChunk{{Index: 1, Result: RiskRecordResultSafe}}
		}},
		{name: "negative chunk usage", mutate: func(input *RiskRecordInput) {
			input.Source = RiskRecordSourceProvider
			input.ProviderCalled = true
			input.Chunks = []RiskRecordChunk{{Index: 0, Result: RiskRecordResultSafe, TotalTokens: -1}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validRiskRecordInput(RiskRecordResultSafe)
			test.mutate(&input)

			// When
			err := RecordRiskObservation(context.Background(), input)

			// Then
			require.ErrorIs(t, err, ErrInvalidRiskRecord)
		})
	}
}

func TestListRiskRecords_paginatesByObservationTime(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	baseTime := time.Date(2026, time.July, 25, 10, 0, 0, 0, time.UTC)
	for index, requestID := range []string{"req-old", "req-middle", "req-new"} {
		input := validRiskRecordInput(RiskRecordResultSafe)
		input.RequestID = requestID
		input.ObservedAt = baseTime.Add(time.Duration(index) * time.Minute)
		require.NoError(t, RecordRiskObservation(context.Background(), input))
	}

	// When
	records, total, err := ListRiskRecords(context.Background(), 1, 1)

	// Then
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Len(t, records, 1)
	assert.Equal(t, "req-middle", records[0].RequestID)
}

func TestListRiskRecords_rejectsUnboundedPagination(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	tests := []struct {
		name   string
		offset int
		limit  int
	}{
		{name: "negative offset", offset: -1, limit: 20},
		{name: "zero limit", offset: 0, limit: 0},
		{name: "negative limit", offset: 0, limit: -1},
		{name: "oversized limit", offset: 0, limit: 101},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			_, _, err := ListRiskRecords(context.Background(), test.offset, test.limit)

			// Then
			require.ErrorIs(t, err, ErrInvalidRiskRecordPage)
		})
	}
}

func TestRecordRiskObservation_acceptsEmptyRuleList(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	input := validRiskRecordInput(RiskRecordResultSafe)
	input.RuleIDs = nil

	// When
	err := RecordRiskObservation(context.Background(), input)

	// Then
	require.NoError(t, err)
	records, _, err := ListRiskRecords(context.Background(), 0, 1)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Empty(t, records[0].RuleIDs)
}

func TestRecordRiskObservation_persistsFullReviewChunkAudit(t *testing.T) {
	// Given
	setupRiskRecordModelTest(t)
	input := validRiskRecordInput(RiskRecordResultUnsafe)
	input.Source = RiskRecordSourceProvider
	input.ProviderCalled = true
	input.Chunks = []RiskRecordChunk{
		{
			Index: 0, Result: RiskRecordResultSafe, Categories: []string{}, LatencyMS: 21,
			PromptTokens: 4, CompletionTokens: 1, TotalTokens: 5, Neurons: 7,
		},
		{
			Index: 1, Result: RiskRecordResultUnsafe, Categories: []string{"S1"}, LatencyMS: 34,
			PromptTokens: 6, CompletionTokens: 2, TotalTokens: 8, Neurons: 9,
		},
	}

	// When
	err := RecordRiskObservation(context.Background(), input)

	// Then
	require.NoError(t, err)
	records, _, err := ListRiskRecords(context.Background(), 0, 1)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, input.Chunks, records[0].Chunks)
}

func TestRiskRecordMigration_hasNoFullContentOrSecretColumns(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)

	// When
	columns, err := db.Migrator().ColumnTypes(&RiskRecord{})

	// Then
	require.NoError(t, err)
	columnNames := make(map[string]struct{}, len(columns))
	for _, column := range columns {
		columnNames[column.Name()] = struct{}{}
	}
	for _, forbidden := range []string{"content", "text", "excerpt", "error_message", "credential", "query"} {
		_, exists := columnNames[forbidden]
		assert.False(t, exists, "forbidden column %q", forbidden)
	}
}

func TestRiskRecordMigration_addsChunkAuditToExistingRows(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)
	input := validRiskRecordInput(RiskRecordResultSafe)
	require.NoError(t, RecordRiskObservation(context.Background(), input))
	require.NoError(t, db.Migrator().DropColumn(&RiskRecord{}, "chunks"))
	require.False(t, db.Migrator().HasColumn(&RiskRecord{}, "chunks"))

	// When
	err := db.AutoMigrate(&RiskRecord{})

	// Then
	require.NoError(t, err)
	require.True(t, db.Migrator().HasColumn(&RiskRecord{}, "chunks"))
	records, total, err := ListRiskRecords(context.Background(), 0, 1)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, records, 1)
	assert.Equal(t, input.RequestID, records[0].RequestID)
	assert.Empty(t, records[0].Chunks)
}

func TestMigrateDB_includesRiskRecord(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)
	require.NoError(t, db.Migrator().DropTable(&RiskRecord{}))
	require.NoError(t, db.Migrator().DropTable(&RiskRecordGovernance{}))
	require.False(t, db.Migrator().HasTable(&RiskRecord{}))
	require.False(t, db.Migrator().HasTable(&RiskRecordGovernance{}))

	// When
	err := migrateDB()

	// Then
	require.NoError(t, err)
	require.True(t, db.Migrator().HasTable(&RiskRecord{}))
	require.True(t, db.Migrator().HasTable(&RiskRecordGovernance{}))
}
