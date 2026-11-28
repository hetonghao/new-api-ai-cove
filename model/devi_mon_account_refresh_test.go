package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
			previousType := common.MainDatabaseType()
			common.SetMainDatabaseType(common.DatabaseType(dialect))
			t.Cleanup(func() { common.SetMainDatabaseType(previousType) })
			// Exercise both a fresh install and an upgrade from the released schema.
			for range 2 {
				require.NoError(t, db.AutoMigrate(&DeviMonResetPolicy{}))
			}
			ctx := t.Context()
			require.NoError(t, SetDeviMonResetPolicy(ctx, "new", DeviMonSilentOnExhaustion))
			require.NoError(t, db.Migrator().DropTable(&DeviMonResetPolicy{}))
			type releasedPolicy struct {
				AccountKey             string `gorm:"type:varchar(64);primaryKey"`
				Enabled                bool   `gorm:"not null;default:false"`
				KeepDisabledAfterReset bool   `gorm:"not null;default:false"`
				PausedCredential       string `gorm:"type:varchar(64);not null;default:''"`
				PausedWindowEnd        int64  `gorm:"not null;default:0"`
				AttemptedWindowEnd     int64  `gorm:"not null;default:0"`
				AttemptedAt            int64  `gorm:"not null;default:0"`
				HoldExhaustedUsage     bool   `gorm:"not null;default:false"`
			}
			table := db.NamingStrategy.TableName("DeviMonResetPolicy")
			require.NoError(t, db.Table(table).AutoMigrate(&releasedPolicy{}))
			require.NoError(t, db.Table(table).Create(&releasedPolicy{AccountKey: "legacy-card", Enabled: true, AttemptedWindowEnd: 800, AttemptedAt: 600, HoldExhaustedUsage: true}).Error)
			require.NoError(t, db.Table(table).Create(&releasedPolicy{AccountKey: "legacy-disable", KeepDisabledAfterReset: true}).Error)
			for range 2 {
				require.NoError(t, db.AutoMigrate(&DeviMonResetPolicy{}, &SystemTaskLock{}))
			}
			read := func(key string) DeviMonResetPolicy {
				t.Helper()
				rows, err := GetDeviMonResetPolicies(ctx)
				require.NoError(t, err)
				return rows[key]
			}
			require.Equal(t, DeviMonDisableOnExhaustion, read("legacy-disable").ExhaustionAction())
			require.True(t, read("legacy-disable").KeepDisabledAfterReset)
			old := read("legacy-card")
			require.Equal(t, DeviMonUseCardOnExhaustion, old.ExhaustionAction())
			require.True(t, old.Enabled)
			require.False(t, old.KeepDisabledAfterReset)
			require.True(t, old.HoldExhaustedUsage)
			require.EqualValues(t, 800, old.AttemptedWindowEnd)
			require.EqualValues(t, 600, old.AttemptedAt)
			require.NoError(t, SetDeviMonResumeAfterReset(ctx, "legacy-card", false))
			old = read("legacy-card")
			require.True(t, old.KeepDisabledAfterReset)
			require.True(t, old.Enabled)
			require.NoError(t, SetDeviMonResetPolicy(ctx, "legacy-card", DeviMonSilentOnExhaustion))
			require.ErrorIs(t, SetDeviMonResetPolicy(ctx, "legacy-card", "unknown"), ErrDeviMonInvalidPolicy)
			silent := read("legacy-card")
			require.Equal(t, DeviMonSilentOnExhaustion, silent.ExhaustionAction())
			require.True(t, silent.KeepDisabledAfterReset)
			require.EqualValues(t, 800, silent.AttemptedWindowEnd)
			claimed, err := ClaimDeviMonReset(ctx, "legacy-card", 2000, 1500, nil)
			require.NoError(t, err)
			require.False(t, claimed)
			require.NoError(t, SetDeviMonResetPolicy(ctx, "legacy-card", DeviMonDisableOnExhaustion))
			require.Equal(t, DeviMonDisableOnExhaustion, read("legacy-card").ExhaustionAction())
			require.True(t, read("legacy-card").KeepDisabledAfterReset)
			require.NoError(t, UpdateDeviMonQuotaPause(ctx, "owned", nil, func(p *DeviMonResetPolicy) error { p.PausedWindowEnd, p.PausedCredential = 42, "devin-id"; return nil }))
			require.EqualValues(t, 42, read("owned").PausedWindowEnd)
			require.NoError(t, ForgetDeviMonQuotaPause(ctx, "devin-id"))
			require.Empty(t, read("owned").PausedCredential)
			t.Cleanup(func() { require.NoError(t, db.Migrator().DropTable(&DeviMonResetPolicy{}, &SystemTaskLock{})) })
			if dialect != "sqlite" {
				sqlDB, err := db.DB()
				require.NoError(t, err)
				sqlDB.SetMaxOpenConns(8)
			}
			key := "account-1"
			require.NoError(t, SetDeviMonResetPolicy(ctx, key, DeviMonUseCardOnExhaustion))
			owner := &SystemTask{Type: SystemTaskTypeDevinMonitor, TaskID: "monitor-task", LockedBy: "old-node"}
			lock := &SystemTaskLock{Type: owner.Type, TaskID: owner.TaskID, LockedBy: owner.LockedBy, LockedUntil: 999}
			require.NoError(t, db.Create(lock).Error)
			claimed, err = ClaimDeviMonReset(ctx, key, 2000, 1000, owner)
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
			require.NoError(t, SetDeviMonResetPolicy(ctx, key, DeviMonDisableOnExhaustion))
			claimed, err = ClaimDeviMonReset(ctx, key, 3000, 2200, nil)
			require.NoError(t, err)
			require.False(t, claimed, "disabled accounts cannot claim even the next window")
			require.NoError(t, SetDeviMonResetPolicy(ctx, key, DeviMonUseCardOnExhaustion))
			claimed, err = ClaimDeviMonReset(ctx, key, 2000, 1200, nil)
			require.NoError(t, err)
			require.False(t, claimed, "toggling must not lose the existing claim")
			claimed, err = ClaimDeviMonReset(ctx, key, 3000, 2200, nil)
			require.NoError(t, err)
			require.True(t, claimed, "a later window is allowed")
		})
	}
}
