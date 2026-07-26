package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/model"
)

const (
	riskRecordCleanupInterval   = 24 * time.Hour
	riskRecordCleanupBatchSize  = 500
	riskRecordCleanupMaxBatches = 100
)

type RiskRecordCleanupResult struct {
	DeletedCount int64 `json:"deleted_count"`
}

type riskRecordCleanupState struct {
	Total     int64 `json:"total"`
	Processed int64 `json:"processed"`
	Progress  int   `json:"progress"`
}

type riskRecordCleanupHandler struct {
	now        func() time.Time
	batchSize  int
	maxBatches int
}

func (riskRecordCleanupHandler) Type() string { return model.SystemTaskTypeRiskRecordCleanup }

func (riskRecordCleanupHandler) Enabled() bool { return true }

func (riskRecordCleanupHandler) Interval() time.Duration { return riskRecordCleanupInterval }

func (riskRecordCleanupHandler) NewPayload() any { return nil }

func (h riskRecordCleanupHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	governance, err := model.GetRiskRecordGovernance(ctx)
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	now := time.Now
	if h.now != nil {
		now = h.now
	}
	batchSize := h.batchSize
	if batchSize < 1 {
		batchSize = riskRecordCleanupBatchSize
	}
	maxBatches := h.maxBatches
	if maxBatches < 1 {
		maxBatches = riskRecordCleanupMaxBatches
	}
	cutoff := now().UTC().Add(-time.Duration(governance.RetentionDays) * 24 * time.Hour)
	expiredCount, err := model.CountExpiredRiskRecords(ctx, cutoff)
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	maxRows := int64(batchSize) * int64(maxBatches)
	if expiredCount > maxRows {
		expiredCount = maxRows
	}
	state := riskRecordCleanupState{Total: expiredCount}
	if expiredCount == 0 {
		state.Progress = 100
	}
	if err := model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
		logSystemTaskLockError(ctx, task, err)
		return
	}

	var deletedCount int64
	for range maxBatches {
		if state.Total == 0 {
			break
		}
		deleted, deleteErr := model.DeleteExpiredRiskRecordsBatch(ctx, cutoff, batchSize)
		if deleteErr != nil {
			failSystemTask(task, runnerID, deleteErr)
			return
		}
		deletedCount += deleted
		state.Processed = deletedCount
		state.Progress = int(deletedCount * 100 / state.Total)
		if state.Progress > 100 {
			state.Progress = 100
		}
		if err := model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
			logSystemTaskLockError(ctx, task, err)
			return
		}
		if deleted < int64(batchSize) {
			break
		}
	}
	if state.Progress < 100 {
		state.Progress = 100
		if err := model.UpdateSystemTaskState(task.TaskID, runnerID, state); err != nil {
			logSystemTaskLockError(ctx, task, err)
			return
		}
	}
	result := RiskRecordCleanupResult{DeletedCount: deletedCount}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, result, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func init() {
	RegisterSystemTaskHandler(riskRecordCleanupHandler{})
}
