package model

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRiskRecordMigration_supportsConfiguredDatabases(t *testing.T) {
	// Given
	targets := []struct {
		name      string
		dialector gorm.Dialector
	}{
		{name: "sqlite", dialector: sqlite.Open("file:risk-record-dialects?mode=memory&cache=shared")},
	}
	if dsn := os.Getenv("RISK_RECORD_TEST_MYSQL_DSN"); dsn != "" {
		targets = append(targets, struct {
			name      string
			dialector gorm.Dialector
		}{name: "mysql", dialector: mysql.Open(dsn)})
	}
	if dsn := os.Getenv("RISK_RECORD_TEST_POSTGRES_DSN"); dsn != "" {
		targets = append(targets, struct {
			name      string
			dialector gorm.Dialector
		}{name: "postgres", dialector: postgres.Open(dsn)})
	}
	expectedColumns := []string{
		"id", "request_id", "channel_id", "user_id", "token_id", "model", "path", "preview", "content_hash",
		"rule_ids", "provider_id", "provider_name", "result", "categories", "latency_ms", "prompt_tokens",
		"completion_tokens", "total_tokens", "neurons", "chunks", "error_code", "error_detail", "source", "cache_hit", "provider_called", "blocked", "observed_at",
	}
	expectedIndexes := []string{
		"idx_risk_records_channel_id",
		"idx_risk_records_user_id",
		"idx_risk_records_provider_id",
		"idx_risk_records_result",
		"idx_risk_records_source",
		"idx_risk_records_observed_at",
	}

	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			// When
			db, err := gorm.Open(target.dialector, &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&RiskRecord{}, &RiskRecordGovernance{}))

			// Then
			require.True(t, db.Migrator().HasTable(&RiskRecord{}))
			require.True(t, db.Migrator().HasTable(&RiskRecordGovernance{}))
			for _, column := range expectedColumns {
				require.True(t, db.Migrator().HasColumn(&RiskRecord{}, column), "missing column %s", column)
			}
			for _, index := range expectedIndexes {
				require.True(t, db.Migrator().HasIndex(&RiskRecord{}, index), "missing index %s", index)
			}
			require.NoError(t, db.Exec("DELETE FROM risk_records").Error)
			cutoff := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
			for index, observedAt := range []time.Time{cutoff.Add(-time.Second), cutoff} {
				require.NoError(t, db.Create(&RiskRecord{
					RequestID: string(rune('a' + index)), ChannelID: 1, UserID: 1,
					RuleIDs: []int{}, ProviderID: 1, ProviderName: "provider",
					Result: RiskRecordResultSafe, Categories: []string{}, Chunks: []RiskRecordChunk{}, Source: RiskRecordSourceProvider,
					ObservedAt: observedAt,
				}).Error)
			}
			originalDB := DB
			DB = db
			deleted, err := DeleteExpiredRiskRecordsBatch(context.Background(), cutoff, 500)
			DB = originalDB
			require.NoError(t, err)
			require.EqualValues(t, 1, deleted)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Close())
		})
	}
}
