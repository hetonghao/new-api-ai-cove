package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLogRequestQueryPlan_uses_request_id_index(t *testing.T) {
	// Given
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Log{}))
	require.True(t, db.Migrator().HasIndex(&Log{}, "idx_logs_request_id"))

	// When
	var planRows []struct {
		Detail string `gorm:"column:detail"`
	}
	err = db.Raw("EXPLAIN QUERY PLAN SELECT * FROM logs WHERE request_id = ? LIMIT 1", "target-request").Scan(&planRows).Error

	// Then
	require.NoError(t, err)
	require.NotEmpty(t, planRows)
	plan := strings.ToLower(planRows[0].Detail)
	t.Logf("request ID query plan: %s", planRows[0].Detail)
	require.Contains(t, plan, "search logs using index idx_logs_request_id")
	require.NotContains(t, plan, "scan logs")
}
