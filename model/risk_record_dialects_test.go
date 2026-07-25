package model

import (
	"os"
	"testing"

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
		"id", "request_id", "channel_id", "user_id", "rule_ids", "provider_id", "provider_name", "result",
		"categories", "latency_ms", "prompt_tokens", "completion_tokens", "total_tokens", "neurons", "error_code", "observed_at",
	}

	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			// When
			db, err := gorm.Open(target.dialector, &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&RiskRecord{}))

			// Then
			require.True(t, db.Migrator().HasTable(&RiskRecord{}))
			for _, column := range expectedColumns {
				require.True(t, db.Migrator().HasColumn(&RiskRecord{}, column), "missing column %s", column)
			}
			sqlDB, err := db.DB()
			require.NoError(t, err)
			require.NoError(t, sqlDB.Close())
		})
	}
}
