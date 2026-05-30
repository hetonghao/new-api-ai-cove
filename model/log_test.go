package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func seedUsageLogsForExcludeSelfTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	now := time.Now().Unix()

	require.NoError(t, LOG_DB.Create(&[]Log{
		{
			UserId:           1,
			Username:         "admin",
			CreatedAt:        now - 3,
			Type:             LogTypeConsume,
			ModelName:        "gpt-5.4",
			TokenName:        "admin-token",
			Group:            "default",
			Quota:            10,
			PromptTokens:     100,
			CompletionTokens: 200,
		},
		{
			UserId:           2,
			Username:         "alice",
			CreatedAt:        now - 2,
			Type:             LogTypeConsume,
			ModelName:        "gpt-5.4",
			TokenName:        "alice-token",
			Group:            "default",
			Quota:            20,
			PromptTokens:     300,
			CompletionTokens: 400,
		},
		{
			UserId:           3,
			Username:         "bob",
			CreatedAt:        now - 1,
			Type:             LogTypeConsume,
			ModelName:        "gpt-5.4",
			TokenName:        "bob-token",
			Group:            "default",
			Quota:            30,
			PromptTokens:     500,
			CompletionTokens: 600,
		},
	}).Error)
}

func TestGetAllLogsCanExcludeCurrentAdminUser(t *testing.T) {
	seedUsageLogsForExcludeSelfTest(t)

	logs, total, err := GetAllLogs(
		LogTypeConsume,
		0,
		0,
		"",
		"",
		"",
		0,
		20,
		0,
		"",
		"",
		"",
		1,
	)

	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, logs, 2)
	require.Equal(t, 3, logs[0].UserId)
	require.Equal(t, 2, logs[1].UserId)
}

func TestSumUsedQuotaCanExcludeCurrentAdminUser(t *testing.T) {
	seedUsageLogsForExcludeSelfTest(t)

	stat, err := SumUsedQuota(
		LogTypeConsume,
		0,
		0,
		"",
		"",
		"",
		0,
		"",
		1,
	)

	require.NoError(t, err)
	require.Equal(t, 50, stat.Quota)
	require.Equal(t, 2, stat.Rpm)
	require.Equal(t, 1800, stat.Tpm)
}
