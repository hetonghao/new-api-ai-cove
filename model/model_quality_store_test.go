package model

import (
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var qualityStoreTestTables = []any{
	&ModelQualitySettings{}, &ModelQualityCase{}, &ModelQualityRevision{},
	&ModelQualityRun{}, &ModelQualitySample{}, &ModelQualityArtifact{},
	&ModelQualityBudget{}, &Channel{}, &SystemTask{}, &SystemTaskLock{},
}

func setupQualityStoreTest(t *testing.T, dialect string) time.Time {
	t.Helper()
	var driver gorm.Dialector
	switch dialect {
	case "sqlite":
		driver = sqlite.Open(":memory:")
	case "mysql":
		dsn := os.Getenv("TEST_MYSQL_DSN")
		if dsn == "" {
			t.Skip("TEST_MYSQL_DSN is not configured")
		}
		driver = mysql.Open(dsn)
	case "postgres":
		dsn := os.Getenv("TEST_POSTGRES_DSN")
		if dsn == "" {
			t.Skip("TEST_POSTGRES_DSN is not configured")
		}
		driver = postgres.Open(dsn)
	default:
		t.Fatalf("unknown dialect %s", dialect)
	}
	db, err := gorm.Open(driver, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB := DB
	DB = db
	t.Cleanup(func() {
		DB = oldDB
		if dialect != "sqlite" {
			for _, table := range qualityStoreTestTables {
				_ = db.Migrator().DropTable(table)
			}
		}
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(qualityStoreTestTables...))
	require.NoError(t, seedQualitySettings(db))
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	cfg := DefaultQualitySettings()
	cfg.Enabled, cfg.TokenIDs = true, []int{1}
	require.NoError(t, SaveQualitySettings(cfg, 1, now))
	require.NoError(t, DB.Create(&SystemTask{TaskID: "executor", Type: ModelQualityExecuteTask, Status: SystemTaskStatusRunning, LockedBy: "runner"}).Error)
	require.NoError(t, DB.Create(&SystemTaskLock{Type: ModelQualityExecuteTask, TaskID: "executor", LockedBy: "runner", LockedUntil: now.Add(48 * time.Hour).Unix()}).Error)
	return now
}

func createQualityStoreCase(t *testing.T, now time.Time, samples int) ModelQualityCase {
	t.Helper()
	cfg := QualityConfig{Model: "gpt-6-astra", OutputType: "text", Mode: "channel", Protocol: "responses", TokenID: 1, Group: "default", ChannelIDs: []int{7}, Prompt: "test fixture", MaxOutputTokens: 100, SamplesPerTarget: samples, TimeoutSeconds: 180, DailyLimit: 10}
	body, fingerprint, err := QualityConfigFingerprint(cfg)
	require.NoError(t, err)
	c := ModelQualityCase{Name: "case", Enabled: true, Version: 1, EditVersion: 1, CreatedAt: now.UnixMilli()}
	require.NoError(t, DB.Create(&c).Error)
	require.NoError(t, DB.Create(&ModelQualityRevision{CaseID: c.ID, Version: 1, ConfigJSON: body, Fingerprint: fingerprint}).Error)
	return c
}

func createQualityStoreRun(t *testing.T, c ModelQualityCase, now time.Time) *ModelQualityRun {
	t.Helper()
	run, created, err := CreateQualityRun(c.ID, QualityRunRequest{}, 1, "request", "manual", 0, now)
	require.NoError(t, err)
	require.True(t, created)
	return run
}

func testModelQualityStoreIdempotencyUsesOriginalRequest(t *testing.T, dialect string) {
	now := setupQualityStoreTest(t, dialect)
	c := createQualityStoreCase(t, now, 1)
	run := createQualityStoreRun(t, c, now)
	require.NoError(t, DB.Model(&ModelQualityCase{}).Where("id = ?", c.ID).Updates(map[string]any{"version": 2, "enabled": false}).Error)
	replay, created, err := CreateQualityRun(c.ID, QualityRunRequest{}, 1, "request", "manual", 0, now)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, run.ID, replay.ID)
	_, _, err = CreateQualityRun(c.ID, QualityRunRequest{SamplesPerTarget: 2}, 1, "request", "manual", 0, now)
	assert.ErrorIs(t, err, ErrQualityConflict)
}

func testModelQualityStoreRejectsInvalidOverrides(t *testing.T, dialect string) {
	for _, req := range []QualityRunRequest{{SamplesPerTarget: -1}, {ChannelIDs: []int{7, 7}}, {Version: -1}} {
		t.Run(stringMustMarshalQualityRequest(t, req), func(t *testing.T) {
			now := setupQualityStoreTest(t, dialect)
			c := createQualityStoreCase(t, now, 1)
			_, _, err := CreateQualityRun(c.ID, req, 1, "invalid", "manual", 0, now)
			require.Error(t, err)
			var count int64
			require.NoError(t, DB.Model(&ModelQualityRun{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

func stringMustMarshalQualityRequest(t *testing.T, req QualityRunRequest) string {
	t.Helper()
	b, err := common.Marshal(req)
	require.NoError(t, err)
	return string(b)
}

func testModelQualityStoreRevocationCommitsSkippedSamples(t *testing.T, dialect string) {
	now := setupQualityStoreTest(t, dialect)
	c := createQualityStoreCase(t, now, 2)
	run := createQualityStoreRun(t, c, now)
	cfg, version, err := GetQualitySettings()
	require.NoError(t, err)
	cfg.TokenIDs = []int{2}
	require.NoError(t, SaveQualitySettings(cfg, version, now))
	_, _, err = ClaimQualitySample("executor", nil, now)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, samples, err := GetQualityRun(run.ID)
	require.NoError(t, err)
	for _, s := range samples {
		assert.Equal(t, "skipped", s.Status)
		assert.Zero(t, s.StartedAt)
	}
	var stored ModelQualityCase
	require.NoError(t, DB.First(&stored, c.ID).Error)
	assert.Empty(t, stored.ActiveRunID)
	var budget ModelQualityBudget
	require.NoError(t, DB.First(&budget, "id = ?", qualityGlobalBudgetID(qualityBudgetDay(now))).Error)
	assert.Zero(t, budget.Reserved)
	assert.Zero(t, budget.Started)
}

func testModelQualityStoreCrossDayExhaustionReleasesReservation(t *testing.T, dialect string) {
	for _, scope := range []string{"global", "case"} {
		t.Run(scope, func(t *testing.T) {
			now := setupQualityStoreTest(t, dialect)
			c := createQualityStoreCase(t, now, 1)
			run := createQualityStoreRun(t, c, now)
			tomorrow := now.Add(24 * time.Hour)
			day := qualityBudgetDay(tomorrow)
			id, limit := qualityGlobalBudgetID(day), 200
			if scope == "case" {
				id, limit = qualityCaseBudgetID(c.ID, day), 10
			}
			require.NoError(t, DB.Create(&ModelQualityBudget{ID: id, Day: day, Started: limit}).Error)
			_, _, err := ClaimQualitySample("executor", nil, tomorrow)
			require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			_, samples, err := GetQualityRun(run.ID)
			require.NoError(t, err)
			assert.Equal(t, "skipped", samples[0].Status)
			var old ModelQualityBudget
			require.NoError(t, DB.First(&old, "id = ?", qualityGlobalBudgetID(qualityBudgetDay(now))).Error)
			assert.Zero(t, old.Reserved)
			var rows []ModelQualityBudget
			require.NoError(t, DB.Where("day = ?", day).Find(&rows).Error)
			for _, row := range rows {
				if row.ID != id {
					assert.Zero(t, row.Started)
				}
			}
		})
	}
}

func testModelQualityStoreFairClaimsAndActiveRunGuard(t *testing.T, dialect string) {
	now := setupQualityStoreTest(t, dialect)
	a := createQualityStoreCase(t, now, 2)
	b := createQualityStoreCase(t, now, 1)
	createQualityStoreRun(t, a, now)
	createQualityStoreRun(t, b, now.Add(time.Millisecond))
	_, _, err := CreateQualityRun(a.ID, QualityRunRequest{}, 1, "different", "manual", 0, now)
	require.ErrorIs(t, err, ErrQualityConflict)
	for _, want := range []int64{a.ID, b.ID, a.ID} {
		sample, _, err := ClaimQualitySample("executor", nil, now)
		require.NoError(t, err)
		assert.Equal(t, want, sample.CaseID)
		assert.Equal(t, "executor", sample.ExecutorID)
	}
}

func testModelQualityStoreFinishPreservesAttributionAndRejectsOversize(t *testing.T, dialect string) {
	now := setupQualityStoreTest(t, dialect)
	c := createQualityStoreCase(t, now, 1)
	createQualityStoreRun(t, c, now)
	sample, _, err := ClaimQualitySample("executor", nil, now)
	require.NoError(t, err)
	require.NoError(t, FinishQualitySample(sample.ID, "executor", QualityResult{Status: "succeeded", SVG: []byte(strings.Repeat("x", QualityMaxSVGBytes+1))}, now))
	var stored ModelQualitySample
	require.NoError(t, DB.First(&stored, sample.ID).Error)
	assert.Equal(t, 7, stored.ChannelID)
	assert.Equal(t, "failed", stored.Status)
	assert.Equal(t, "artifact_too_large", stored.ErrorCode)
	_, err = GetQualityArtifact(sample.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.ErrorIs(t, FinishQualitySample(sample.ID, "other", QualityResult{Status: "failed"}, now), ErrQualityOwnership)
}

func testModelQualityStoreRetentionProtectsPinnedArtifacts(t *testing.T, dialect string) {
	now := setupQualityStoreTest(t, dialect)
	cfg, version, err := GetQualitySettings()
	require.NoError(t, err)
	cfg.RetentionEnabled = true
	require.NoError(t, SaveQualitySettings(cfg, version, now))
	for i, pinned := range []bool{true, false} {
		s := ModelQualitySample{RunID: "fixture", Ordinal: i, Status: "succeeded", Pinned: pinned, CreatedAt: now.UnixMilli()}
		require.NoError(t, DB.Create(&s).Error)
		require.NoError(t, DB.Create(&ModelQualityArtifact{SampleID: s.ID, Text: []byte("fixture"), ExpiresAt: now.Add(-time.Hour).UnixMilli()}).Error)
	}
	require.NoError(t, CleanupQualityRetention(now))
	var samples []ModelQualitySample
	require.NoError(t, DB.Order("id ASC").Find(&samples).Error)
	assert.False(t, samples[0].ArtifactExpired)
	assert.True(t, samples[1].ArtifactExpired)
	_, err = GetQualityArtifact(samples[0].ID)
	assert.NoError(t, err)
	_, err = GetQualityArtifact(samples[1].ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func testModelQualityStoreScheduleDeduplicatesAndSkipsBusySlots(t *testing.T, dialect string) {
	now := setupQualityStoreTest(t, dialect)
	c := createQualityStoreCase(t, now, 1)
	schedule := QualitySchedule{Enabled: true, Kind: "interval", IntervalMinutes: 5, Timezone: "UTC"}
	body, err := common.Marshal(schedule)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&ModelQualityCase{}).Where("id = ?", c.ID).Updates(map[string]any{"schedule_json": string(body), "schedule_version": 1, "next_run_at": now.UnixMilli()}).Error)
	require.NoError(t, ScheduleQualityRuns(now))
	require.NoError(t, ScheduleQualityRuns(now))
	var runs []ModelQualityRun
	require.NoError(t, DB.Find(&runs).Error)
	require.Len(t, runs, 1)
	assert.Equal(t, "scheduled", runs[0].Source)
	next := now.Add(5 * time.Minute)
	require.NoError(t, ScheduleQualityRuns(next))
	var stored ModelQualityCase
	require.NoError(t, DB.First(&stored, c.ID).Error)
	assert.Equal(t, "busy", stored.LastScheduleReason)
	assert.Equal(t, now.Add(10*time.Minute).UnixMilli(), stored.NextRunAt)
	require.NoError(t, CancelQualityRun(runs[0].ID, next))
	require.NoError(t, ScheduleQualityRuns(now.Add(30*time.Minute)))
	require.NoError(t, DB.First(&stored, c.ID).Error)
	assert.Equal(t, "missed", stored.LastScheduleReason)
	assert.Equal(t, now.Add(35*time.Minute).UnixMilli(), stored.NextRunAt)
	require.NoError(t, DB.Find(&runs).Error)
	assert.Len(t, runs, 1)
}

func testModelQualityStoreLeaseLossCannotClaimOrOverwriteRecovery(t *testing.T, dialect string) {
	now := setupQualityStoreTest(t, dialect)
	c := createQualityStoreCase(t, now, 2)
	run := createQualityStoreRun(t, c, now)
	sample, _, err := ClaimQualitySample("executor", nil, now)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&SystemTaskLock{}).Where("task_id = ?", "executor").Update("locked_until", now.Add(-time.Second).Unix()).Error)
	_, _, err = ClaimQualitySample("executor", nil, now)
	assert.ErrorIs(t, err, ErrQualityOwnership)
	require.NoError(t, RecoverQualityWork("next-executor", now))
	assert.ErrorIs(t, FinishQualitySample(sample.ID, "executor", QualityResult{Status: "succeeded"}, now), ErrQualityOwnership)
	_, samples, err := GetQualityRun(run.ID)
	require.NoError(t, err)
	assert.Equal(t, "interrupted", samples[0].Status)
	assert.Equal(t, "pending", samples[1].Status)
	var budget ModelQualityBudget
	require.NoError(t, DB.First(&budget, "id = ?", qualityGlobalBudgetID(qualityBudgetDay(now))).Error)
	assert.Equal(t, 1, budget.Started)
	assert.Equal(t, 1, budget.Reserved)
}

func testModelQualityStorePreflightSkipRefundsOnlyUnsentSample(t *testing.T, dialect string) {
	now := setupQualityStoreTest(t, dialect)
	c := createQualityStoreCase(t, now, 1)
	createQualityStoreRun(t, c, now)
	sample, _, err := ClaimQualitySample("executor", nil, now)
	require.NoError(t, err)
	require.NoError(t, FinishQualitySample(sample.ID, "executor", QualityResult{Status: "skipped", ErrorCode: "invalid_configuration"}, now))
	var budget ModelQualityBudget
	require.NoError(t, DB.First(&budget, "id = ?", qualityGlobalBudgetID(qualityBudgetDay(now))).Error)
	assert.Zero(t, budget.Started)
	assert.Zero(t, budget.Reserved)
	var stored ModelQualitySample
	require.NoError(t, DB.First(&stored, sample.ID).Error)
	assert.Zero(t, stored.StartedAt)
}

func testModelQualityStoreKillSwitchCancelsPendingAndReleasesBudget(t *testing.T, dialect string) {
	now := setupQualityStoreTest(t, dialect)
	c := createQualityStoreCase(t, now, 2)
	run := createQualityStoreRun(t, c, now)
	cfg, version, err := GetQualitySettings()
	require.NoError(t, err)
	cfg.Enabled = false
	require.NoError(t, SaveQualitySettings(cfg, version, now))
	_, samples, err := GetQualityRun(run.ID)
	require.NoError(t, err)
	for _, sample := range samples {
		assert.Equal(t, "cancelled", sample.Status)
	}
	var budget ModelQualityBudget
	require.NoError(t, DB.First(&budget, "id = ?", qualityGlobalBudgetID(qualityBudgetDay(now))).Error)
	assert.Zero(t, budget.Reserved)
	var stored ModelQualityCase
	require.NoError(t, DB.First(&stored, c.ID).Error)
	assert.Empty(t, stored.ActiveRunID)
}

func TestModelQualityStoreDialects(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T, dialect string)
	}{
		{"idempotency_uses_original_request", testModelQualityStoreIdempotencyUsesOriginalRequest},
		{"rejects_invalid_overrides", testModelQualityStoreRejectsInvalidOverrides},
		{"revocation_commits_skipped_samples", testModelQualityStoreRevocationCommitsSkippedSamples},
		{"cross_day_exhaustion_releases_reservation", testModelQualityStoreCrossDayExhaustionReleasesReservation},
		{"fair_claims_and_active_run_guard", testModelQualityStoreFairClaimsAndActiveRunGuard},
		{"finish_preserves_attribution_and_rejects_oversize", testModelQualityStoreFinishPreservesAttributionAndRejectsOversize},
		{"retention_protects_pinned_artifacts", testModelQualityStoreRetentionProtectsPinnedArtifacts},
		{"schedule_deduplicates_and_skips_busy_slots", testModelQualityStoreScheduleDeduplicatesAndSkipsBusySlots},
		{"lease_loss_cannot_claim_or_overwrite_recovery", testModelQualityStoreLeaseLossCannotClaimOrOverwriteRecovery},
		{"preflight_skip_refunds_only_unsent_sample", testModelQualityStorePreflightSkipRefundsOnlyUnsentSample},
		{"kill_switch_cancels_pending_and_releases_budget", testModelQualityStoreKillSwitchCancelsPendingAndReleasesBudget},
		{"list_runs_filters_and_paginates", testModelQualityStoreListQualityRuns},
		{"delete_keeps_history_and_guards_active_run", testModelQualityStoreDeleteCase},
		{"all_versions_scope_covers_every_revision", testModelQualityStoreAllVersionsScope},
	}
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) { tc.run(t, dialect) })
			}
		})
	}
}

func TestModelQualityMigrationIdempotentAndLargeArtifact(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			setupQualityStoreTest(t, dialect)
			var version string
			if dialect == "sqlite" {
				require.NoError(t, DB.Raw("select sqlite_version()").Scan(&version).Error)
			} else {
				require.NoError(t, DB.Raw("select version()").Scan(&version).Error)
			}
			t.Logf("%s version: %s", dialect, version)
			if dialect == "mysql" {
				type columnInfo struct {
					Field string `gorm:"column:Field"`
					Type  string `gorm:"column:Type"`
				}
				var cols []columnInfo
				require.NoError(t, DB.Raw("SHOW COLUMNS FROM model_quality_artifacts WHERE Field IN ('text','svg')").Scan(&cols).Error)
				for _, col := range cols {
					t.Logf("mysql model_quality_artifacts.%s type: %s", col.Field, col.Type)
				}
			}
			recorder := &migrationSQLRecorder{}
			db := DB.Session(&gorm.Session{Logger: recorder})
			require.NoError(t, db.AutoMigrate(qualityStoreTestTables...))
			assert.Empty(t, recorder.schemaMutations())
			text := make([]byte, 1<<20)
			svg := make([]byte, 1<<19)
			_, err := rand.Read(text)
			require.NoError(t, err)
			_, err = rand.Read(svg)
			require.NoError(t, err)
			sample := ModelQualitySample{RunID: "fixture", Ordinal: 1, Status: "succeeded", CreatedAt: 1}
			require.NoError(t, DB.Create(&sample).Error)
			require.NoError(t, DB.Create(&ModelQualityArtifact{SampleID: sample.ID, Text: text, SVG: svg, SHA256: "fixture"}).Error)
			artifact, err := GetQualityArtifact(sample.ID)
			require.NoError(t, err)
			assert.Equal(t, text, artifact.Text)
			assert.Equal(t, svg, artifact.SVG)
		})
	}
}

func testModelQualityStoreListQualityRuns(t *testing.T, dialect string) {
	now := setupQualityStoreTest(t, dialect)
	c := createQualityStoreCase(t, now, 1)
	seedRun := func(id, source, status string, createdAt time.Time) *ModelQualityRun {
		run := &ModelQualityRun{
			ID: id, CaseID: c.ID, Version: 1, Source: source,
			Status: status, IdempotencyKey: id, CreatedBy: 1,
			CreatedAt: createdAt.UnixMilli(), SampleCount: 1,
		}
		require.NoError(t, DB.Create(run).Error)
		return run
	}
	first := seedRun("run-first", "scheduled", "completed", now.Add(-2*time.Hour))
	second := seedRun("run-second", "manual", "completed", now.Add(-time.Hour))
	third := seedRun("run-third", "manual", "running", now)

	runs, total, err := ListQualityRuns(QualityRunFilter{Status: "completed"}, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, runs, 2)
	assert.Equal(t, second.ID, runs[0].ID)
	assert.Equal(t, first.ID, runs[1].ID)

	runs, total, err = ListQualityRuns(QualityRunFilter{Source: "manual"}, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, runs, 2)
	assert.Equal(t, third.ID, runs[0].ID)

	runs, total, err = ListQualityRuns(QualityRunFilter{
		Start: now.Add(-90 * time.Minute).UnixMilli(),
		End:   now.Add(-30 * time.Minute).UnixMilli(),
	}, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, runs, 1)
	assert.Equal(t, second.ID, runs[0].ID)

	runs, total, err = ListQualityRuns(QualityRunFilter{}, 1, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, runs, 1)
	assert.Equal(t, second.ID, runs[0].ID)
}

func testModelQualityStoreDeleteCase(t *testing.T, dialect string) {
	now := setupQualityStoreTest(t, dialect)
	c := createQualityStoreCase(t, now, 1)
	run := createQualityStoreRun(t, c, now)
	assert.ErrorIs(t, DeleteQualityCase(c.ID, 0), ErrQualityConflict)
	assert.ErrorIs(t, DeleteQualityCase(c.ID, 999), ErrQualityConflict)
	require.NoError(t, CancelQualityRun(run.ID, now))
	require.NoError(t, DeleteQualityCase(c.ID, c.EditVersion))
	_, err := GetQualityCase(c.ID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	revisions, err := ListQualityRevisions(c.ID)
	require.NoError(t, err)
	assert.Len(t, revisions, 1)
	stored, samples, err := GetQualityRun(run.ID)
	require.NoError(t, err)
	assert.Equal(t, c.ID, stored.CaseID)
	assert.Len(t, samples, 1)
}

func testModelQualityStoreAllVersionsScope(t *testing.T, dialect string) {
	now := setupQualityStoreTest(t, dialect)
	c := createQualityStoreCase(t, now, 1)
	cfg := QualityConfig{Model: "gpt-6-astra", OutputType: "text", Mode: "channel", Protocol: "responses", TokenID: 1, Group: "default", ChannelIDs: []int{7}, Prompt: "test fixture v2", MaxOutputTokens: 100, SamplesPerTarget: 1, TimeoutSeconds: 180, DailyLimit: 10}
	body, fingerprint, err := QualityConfigFingerprint(cfg)
	require.NoError(t, err)
	require.NoError(t, DB.Create(&ModelQualityRevision{CaseID: c.ID, Version: 2, ConfigJSON: body, Fingerprint: fingerprint}).Error)
	require.NoError(t, DB.Model(&ModelQualityCase{}).Where("id = ?", c.ID).Update("version", 2).Error)
	duration := int64(500)
	for i, version := range []int{1, 2} {
		status := "succeeded"
		if version == 2 {
			status = "failed"
		}
		require.NoError(t, DB.Create(&ModelQualitySample{
			RunID: fmt.Sprintf("run-v%d", version), Ordinal: i, CaseID: c.ID, Version: version,
			ChannelID: 7, ChannelName: "chan", Status: status, DurationMs: &duration,
			StartedAt: now.Add(-time.Hour).UnixMilli(), CreatedAt: now.Add(-time.Hour).UnixMilli(),
		}).Error)
	}

	all, err := QualityDashboard(c.ID, -1, 7, now)
	require.NoError(t, err)
	assert.Equal(t, 0, all.Version)
	assert.Equal(t, 1, all.Summary.Success)
	assert.Equal(t, 1, all.Summary.Failure)
	pinned, err := QualityDashboard(c.ID, 2, 7, now)
	require.NoError(t, err)
	assert.Equal(t, 2, pinned.Version)
	assert.Equal(t, 0, pinned.Summary.Success)
	assert.Equal(t, 1, pinned.Summary.Failure)

	samples, err := QualitySamples(c.ID, -1, 7, 0, 0, 0, 0, 0)
	require.NoError(t, err)
	assert.Len(t, samples, 2)
	samples, err = QualitySamples(c.ID, 1, 7, 0, 0, 0, 0, 0)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, 1, samples[0].Version)
}
