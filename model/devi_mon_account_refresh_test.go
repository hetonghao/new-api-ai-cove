package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeviMonAccountRefreshDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := useSystemTaskMatrixDB(t, dialect)
			require.NoError(t, db.AutoMigrate(&DeviMonAccountRefresh{}))
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&DeviMonAccountRefresh{})) })
			for _, row := range []DeviMonAccountRefresh{{Email: "account@example.test", StartedAtUS: 5, Data: "first"}, {Email: "account@example.test", StartedAtUS: 15, Data: "newest"}, {Email: "account@example.test", StartedAtUS: 10, Data: "older request finished last"}, {Email: "other@example.test", StartedAtUS: 6, Data: "other"}} {
				require.NoError(t, SaveDeviMonAccountRefresh(context.Background(), &row))
			}
			require.NoError(t, DeleteDeviMonRefreshesBefore(15))
			rows, err := GetDeviMonRefreshesSince(0)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.Equal(t, "newest", rows[0].Data)
		})
	}
}

func TestDeviMonResetPolicyDatabaseMatrix(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db := useSystemTaskMatrixDB(t, dialect)
			require.NoError(t, db.AutoMigrate(&DeviMonResetPolicy{}, &SystemTaskLock{}))
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&DeviMonResetPolicy{}, &SystemTaskLock{})) })
			if dialect != "sqlite" {
				sqlDB, err := db.DB()
				require.NoError(t, err)
				sqlDB.SetMaxOpenConns(8)
			}
			ctx := context.Background()
			key := "account-1"
			require.NoError(t, SetDeviMonResetPolicy(ctx, key, true))
			owner := &SystemTask{Type: SystemTaskTypeDevinMonitor, TaskID: "monitor-task", LockedBy: "old-node"}
			lock := &SystemTaskLock{Type: owner.Type, TaskID: owner.TaskID, LockedBy: owner.LockedBy, LockedUntil: 999}
			require.NoError(t, db.Create(lock).Error)
			claimed, err := ClaimDeviMonReset(ctx, key, 2000, 1000, owner)
			require.NoError(t, err)
			require.False(t, claimed, "an expired lease cannot authorize a card reset")
			owner.LockedBy = "new-node"
			require.NoError(t, db.Model(lock).Updates(map[string]any{"locked_until": 1100, "locked_by": owner.LockedBy}).Error)
			results := make(chan bool, 8)
			start := make(chan struct{})
			for range cap(results) {
				go func() {
					<-start
					claimed, err := ClaimDeviMonReset(ctx, key, 2000, 1000, owner)
					if err != nil {
						t.Errorf("claim: %v", err)
					}
					results <- claimed
				}()
			}
			close(start)
			winners := 0
			for range cap(results) {
				if <-results {
					winners++
				}
			}
			require.Equal(t, 1, winners, "a weekly window must be claimed once across workers")
			require.NoError(t, SetDeviMonResetPolicy(ctx, key, false))
			claimed, err = ClaimDeviMonReset(ctx, key, 3000, 2200, nil)
			require.NoError(t, err)
			require.False(t, claimed, "disabled accounts cannot claim even the next window")
			require.NoError(t, SetDeviMonResetPolicy(ctx, key, true))
			claimed, err = ClaimDeviMonReset(ctx, key, 2000, 1200, nil)
			require.NoError(t, err)
			require.False(t, claimed, "toggling must not lose the existing claim")
			claimed, err = ClaimDeviMonReset(ctx, key, 3000, 2200, nil)
			require.NoError(t, err)
			require.True(t, claimed, "a later window is allowed")
		})
	}
}
