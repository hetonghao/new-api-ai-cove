package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Storage seam for the model quality inspection feature. All timestamps are
// Unix milliseconds unless noted; SystemTask lease fields stay in seconds.

const (
	qualitySettingsRowID           = 1
	qualityRunSourceManual         = "manual"
	qualityRunSourceSchedule       = "scheduled"
	qualitySampleStatusPending     = "pending"
	qualitySampleStatusRequesting  = "requesting"
	qualitySampleStatusSucceeded   = "succeeded"
	qualitySampleStatusFailed      = "failed"
	qualitySampleStatusCancelled   = "cancelled"
	qualitySampleStatusInterrupted = "interrupted"
	qualitySampleStatusSkipped     = "skipped"
	qualityArtifactValidation      = "svg-static-v1"
	qualityPinnedPerCaseLimit      = 20
	qualityRetentionBatch          = 200
)

func qualityTransaction(fn func(*gorm.DB) error) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var settings ModelQualitySettings
		if err := lockForUpdate(tx).First(&settings, qualitySettingsRowID).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return fn(tx)
	})
}

func qualityBudgetDay(now time.Time) string {
	return now.UTC().Format("2006-01-02")
}

func qualityGlobalBudgetID(day string) string { return "global:" + day }

func qualityCaseBudgetID(caseID int64, day string) string {
	return fmt.Sprintf("case:%d:%s", caseID, day)
}

// ---------------------------------------------------------------------------
// Settings (singleton row, optimistic version)
// ---------------------------------------------------------------------------

// seedQualitySettings inserts the default singleton row on first migration
// only; an existing row is never overridden.
func seedQualitySettings(db *gorm.DB) error {
	body, err := common.Marshal(DefaultQualitySettings())
	if err != nil {
		return err
	}
	row := ModelQualitySettings{
		ID:        qualitySettingsRowID,
		Version:   1,
		Config:    string(body),
		UpdatedAt: time.Now().UnixMilli(),
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

func GetQualitySettings() (QualitySettingsConfig, int64, error) {
	var row ModelQualitySettings
	err := DB.First(&row, qualitySettingsRowID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DefaultQualitySettings(), 0, nil
	}
	if err != nil {
		return QualitySettingsConfig{}, 0, err
	}
	cfg := DefaultQualitySettings()
	if err := common.UnmarshalJsonStr(row.Config, &cfg); err != nil {
		return QualitySettingsConfig{}, 0, err
	}
	return cfg, row.Version, nil
}

func SaveQualitySettings(cfg QualitySettingsConfig, expectedVersion int64, now time.Time) error {
	if err := ValidateQualitySettings(cfg); err != nil {
		return err
	}
	body, err := common.Marshal(cfg)
	if err != nil {
		return err
	}
	return qualityTransaction(func(tx *gorm.DB) error {
		var row ModelQualitySettings
		err := lockForUpdate(tx).First(&row, qualitySettingsRowID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if expectedVersion != 0 {
				return ErrQualityConflict
			}
			row = ModelQualitySettings{
				ID:        qualitySettingsRowID,
				Version:   1,
				Config:    string(body),
				UpdatedAt: now.UnixMilli(),
			}
			return tx.Create(&row).Error
		}
		if err != nil {
			return err
		}
		if row.Version != expectedVersion {
			return ErrQualityConflict
		}
		res := tx.Model(&ModelQualitySettings{}).
			Where("id = ? AND version = ?", qualitySettingsRowID, expectedVersion).
			Updates(map[string]any{
				"version":    expectedVersion + 1,
				"config":     string(body),
				"updated_at": now.UnixMilli(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrQualityConflict
		}
		if !cfg.Enabled {
			var caseIDs []int64
			if err := tx.Model(&ModelQualitySample{}).Where("status = ?", qualitySampleStatusPending).Distinct().Pluck("case_id", &caseIDs).Error; err != nil {
				return err
			}
			for _, id := range caseIDs {
				if err := cancelQualityPendingTx(tx, id, now); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// Cases / revisions
// ---------------------------------------------------------------------------

func decodeQualityCase(c *ModelQualityCase) (*QualityCaseView, error) {
	view := &QualityCaseView{ModelQualityCase: *c}
	var revision ModelQualityRevision
	if err := DB.Where("case_id = ? AND version = ?", c.ID, c.Version).First(&revision).Error; err != nil {
		return nil, err
	}
	if err := common.UnmarshalJsonStr(revision.ConfigJSON, &view.Config); err != nil {
		return nil, err
	}
	if c.ScheduleJSON != "" {
		if err := common.UnmarshalJsonStr(c.ScheduleJSON, &view.Schedule); err != nil {
			return nil, err
		}
	}
	return view, nil
}

func ListQualityCases() ([]QualityCaseView, error) {
	var rows []ModelQualityCase
	if err := DB.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]QualityCaseView, 0, len(rows))
	for i := range rows {
		view, err := decodeQualityCase(&rows[i])
		if err != nil {
			return nil, err
		}
		views = append(views, *view)
	}
	return views, nil
}

func GetQualityCase(id int64) (*QualityCaseView, error) {
	var row ModelQualityCase
	if err := DB.First(&row, id).Error; err != nil {
		return nil, err
	}
	return decodeQualityCase(&row)
}

func ListQualityRevisions(caseID int64) ([]ModelQualityRevision, error) {
	var rows []ModelQualityRevision
	err := DB.Where("case_id = ?", caseID).Order("version DESC").Find(&rows).Error
	return rows, err
}

// validateQualityCaseWrite enforces the write contract shared by create/update.
func validateQualityCaseWrite(input QualityCaseWrite) error {
	if strings.TrimSpace(input.Name) == "" || len(input.Name) > 128 || len(input.Description) > 4096 {
		return errors.New("invalid case name or description length")
	}
	if err := ValidateQualityConfig(input.Config); err != nil {
		return err
	}
	// Schedule fields are validated through NextQualitySchedule even when the
	// schedule is disabled, using safe defaults for empty fields so a half-set
	// disabled schedule does not block saving.
	probe := input.Schedule
	if !probe.Enabled {
		probe.Enabled = true
		if probe.Kind == "" {
			probe.Kind = "interval"
		}
		if probe.IntervalMinutes == 0 {
			probe.IntervalMinutes = 60
		}
		if probe.Timezone == "" {
			probe.Timezone = "UTC"
		}
	}
	_, err := NextQualitySchedule(probe, time.Now(), time.Time{})
	return err
}

func qualityScheduleJSON(schedule QualitySchedule) (string, error) {
	body, err := common.Marshal(schedule)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func qualityNextRunAt(schedule QualitySchedule, now time.Time) (int64, error) {
	if !schedule.Enabled {
		return 0, nil
	}
	next, err := NextQualitySchedule(schedule, now, time.Time{})
	if err != nil {
		return 0, err
	}
	return next.UnixMilli(), nil
}

// qualityEnabledAllowed gates enabling a case on the global kill switch and
// the approved execution token list.
func qualityEnabledAllowed(tx *gorm.DB, cfg QualityConfig) error {
	settings, _, err := getQualitySettingsTx(tx)
	if err != nil {
		return err
	}
	if !settings.Enabled || !slices.Contains(settings.TokenIDs, cfg.TokenID) {
		return ErrQualityDisabled
	}
	return nil
}

func auditQualityCase(actor int, content string) {
	role := 0
	if user, err := GetUserById(actor, false); err == nil {
		role = user.Role
	}
	RecordAuditLog(nil, AuditLog{
		UserId:    actor,
		ActorRole: role,
		Category:  AuditCategoryOperation,
		Action:    "model_quality",
		Content:   content,
		Success:   true,
	})
}

func SaveQualityCase(id int64, input QualityCaseWrite, actor int, now time.Time) (*QualityCaseView, error) {
	if err := validateQualityCaseWrite(input); err != nil {
		return nil, err
	}
	configJSON, fingerprint, err := QualityConfigFingerprint(input.Config)
	if err != nil {
		return nil, err
	}
	scheduleJSON, err := qualityScheduleJSON(input.Schedule)
	if err != nil {
		return nil, err
	}

	var savedID int64
	var enabledChanged, scheduleChanged bool
	err = qualityTransaction(func(tx *gorm.DB) error {
		if input.Enabled {
			if err := qualityEnabledAllowed(tx, input.Config); err != nil {
				return err
			}
		}
		nowMs := now.UnixMilli()
		if id == 0 {
			nextRunAt, err := qualityNextRunAt(input.Schedule, now)
			if err != nil {
				return err
			}
			row := ModelQualityCase{
				Name:            input.Name,
				Description:     input.Description,
				Enabled:         input.Enabled,
				Archived:        false,
				Version:         1,
				EditVersion:     1,
				ScheduleJSON:    scheduleJSON,
				ScheduleVersion: 1,
				NextRunAt:       nextRunAt,
				CreatedBy:       actor,
				UpdatedBy:       actor,
				CreatedAt:       nowMs,
				UpdatedAt:       nowMs,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			revision := ModelQualityRevision{
				CaseID:      row.ID,
				Version:     1,
				ConfigJSON:  configJSON,
				Fingerprint: fingerprint,
				CreatedBy:   actor,
				CreatedAt:   nowMs,
			}
			if err := tx.Create(&revision).Error; err != nil {
				return err
			}
			savedID = row.ID
			return nil
		}

		var existing ModelQualityCase
		if err := lockForUpdate(tx).First(&existing, id).Error; err != nil {
			return err
		}
		if existing.EditVersion != input.ExpectedEditVersion {
			return ErrQualityConflict
		}
		if input.Enabled && existing.Archived {
			return errors.New("archived case cannot be enabled")
		}

		var prevRevision ModelQualityRevision
		if err := tx.Where("case_id = ? AND version = ?", id, existing.Version).First(&prevRevision).Error; err != nil {
			return err
		}
		version := existing.Version
		if prevRevision.Fingerprint != fingerprint {
			version = existing.Version + 1
			revision := ModelQualityRevision{
				CaseID:      id,
				Version:     version,
				ConfigJSON:  configJSON,
				Fingerprint: fingerprint,
				CreatedBy:   actor,
				CreatedAt:   nowMs,
			}
			if err := tx.Create(&revision).Error; err != nil {
				return err
			}
		}

		scheduleVersion := existing.ScheduleVersion
		nextRunAt := existing.NextRunAt
		if existing.ScheduleJSON != scheduleJSON || (!existing.Enabled && input.Enabled) {
			scheduleChanged = true
			scheduleVersion = existing.ScheduleVersion + 1
			nextRunAt, err = qualityNextRunAt(input.Schedule, now)
			if err != nil {
				return err
			}
		}
		enabledChanged = existing.Enabled != input.Enabled

		res := tx.Model(&ModelQualityCase{}).
			Where("id = ? AND edit_version = ?", id, input.ExpectedEditVersion).
			Updates(map[string]any{
				"name":             input.Name,
				"description":      input.Description,
				"enabled":          input.Enabled,
				"version":          version,
				"edit_version":     input.ExpectedEditVersion + 1,
				"schedule_json":    scheduleJSON,
				"schedule_version": scheduleVersion,
				"next_run_at":      nextRunAt,
				"updated_by":       actor,
				"updated_at":       nowMs,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrQualityConflict
		}

		// Disabling stops queueing new work but lets in-flight samples settle.
		if existing.Enabled && !input.Enabled {
			if err := cancelQualityPendingTx(tx, id, now); err != nil {
				return err
			}
		}
		savedID = id
		return nil
	})
	if err != nil {
		return nil, err
	}
	if id == 0 {
		auditQualityCase(actor, fmt.Sprintf("quality case %d created: enabled=%t", savedID, input.Enabled))
	} else if enabledChanged || scheduleChanged {
		auditQualityCase(actor, fmt.Sprintf("quality case %d saved: enabled=%t schedule_changed=%t", savedID, input.Enabled, scheduleChanged))
	}
	return GetQualityCase(savedID)
}

// cancelQualityPendingTx cancels pending samples of a case inside tx and
// releases their reserved daily budget. Requesting samples settle normally.
func cancelQualityPendingTx(tx *gorm.DB, caseID int64, now time.Time) error {
	if err := tx.Model(&ModelQualityRun{}).Where("case_id = ? AND status IN ?", caseID, []string{"queued", "running"}).Update("cancel_requested", true).Error; err != nil {
		return err
	}
	var pending []ModelQualitySample
	if err := tx.Where("case_id = ? AND status = ?", caseID, qualitySampleStatusPending).
		Select("id", "budget_day", "run_id").Find(&pending).Error; err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	if err := tx.Model(&ModelQualitySample{}).
		Where("case_id = ? AND status = ?", caseID, qualitySampleStatusPending).
		Updates(map[string]any{"status": qualitySampleStatusCancelled, "finished_at": now.UnixMilli()}).Error; err != nil {
		return err
	}
	byDay := map[string]int{}
	for _, sample := range pending {
		if sample.BudgetDay != "" {
			byDay[sample.BudgetDay]++
		}
	}
	for day, count := range byDay {
		if err := releaseQualityBudgetTx(tx, caseID, day, count); err != nil {
			return err
		}
	}
	seenRuns := map[string]bool{}
	for _, sample := range pending {
		if seenRuns[sample.RunID] {
			continue
		}
		seenRuns[sample.RunID] = true
		if err := closeQualityRunIfDoneTx(tx, sample.RunID, now); err != nil {
			return err
		}
	}
	return nil
}

func SetQualityCaseState(id int64, enabled, archived bool, expectedEditVersion int64, actor int, now time.Time) error {
	if enabled && archived {
		return errors.New("archived case cannot be enabled")
	}
	var prev ModelQualityCase
	err := qualityTransaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).First(&prev, id).Error; err != nil {
			return err
		}
		if prev.EditVersion != expectedEditVersion {
			return ErrQualityConflict
		}
		if archived && prev.ActiveRunID != "" {
			return ErrQualityConflict
		}
		if enabled && !prev.Enabled {
			var revision ModelQualityRevision
			if err := tx.Where("case_id = ? AND version = ?", id, prev.Version).First(&revision).Error; err != nil {
				return err
			}
			var cfg QualityConfig
			if err := common.UnmarshalJsonStr(revision.ConfigJSON, &cfg); err != nil {
				return err
			}
			if err := qualityEnabledAllowed(tx, cfg); err != nil {
				return err
			}
		}
		nextRunAt := prev.NextRunAt
		if enabled && !prev.Enabled {
			var schedule QualitySchedule
			if err := common.UnmarshalJsonStr(prev.ScheduleJSON, &schedule); err != nil {
				return err
			}
			var err error
			nextRunAt, err = qualityNextRunAt(schedule, now)
			if err != nil {
				return err
			}
		}
		res := tx.Model(&ModelQualityCase{}).
			Where("id = ? AND edit_version = ?", id, expectedEditVersion).
			Updates(map[string]any{
				"enabled":      enabled,
				"archived":     archived,
				"next_run_at":  nextRunAt,
				"edit_version": expectedEditVersion + 1,
				"updated_by":   actor,
				"updated_at":   now.UnixMilli(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrQualityConflict
		}
		if prev.Enabled && !enabled {
			if err := cancelQualityPendingTx(tx, id, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if prev.Enabled != enabled || prev.Archived != archived {
		auditQualityCase(actor, fmt.Sprintf("quality case %d state: enabled=%t archived=%t", id, enabled, archived))
	}
	return nil
}

func DeleteQualityCase(id int64, expectedEditVersion int64) error {
	return qualityTransaction(func(tx *gorm.DB) error {
		var kase ModelQualityCase
		if err := lockForUpdate(tx).First(&kase, id).Error; err != nil {
			return err
		}
		if kase.ActiveRunID != "" {
			return ErrQualityConflict
		}
		if expectedEditVersion != 0 && kase.EditVersion != expectedEditVersion {
			return ErrQualityConflict
		}
		return tx.Delete(&kase).Error
	})
}

// ---------------------------------------------------------------------------
// Budget (UTC-day scoped, dual global+case rows)
// ---------------------------------------------------------------------------

// reserveQualityBudgetTx creates-or-increments reservations on both scopes in
// the same transaction; limit checks are atomic conditional increments.
func reserveQualityBudgetTx(tx *gorm.DB, caseID int64, day string, count int, globalLimit, caseLimit int) error {
	scopes := []struct {
		id    string
		limit int
	}{
		{qualityGlobalBudgetID(day), globalLimit},
		{qualityCaseBudgetID(caseID, day), caseLimit},
	}
	for _, scope := range scopes {
		row := ModelQualityBudget{ID: scope.id, Day: day}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		res := tx.Model(&ModelQualityBudget{}).
			Where("id = ? AND reserved + started + ? <= ?", scope.id, count, scope.limit).
			Update("reserved", gorm.Expr("reserved + ?", count))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrQualityBudget
		}
	}
	return nil
}

func releaseQualityBudgetTx(tx *gorm.DB, caseID int64, day string, count int) error {
	for _, id := range []string{qualityGlobalBudgetID(day), qualityCaseBudgetID(caseID, day)} {
		res := tx.Model(&ModelQualityBudget{}).
			Where("id = ? AND reserved >= ?", id, count).
			Update("reserved", gorm.Expr("reserved - ?", count))
		if res.Error != nil {
			return res.Error
		}
		// A missing row or smaller reservation means the budget was already
		// released; releasing twice would corrupt the counter so tolerate 0.
	}
	return nil
}

// consumeQualityBudgetTx moves one reservation to started on the UTC day the
// sample actually starts. A stale-day reservation is released on its original
// day and re-acquired on the current day under the same dual checks.
func consumeQualityBudgetTx(tx *gorm.DB, caseID int64, sampleDay string, now time.Time, globalLimit, caseLimit int) (string, error) {
	today := qualityBudgetDay(now)
	if sampleDay != "" && sampleDay != today {
		if err := releaseQualityBudgetTx(tx, caseID, sampleDay, 1); err != nil {
			return "", err
		}
		scopes := []struct {
			id    string
			limit int
		}{
			{qualityGlobalBudgetID(today), globalLimit},
			{qualityCaseBudgetID(caseID, today), caseLimit},
		}
		for _, scope := range scopes {
			row := ModelQualityBudget{ID: scope.id, Day: today}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return "", err
			}
			res := tx.Model(&ModelQualityBudget{}).
				Where("id = ? AND reserved + started + 1 <= ?", scope.id, scope.limit).
				Update("started", gorm.Expr("started + 1"))
			if res.Error != nil {
				return "", res.Error
			}
			if res.RowsAffected != 1 {
				return "", ErrQualityBudget
			}
		}
		return today, nil
	}
	for _, scope := range []struct {
		id    string
		limit int
	}{{qualityGlobalBudgetID(today), globalLimit}, {qualityCaseBudgetID(caseID, today), caseLimit}} {
		res := tx.Model(&ModelQualityBudget{}).
			Where("id = ? AND reserved >= 1 AND started < ?", scope.id, scope.limit).
			Updates(map[string]any{
				"reserved": gorm.Expr("reserved - 1"),
				"started":  gorm.Expr("started + 1"),
			})
		if res.Error != nil {
			return "", res.Error
		}
		if res.RowsAffected != 1 {
			return "", ErrQualityBudget
		}
	}
	return today, nil
}

// ---------------------------------------------------------------------------
// Runs
// ---------------------------------------------------------------------------

func qualityIdempotencyKey(actor int, caseID int64, key string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s", actor, caseID, key)))
	return hex.EncodeToString(sum[:])
}

func qualityRequestHash(version int, channelIDs []int, samplesPerTarget int) (string, error) {
	ids := append([]int{}, channelIDs...)
	sort.Ints(ids)
	body, err := common.Marshal(map[string]any{
		"version":            version,
		"channel_ids":        ids,
		"samples_per_target": samplesPerTarget,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func createQualityRunTx(tx *gorm.DB, caseID int64, req QualityRunRequest, actor int, key, source string, slot int64, now time.Time) (*ModelQualityRun, bool, error) {
	if len(key) == 0 || len(key) > 128 || req.Version < 0 || req.SamplesPerTarget < 0 || req.SamplesPerTarget > 10 {
		return nil, false, errors.New("invalid run request")
	}
	seen := make(map[int]bool, len(req.ChannelIDs))
	for _, id := range req.ChannelIDs {
		if id <= 0 || seen[id] {
			return nil, false, errors.New("invalid channel subset")
		}
		seen[id] = true
	}
	requestHash, err := qualityRequestHash(req.Version, req.ChannelIDs, req.SamplesPerTarget)
	if err != nil {
		return nil, false, err
	}
	idemKey := qualityIdempotencyKey(actor, caseID, key)
	// Idempotent replay: same key returns the stored run; different payload is
	// a conflict.
	var existing ModelQualityRun
	existingErr := tx.Where("idempotency_key = ?", idemKey).First(&existing).Error
	if existingErr == nil {
		if existing.RequestHash != requestHash {
			return nil, false, ErrQualityConflict
		}
		return &existing, false, nil
	}
	if !errors.Is(existingErr, gorm.ErrRecordNotFound) {
		return nil, false, existingErr
	}
	nowMs := now.UnixMilli()

	// Consistent lock order: settings row, then case row.
	var settingsRow ModelQualitySettings
	settingsErr := lockForUpdate(tx).First(&settingsRow, qualitySettingsRowID).Error
	if settingsErr != nil && !errors.Is(settingsErr, gorm.ErrRecordNotFound) {
		return nil, false, settingsErr
	}
	settings := DefaultQualitySettings()
	if settingsErr == nil {
		if err := common.UnmarshalJsonStr(settingsRow.Config, &settings); err != nil {
			return nil, false, err
		}
	}

	var kase ModelQualityCase
	if err := lockForUpdate(tx).First(&kase, caseID).Error; err != nil {
		return nil, false, err
	}
	if !kase.Enabled || kase.Archived {
		return nil, false, ErrQualityDisabled
	}
	if !settings.Enabled {
		return nil, false, ErrQualityDisabled
	}

	var revision ModelQualityRevision
	if err := tx.Where("case_id = ? AND version = ?", caseID, kase.Version).First(&revision).Error; err != nil {
		return nil, false, err
	}
	var cfg QualityConfig
	if err := common.UnmarshalJsonStr(revision.ConfigJSON, &cfg); err != nil {
		return nil, false, err
	}
	if !slices.Contains(settings.TokenIDs, cfg.TokenID) {
		return nil, false, ErrQualityDisabled
	}
	if req.Version != 0 && req.Version != kase.Version {
		return nil, false, ErrQualityConflict
	}
	if cfg.Mode == "route" && len(req.ChannelIDs) > 0 {
		return nil, false, errors.New("normal routing cannot pin channels")
	}
	targets := cfg.ChannelIDs
	if len(req.ChannelIDs) > 0 {
		for _, id := range req.ChannelIDs {
			if !slices.Contains(cfg.ChannelIDs, id) {
				return nil, false, errors.New("channel subset is outside the case targets")
			}
		}
		targets = slices.Clone(req.ChannelIDs)
	}
	samplesPerTarget := cfg.SamplesPerTarget
	if req.SamplesPerTarget > 0 {
		if req.SamplesPerTarget > 10 {
			return nil, false, errors.New("samples per target out of range")
		}
		samplesPerTarget = req.SamplesPerTarget
	}
	total := samplesPerTarget * len(targets)
	if cfg.Mode == "route" {
		total = samplesPerTarget
	}
	if total < 1 || total > 100 {
		return nil, false, errors.New("sample count out of range")
	}
	cfg.ChannelIDs = targets
	cfg.SamplesPerTarget = samplesPerTarget
	runConfigJSON, err := common.Marshal(cfg)
	if err != nil {
		return nil, false, err
	}

	runID, err := common.GenerateRandomCharsKey(32)
	if err != nil {
		return nil, false, err
	}
	runID = "mq-" + runID

	// Per-case single active run, guarded by a CAS on active_run_id.
	res := tx.Model(&ModelQualityCase{}).
		Where("id = ? AND active_run_id = ?", caseID, "").
		Update("active_run_id", runID)
	if res.Error != nil {
		return nil, false, res.Error
	}
	if res.RowsAffected != 1 {
		return nil, false, ErrQualityConflict
	}

	day := qualityBudgetDay(now)
	if err := reserveQualityBudgetTx(tx, caseID, day, total, settings.DailyLimit, cfg.DailyLimit); err != nil {
		return nil, false, err
	}

	run := ModelQualityRun{
		ID:             runID,
		CaseID:         caseID,
		Version:        kase.Version,
		Source:         source,
		IdempotencyKey: idemKey,
		RequestHash:    requestHash,
		ConfigJSON:     string(runConfigJSON),
		Status:         "queued",
		CreatedBy:      actor,
		CreatedAt:      nowMs,
		SampleCount:    total,
	}

	// Interleave targets so early samples spread across channels.
	sampleRows := make([]ModelQualitySample, 0, total)
	channelNames := map[int]string{}
	for _, target := range targets {
		if target > 0 {
			name := ""
			var channel Channel
			if err := tx.Select("id", "name").First(&channel, target).Error; err == nil {
				name = channel.Name
			}
			channelNames[target] = name
		}
	}
	for ordinal := 0; ordinal < total; ordinal++ {
		target := 0
		if len(targets) > 0 {
			target = targets[ordinal%len(targets)]
		}
		sampleRows = append(sampleRows, ModelQualitySample{
			RunID:           runID,
			Ordinal:         ordinal,
			CaseID:          caseID,
			Version:         kase.Version,
			TargetChannelID: target,
			ChannelID:       target,
			ChannelName:     channelNames[target],
			Model:           cfg.Model,
			OutputType:      cfg.OutputType,
			Source:          source,
			Status:          qualitySampleStatusPending,
			CreatedAt:       nowMs,
			BudgetDay:       day,
		})
	}
	if err := tx.Create(&run).Error; err != nil {
		return nil, false, err
	}
	if err := tx.Create(&sampleRows).Error; err != nil {
		return nil, false, err
	}
	_ = slot
	return &run, true, nil
}

func CreateQualityRun(caseID int64, req QualityRunRequest, actor int, key, source string, slot int64, now time.Time) (*ModelQualityRun, bool, error) {
	var run *ModelQualityRun
	var created bool
	err := qualityTransaction(func(tx *gorm.DB) error {
		var err error
		run, created, err = createQualityRunTx(tx, caseID, req, actor, key, source, slot, now)
		return err
	})
	return run, created, err
}

type QualityRunFilter struct {
	CaseID int64
	Source string
	Status string
	Start  int64
	End    int64
}

func qualityRunFilterQuery(filter QualityRunFilter) *gorm.DB {
	query := DB.Model(&ModelQualityRun{})
	if filter.CaseID > 0 {
		query = query.Where("case_id = ?", filter.CaseID)
	}
	if filter.Source != "" {
		query = query.Where("source = ?", filter.Source)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Start > 0 {
		query = query.Where("created_at >= ?", filter.Start)
	}
	if filter.End > 0 {
		query = query.Where("created_at <= ?", filter.End)
	}
	return query
}

func ListQualityRuns(filter QualityRunFilter, offset, limit int) ([]ModelQualityRun, int64, error) {
	var total int64
	if err := qualityRunFilterQuery(filter).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	var runs []ModelQualityRun
	err := qualityRunFilterQuery(filter).
		Order("created_at DESC, id DESC").
		Offset(max(offset, 0)).
		Limit(min(limit, 100)).
		Find(&runs).Error
	return runs, total, err
}

func GetQualityRun(id string) (*ModelQualityRun, []ModelQualitySample, error) {
	var run ModelQualityRun
	if err := DB.Where("id = ?", id).First(&run).Error; err != nil {
		return nil, nil, err
	}
	var samples []ModelQualitySample
	if err := DB.Where("run_id = ?", id).Order("ordinal ASC").Find(&samples).Error; err != nil {
		return nil, nil, err
	}
	return &run, samples, nil
}

func CancelQualityRun(id string, now time.Time) error {
	return qualityTransaction(func(tx *gorm.DB) error {
		var run ModelQualityRun
		if err := lockForUpdate(tx).Where("id = ?", id).First(&run).Error; err != nil {
			return err
		}
		if run.Status != "queued" && run.Status != "running" {
			return nil
		}
		if err := tx.Model(&ModelQualityRun{}).
			Where("id = ?", id).
			Update("cancel_requested", true).Error; err != nil {
			return err
		}
		// Cancel pending samples and release their reservations.
		if err := cancelQualityPendingTx(tx, run.CaseID, now); err != nil {
			return err
		}
		// Scoped to this run only: re-read pending count for the run.
		var inFlight int64
		if err := tx.Model(&ModelQualitySample{}).
			Where("run_id = ? AND status IN ?", id, []string{qualitySampleStatusPending, qualitySampleStatusRequesting}).
			Count(&inFlight).Error; err != nil {
			return err
		}
		if inFlight == 0 {
			res := tx.Model(&ModelQualityRun{}).
				Where("id = ? AND status IN ?", id, []string{"queued", "running"}).
				Updates(map[string]any{"status": "cancelled", "finished_at": now.UnixMilli()})
			if res.Error != nil {
				return res.Error
			}
			return releaseQualityActiveRunTx(tx, run.CaseID, id)
		}
		return nil
	})
}

func releaseQualityActiveRunTx(tx *gorm.DB, caseID int64, runID string) error {
	res := tx.Model(&ModelQualityCase{}).
		Where("id = ? AND active_run_id = ?", caseID, runID).
		Update("active_run_id", "")
	if res.Error != nil {
		return res.Error
	}
	return nil
}

// ---------------------------------------------------------------------------
// Worker seam
// ---------------------------------------------------------------------------

func HasPendingQualityWork() bool {
	var count int64
	err := DB.Model(&ModelQualitySample{}).
		Where("status IN ?", []string{qualitySampleStatusPending, qualitySampleStatusRequesting}).
		Limit(1).Count(&count).Error
	return err == nil && count > 0
}

// ClaimQualitySample moves the oldest eligible pending sample to requesting
// under executorID, consumes its budget reservation, and returns the run's
// effective config. excludedChannels skips directed samples whose target is
// already in flight for this executor.
func ClaimQualitySample(executorID string, excludedChannels []int, now time.Time) (*ModelQualitySample, QualityConfig, error) {
	var claimed *ModelQualitySample
	var outCfg QualityConfig
	if executorID == "" {
		return nil, outCfg, ErrQualityOwnership
	}
	err := qualityTransaction(func(tx *gorm.DB) error {
		if err := qualityExecutorOwned(tx, executorID, now); err != nil {
			return err
		}
		settings, _, err := getQualitySettingsTx(tx)
		if err != nil {
			return err
		}
		if !settings.Enabled {
			return ErrQualityDisabled
		}

		// FIFO across runs: oldest active run's earliest pending sample first.
		var candidates []ModelQualitySample
		query := tx.Table("model_quality_samples AS s").Select("s.*").
			Joins("JOIN model_quality_runs r ON r.id = s.run_id").
			Joins("JOIN model_quality_cases c ON c.id = s.case_id").
			Where("s.status = ?", qualitySampleStatusPending).
			Where("r.cancel_requested = ?", false).
			Where("r.status IN ?", []string{"queued", "running"}).
			Where("c.enabled = ? AND c.archived = ? AND c.active_run_id = r.id", true, false)
		if len(excludedChannels) > 0 {
			query = query.Where("s.target_channel_id = 0 OR s.target_channel_id NOT IN ?", excludedChannels)
		}
		if err := query.Order("(SELECT COUNT(*) FROM model_quality_samples served WHERE served.run_id = r.id AND served.status <> 'pending') ASC, r.created_at ASC, r.id ASC, s.ordinal ASC").Limit(100).Find(&candidates).Error; err != nil {
			return err
		}
		for _, candidate := range candidates {
			var run ModelQualityRun
			if err := tx.Where("id = ?", candidate.RunID).First(&run).Error; err != nil {
				return err
			}
			if err := common.UnmarshalJsonStr(run.ConfigJSON, &outCfg); err != nil {
				return err
			}
			skipReason := ""
			day := candidate.BudgetDay
			if !slices.Contains(settings.TokenIDs, outCfg.TokenID) {
				// Token approval revoked between run creation and claim: skip the
				// sample without calling upstream, then keep scanning.
				skipReason = "not_approved"
			} else {
				if err := tx.SavePoint("quality_consume").Error; err != nil {
					return err
				}
				day, err = consumeQualityBudgetTx(tx, candidate.CaseID, candidate.BudgetDay, now, settings.DailyLimit, outCfg.DailyLimit)
				if errors.Is(err, ErrQualityBudget) {
					if err := tx.RollbackTo("quality_consume").Error; err != nil {
						return err
					}
					skipReason = "budget_exhausted"
				} else if err != nil {
					return err
				}
			}
			if skipReason != "" {
				if err := tx.Model(&ModelQualitySample{}).Where("id = ? AND status = ?", candidate.ID, qualitySampleStatusPending).
					Updates(map[string]any{"status": qualitySampleStatusSkipped, "error_code": skipReason, "finished_at": now.UnixMilli()}).Error; err != nil {
					return err
				}
				if err := releaseQualityBudgetTx(tx, candidate.CaseID, candidate.BudgetDay, 1); err != nil {
					return err
				}
				if err := closeQualityRunIfDoneTx(tx, candidate.RunID, now); err != nil {
					return err
				}
				continue
			}
			res := tx.Model(&ModelQualitySample{}).Where("id = ? AND status = ?", candidate.ID, qualitySampleStatusPending).
				Updates(map[string]any{"status": qualitySampleStatusRequesting, "executor_id": executorID, "started_at": now.UnixMilli(), "budget_day": day})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return ErrQualityOwnership
			}
			if err := tx.Model(&ModelQualityRun{}).Where("id = ?", run.ID).
				Updates(map[string]any{"status": "running", "executor_id": executorID, "started_at": gorm.Expr("CASE WHEN started_at = 0 THEN ? ELSE started_at END", now.UnixMilli())}).Error; err != nil {
				return err
			}
			candidate.Status, candidate.ExecutorID = qualitySampleStatusRequesting, executorID
			candidate.StartedAt, candidate.BudgetDay = now.UnixMilli(), day
			claimed = &candidate
			return nil
		}
		return nil
	})
	if err != nil {
		return nil, outCfg, err
	}
	if claimed == nil {
		return nil, outCfg, gorm.ErrRecordNotFound
	}
	return claimed, outCfg, nil
}

func qualityExecutorOwned(tx *gorm.DB, executorID string, now time.Time) error {
	var count int64
	err := tx.Table("system_task_locks AS l").
		Joins("JOIN system_tasks t ON t.task_id = l.task_id AND t.locked_by = l.locked_by").
		Where("l.type = ? AND l.task_id = ? AND l.locked_until >= ? AND t.status = ?", ModelQualityExecuteTask, executorID, now.Unix(), SystemTaskStatusRunning).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrQualityOwnership
	}
	return nil
}

func QualitySampleCanContinue(id int64, executorID string, now time.Time) (bool, error) {
	if err := qualityExecutorOwned(DB, executorID, now); err != nil {
		return false, err
	}
	var count int64
	err := DB.Table("model_quality_samples AS s").
		Joins("JOIN model_quality_runs r ON r.id = s.run_id").
		Joins("JOIN model_quality_cases c ON c.id = s.case_id").
		Where("s.id = ? AND s.executor_id = ? AND s.status = ? AND r.cancel_requested = ? AND c.enabled = ? AND c.archived = ? AND c.active_run_id = r.id", id, executorID, qualitySampleStatusRequesting, false, true, false).
		Count(&count).Error
	return count == 1, err
}

func getQualitySettingsTx(tx *gorm.DB) (QualitySettingsConfig, int64, error) {
	var row ModelQualitySettings
	err := tx.First(&row, qualitySettingsRowID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DefaultQualitySettings(), 0, nil
	}
	if err != nil {
		return QualitySettingsConfig{}, 0, err
	}
	cfg := DefaultQualitySettings()
	if err := common.UnmarshalJsonStr(row.Config, &cfg); err != nil {
		return QualitySettingsConfig{}, 0, err
	}
	return cfg, row.Version, nil
}

// FinishQualitySample persists an executor result under ownership conditions,
// stores the bounded artifact once, and closes the run when nothing remains.
func FinishQualitySample(id int64, executorID string, result QualityResult, now time.Time) error {
	if len(result.Text) > QualityMaxResponseBytes || len(result.SVG) > QualityMaxSVGBytes {
		result.Status, result.ErrorCode, result.Validation = qualitySampleStatusFailed, "artifact_too_large", "artifact_too_large"
		result.Text, result.SVG = nil, nil
	}
	terminal := []string{
		qualitySampleStatusSucceeded,
		qualitySampleStatusFailed,
		qualitySampleStatusCancelled,
		qualitySampleStatusInterrupted,
		qualitySampleStatusSkipped,
	}
	if !slices.Contains(terminal, result.Status) {
		return errors.New("invalid result status")
	}
	return qualityTransaction(func(tx *gorm.DB) error {
		if err := qualityExecutorOwned(tx, executorID, now); err != nil {
			return err
		}
		var sample ModelQualitySample
		if err := tx.First(&sample, id).Error; err != nil {
			return err
		}
		if sample.TargetChannelID > 0 && result.ChannelID == 0 {
			result.ChannelID, result.ChannelName = sample.ChannelID, sample.ChannelName
		}
		res := tx.Model(&ModelQualitySample{}).
			Where("id = ? AND status = ? AND executor_id = ?", id, qualitySampleStatusRequesting, executorID).
			Updates(map[string]any{
				"status":          result.Status,
				"request_success": result.RequestSuccess,
				"request_id":      result.RequestID,
				"channel_id":      result.ChannelID,
				"channel_name":    result.ChannelName,
				"response_model":  result.ResponseModel,
				"duration_ms":     result.DurationMs,
				"first_text_ms":   result.FirstTextMs,
				"input_tokens":    result.InputTokens,
				"output_tokens":   result.OutputTokens,
				"error_code":      result.ErrorCode,
				"validation":      result.Validation,
				"finish_reason":   result.FinishReason,
				"finished_at":     now.UnixMilli(),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrQualityOwnership
		}
		if result.ChannelID == 0 {
			// Route samples keep their attribution snapshot columns untouched.
		}
		if result.Status == qualitySampleStatusSkipped {
			for _, budgetID := range []string{qualityGlobalBudgetID(sample.BudgetDay), qualityCaseBudgetID(sample.CaseID, sample.BudgetDay)} {
				if err := tx.Model(&ModelQualityBudget{}).Where("id = ? AND started > 0", budgetID).Update("started", gorm.Expr("started - 1")).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&ModelQualitySample{}).Where("id = ?", id).Update("started_at", 0).Error; err != nil {
				return err
			}
		}

		if len(result.Text) > 0 || len(result.SVG) > 0 {
			if err := storeQualityArtifactTx(tx, sample.ID, result, now); err != nil {
				return err
			}
		}

		return closeQualityRunIfDoneTx(tx, sample.RunID, now)
	})
}

func storeQualityArtifactTx(tx *gorm.DB, sampleID int64, result QualityResult, now time.Time) error {
	text := result.Text
	svg := result.SVG
	sum := sha256.Sum256(text)
	settings, _, err := getQualitySettingsTx(tx)
	if err != nil {
		return err
	}
	days := settings.ArtifactDays
	if days < 1 {
		days = 30
	}
	artifact := ModelQualityArtifact{
		SampleID:          sampleID,
		Text:              text,
		SVG:               svg,
		SHA256:            hex.EncodeToString(sum[:]),
		ValidationVersion: qualityArtifactValidation,
		ExpiresAt:         now.Add(time.Duration(days) * 24 * time.Hour).UnixMilli(),
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&artifact).Error
}

// closeQualityRunIfDoneTx terminalizes a run when no pending/requesting
// samples remain and releases the case's active-run guard.
func closeQualityRunIfDoneTx(tx *gorm.DB, runID string, now time.Time) error {
	var remaining int64
	if err := tx.Model(&ModelQualitySample{}).
		Where("run_id = ? AND status IN ?", runID, []string{qualitySampleStatusPending, qualitySampleStatusRequesting}).
		Count(&remaining).Error; err != nil {
		return err
	}
	if remaining > 0 {
		return nil
	}
	var run ModelQualityRun
	if err := tx.Where("id = ?", runID).First(&run).Error; err != nil {
		return err
	}
	status := "completed"
	if run.CancelRequested {
		status = "cancelled"
	}
	res := tx.Model(&ModelQualityRun{}).
		Where("id = ? AND status IN ?", runID, []string{"queued", "running"}).
		Updates(map[string]any{"status": status, "finished_at": now.UnixMilli()})
	if res.Error != nil {
		return res.Error
	}
	return releaseQualityActiveRunTx(tx, run.CaseID, runID)
}

// RecoverQualityWork requeues samples whose executor died (terminal task or
// expired lock) instead of failing them, so deploys and restarts do not lose
// samples. Pending samples are left eligible; in-flight runs are reconciled to
// terminal states.
func RecoverQualityWork(executorID string, now time.Time) error {
	var executors []string
	if err := DB.Model(&ModelQualitySample{}).
		Where("status = ? AND executor_id <> '' AND executor_id <> ?", qualitySampleStatusRequesting, executorID).
		Distinct().Pluck("executor_id", &executors).Error; err != nil {
		return err
	}
	for _, owner := range executors {
		var task SystemTask
		err := DB.Where("task_id = ?", owner).First(&task).Error
		orphan := errors.Is(err, gorm.ErrRecordNotFound)
		if err == nil {
			if task.Status == SystemTaskStatusSucceeded || task.Status == SystemTaskStatusFailed {
				orphan = true
			} else {
				var lock SystemTaskLock
				lockErr := DB.Where("task_id = ?", owner).First(&lock).Error
				if lockErr != nil && !errors.Is(lockErr, gorm.ErrRecordNotFound) {
					return lockErr
				}
				if errors.Is(lockErr, gorm.ErrRecordNotFound) || (lockErr == nil && lock.LockedUntil < now.Unix()) {
					orphan = true
				}
			}
		} else if !orphan {
			return err
		}
		if !orphan {
			continue
		}
		if err := qualityTransaction(func(tx *gorm.DB) error {
			var stuck []ModelQualitySample
			if err := tx.Where("executor_id = ? AND (status = ? OR (status = ? AND error_code IN ?))",
				owner, qualitySampleStatusRequesting, qualitySampleStatusInterrupted,
				[]string{"executor_interrupted", "interrupted_unknown"}).
				Select("id", "case_id", "budget_day").Find(&stuck).Error; err != nil {
				return err
			}
			if len(stuck) == 0 {
				return nil
			}
			ids := make([]int64, 0, len(stuck))
			type scope struct {
				caseID int64
				day    string
			}
			consumed := map[scope]int{}
			for _, sample := range stuck {
				ids = append(ids, sample.ID)
				if sample.BudgetDay != "" {
					consumed[scope{sample.CaseID, sample.BudgetDay}]++
				}
			}
			if err := tx.Where("sample_id IN ?", ids).Delete(&ModelQualityArtifact{}).Error; err != nil {
				return err
			}
			res := tx.Model(&ModelQualitySample{}).
				Where("id IN ? AND executor_id = ?", ids, owner).
				Updates(map[string]any{
					"status":          qualitySampleStatusPending,
					"executor_id":     "",
					"error_code":      "",
					"validation":      "",
					"finish_reason":   "",
					"request_id":      "",
					"request_success": false,
					"started_at":      0,
					"finished_at":     0,
					"duration_ms":     0,
					"first_text_ms":   0,
					"input_tokens":    0,
					"output_tokens":   0,
				})
			if res.Error != nil {
				return res.Error
			}
			// Route samples attribute the actual channel at finish; a partial
			// interrupted result may have stamped one already.
			if err := tx.Model(&ModelQualitySample{}).
				Where("id IN ? AND target_channel_id = 0", ids).
				Updates(map[string]any{"channel_id": 0, "channel_name": "", "response_model": ""}).Error; err != nil {
				return err
			}
			if err := tx.Model(&ModelQualitySample{}).
				Where("id IN ? AND target_channel_id <> 0", ids).
				Update("response_model", "").Error; err != nil {
				return err
			}
			for s, count := range consumed {
				if err := unconsumeQualityBudgetTx(tx, s.caseID, s.day, count); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return reconcileQualityRuns(now)
}

// unconsumeQualityBudgetTx moves consumed slots back to reserved when in-flight
// samples are requeued after their executor died. A missing row or a smaller
// started count means the slot was already released; tolerate 0 rows.
func unconsumeQualityBudgetTx(tx *gorm.DB, caseID int64, day string, count int) error {
	for _, id := range []string{qualityGlobalBudgetID(day), qualityCaseBudgetID(caseID, day)} {
		res := tx.Model(&ModelQualityBudget{}).
			Where("id = ? AND started >= ?", id, count).
			Updates(map[string]any{
				"started":  gorm.Expr("started - ?", count),
				"reserved": gorm.Expr("reserved + ?", count),
			})
		if res.Error != nil {
			return res.Error
		}
	}
	return nil
}

func reconcileQualityRuns(now time.Time) error {
	var runs []ModelQualityRun
	if err := DB.Where("status IN ?", []string{"queued", "running"}).Find(&runs).Error; err != nil {
		return err
	}
	for _, run := range runs {
		var remaining int64
		if err := DB.Model(&ModelQualitySample{}).
			Where("run_id = ? AND status IN ?", run.ID, []string{qualitySampleStatusPending, qualitySampleStatusRequesting}).
			Count(&remaining).Error; err != nil {
			return err
		}
		if remaining > 0 {
			continue
		}
		if err := qualityTransaction(func(tx *gorm.DB) error {
			return closeQualityRunIfDoneTx(tx, run.ID, now)
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Scheduling
// ---------------------------------------------------------------------------

// ScheduleQualityRuns starts a run for every due enabled case, advances
// next_run_at under an optimistic guard, and records skip reasons instead of
// replaying missed slots.
func ScheduleQualityRuns(now time.Time) error {
	settings, _, err := GetQualitySettings()
	if err != nil {
		return err
	}
	if !settings.Enabled {
		return nil
	}
	var due []ModelQualityCase
	if err := DB.Where("enabled = ? AND archived = ? AND next_run_at > 0 AND next_run_at <= ?",
		true, false, now.UnixMilli()).Find(&due).Error; err != nil {
		return err
	}
	for _, kase := range due {
		if err := scheduleQualityCase(kase, now); err != nil {
			return err
		}
	}
	return nil
}

func scheduleQualityCase(kase ModelQualityCase, now time.Time) error {
	var schedule QualitySchedule
	if err := common.UnmarshalJsonStr(kase.ScheduleJSON, &schedule); err != nil {
		return err
	}
	anchor := time.UnixMilli(kase.NextRunAt)
	next, nextErr := NextQualitySchedule(schedule, now, anchor)
	if nextErr != nil {
		return nextErr
	}
	nextMs := int64(0)
	if !next.IsZero() {
		nextMs = next.UnixMilli()
	}
	slot := kase.NextRunAt

	reason := ""
	err := qualityTransaction(func(tx *gorm.DB) error {
		// Lock the settings row first so this transaction keeps the same
		// settings -> case order as createQualityRunTx and cannot deadlock.
		var settingsRow ModelQualitySettings
		if err := lockForUpdate(tx).First(&settingsRow, qualitySettingsRowID).Error; err != nil &&
			!errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// Optimistic guard: another scheduler pass may have already advanced.
		res := tx.Model(&ModelQualityCase{}).
			Where("id = ? AND schedule_version = ? AND next_run_at = ?", kase.ID, kase.ScheduleVersion, kase.NextRunAt).
			Updates(map[string]any{"next_run_at": nextMs})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return gorm.ErrRecordNotFound // already advanced; treat as no-op
		}
		if !schedule.Enabled {
			reason = "disabled"
			return nil
		}
		key := fmt.Sprintf("%d:%d:%d", kase.ID, kase.ScheduleVersion, slot)
		req := QualityRunRequest{Version: kase.Version}
		if now.Sub(anchor) > time.Minute {
			return tx.Model(&ModelQualityCase{}).Where("id = ?", kase.ID).Update("last_schedule_reason", "missed").Error
		}
		if err := tx.SavePoint("quality_run").Error; err != nil {
			return err
		}
		_, _, runErr := createQualityRunTx(tx, kase.ID, req, 0, key, qualityRunSourceSchedule, slot, now)
		if runErr != nil {
			switch {
			case errors.Is(runErr, ErrQualityConflict):
				reason = "busy"
			case errors.Is(runErr, ErrQualityBudget):
				reason = "budget"
			case errors.Is(runErr, ErrQualityDisabled):
				reason = "disabled"
			default:
				return runErr
			}
			if err := tx.RollbackTo("quality_run").Error; err != nil {
				return err
			}
			return tx.Model(&ModelQualityCase{}).
				Where("id = ?", kase.ID).
				Update("last_schedule_reason", reason).Error
		}
		reason = ""
		return tx.Model(&ModelQualityCase{}).
			Where("id = ?", kase.ID).
			Update("last_schedule_reason", "").Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}

// ---------------------------------------------------------------------------
// Artifacts / annotation / retention
// ---------------------------------------------------------------------------

func GetQualityArtifact(sampleID int64) (*ModelQualityArtifact, error) {
	var artifact ModelQualityArtifact
	if err := DB.First(&artifact, sampleID).Error; err != nil {
		return nil, err
	}
	return &artifact, nil
}

func AnnotateQualitySample(id int64, tag, note string, pinned bool, actor int, now time.Time) error {
	if len(tag) > 32 || len(note) > 2000 {
		return errors.New("annotation exceeds size limit")
	}
	return qualityTransaction(func(tx *gorm.DB) error {
		var sample ModelQualitySample
		if err := lockForUpdate(tx).First(&sample, id).Error; err != nil {
			return err
		}
		if pinned && !sample.Pinned {
			var count int64
			if err := tx.Model(&ModelQualitySample{}).
				Where("case_id = ? AND pinned = ?", sample.CaseID, true).
				Count(&count).Error; err != nil {
				return err
			}
			if count >= qualityPinnedPerCaseLimit {
				return errors.New("pinned sample limit reached")
			}
		}
		res := tx.Model(&ModelQualitySample{}).Where("id = ?", id).Updates(map[string]any{
			"annotation":   tag,
			"note":         note,
			"pinned":       pinned,
			"annotated_by": actor,
			"annotated_at": now.UnixMilli(),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// CleanupQualityRetention removes expired artifacts and stale metadata in one
// bounded batch. Pinned samples keep their rows and artifacts; revisions are
// preserved while referenced by runs.
func CleanupQualityRetention(now time.Time) error {
	return qualityTransaction(func(tx *gorm.DB) error {
		settings, _, err := getQualitySettingsTx(tx)
		if err != nil {
			return err
		}
		if !settings.RetentionEnabled {
			return nil
		}
		var expired []int64
		if err := tx.Table("model_quality_artifacts AS a").
			Joins("JOIN model_quality_samples s ON s.id = a.sample_id").
			Where("a.expires_at > 0 AND a.expires_at < ? AND s.pinned = ?", now.UnixMilli(), false).
			Order("a.sample_id ASC").Limit(qualityRetentionBatch).Pluck("a.sample_id", &expired).Error; err != nil {
			return err
		}
		if len(expired) > 0 {
			if err := tx.Where("sample_id IN ?", expired).Delete(&ModelQualityArtifact{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&ModelQualitySample{}).Where("id IN ?", expired).Update("artifact_expired", true).Error; err != nil {
				return err
			}
		}
		cutoff := now.Add(-time.Duration(settings.MetadataDays) * 24 * time.Hour).UnixMilli()
		var staleSamples []int64
		if err := tx.Model(&ModelQualitySample{}).
			Where("created_at < ? AND pinned = ? AND status NOT IN ?", cutoff, false, []string{qualitySampleStatusPending, qualitySampleStatusRequesting}).
			Order("id ASC").Limit(qualityRetentionBatch).Pluck("id", &staleSamples).Error; err != nil {
			return err
		}
		if len(staleSamples) == 0 {
			return nil
		}
		if err := tx.Where("sample_id IN ?", staleSamples).Delete(&ModelQualityArtifact{}).Error; err != nil {
			return err
		}
		return tx.Where("id IN ?", staleSamples).Delete(&ModelQualitySample{}).Error
	})
}
