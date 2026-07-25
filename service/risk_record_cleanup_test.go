package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupRiskRecordCleanupTest(t *testing.T) {
	t.Helper()
	truncate(t)
	require.NoError(t, model.DB.AutoMigrate(&model.RiskRecord{}, &model.RiskRecordGovernance{}))
	require.NoError(t, model.DB.Exec("DELETE FROM risk_records").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM risk_record_governance").Error)
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM risk_records")
		model.DB.Exec("DELETE FROM risk_record_governance")
	})
}

func TestRiskRecordCleanupHandler_isScheduledEveryTwentyFourHours(t *testing.T) {
	// Given
	handler := riskRecordCleanupHandler{}

	// When / Then
	assert.Equal(t, model.SystemTaskTypeRiskRecordCleanup, handler.Type())
	assert.True(t, handler.Enabled())
	assert.Equal(t, 24*time.Hour, handler.Interval())
	assert.Nil(t, handler.NewPayload())
}

func TestRiskRecordCleanupHandler_deletesAtMostConfiguredBatchesAndRecordsHistory(t *testing.T) {
	// Given
	setupRiskRecordCleanupTest(t)
	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	_, err := model.SaveRiskRecordGovernance(context.Background(), model.RiskRecordGovernanceInput{
		SaveScope: model.RiskRecordSaveAll, RetentionDays: 30,
	})
	require.NoError(t, err)
	for index := range 5 {
		require.NoError(t, model.DB.Create(&model.RiskRecord{
			RequestID: string(rune('a' + index)), ObservedAt: now.Add(-31 * 24 * time.Hour),
		}).Error)
	}
	require.NoError(t, model.DB.Create(&model.RiskRecord{
		RequestID: "fresh", ObservedAt: now.Add(-29 * 24 * time.Hour),
	}).Error)
	task, err := model.CreateSystemTask(model.SystemTaskTypeRiskRecordCleanup, nil, nil)
	require.NoError(t, err)
	const runnerID = "risk-cleanup-test"
	claimedTask, claimed, err := model.ClaimSystemTask(task.ID, task.Type, runnerID, common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, claimed)
	handler := riskRecordCleanupHandler{
		now: func() time.Time { return now }, batchSize: 2, maxBatches: 2,
	}

	// When
	handler.Run(context.Background(), claimedTask, runnerID)

	// Then
	var remaining int64
	require.NoError(t, model.DB.Model(&model.RiskRecord{}).Count(&remaining).Error)
	assert.EqualValues(t, 2, remaining)
	completed, err := model.GetSystemTaskByTaskID(task.TaskID)
	require.NoError(t, err)
	require.Equal(t, model.SystemTaskStatusSucceeded, completed.Status)
	var result RiskRecordCleanupResult
	require.NoError(t, json.Unmarshal([]byte(completed.Result), &result))
	assert.EqualValues(t, 4, result.DeletedCount)
}
