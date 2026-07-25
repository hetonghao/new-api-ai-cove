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

	var deletedCount int64
	for range maxBatches {
		deleted, deleteErr := model.DeleteExpiredRiskRecordsBatch(ctx, cutoff, batchSize)
		if deleteErr != nil {
			failSystemTask(task, runnerID, deleteErr)
			return
		}
		deletedCount += deleted
		if deleted < int64(batchSize) {
			break
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
