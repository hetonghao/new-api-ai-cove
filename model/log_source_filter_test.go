package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLogSourceFiltersMatchWebSocketAndTurboMarkers(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	require.NoError(t, LOG_DB.Create(&[]Log{
		{
			UserId: 1, CreatedAt: now - 3, Type: LogTypeConsume,
			ModelName: "gpt-5.4", Other: `{"transport":"websocket","client_source":"turbo"}`,
		},
		{
			UserId: 1, CreatedAt: now - 2, Type: LogTypeConsume,
			ModelName: "gpt-5.4", Other: `{"transport":"websocket"}`,
		},
		{
			UserId: 1, CreatedAt: now - 1, Type: LogTypeConsume,
			ModelName: "gpt-5.4", Other: `{"transport":"http"}`,
		},
	}).Error)

	logs, total, err := GetAllLogs(
		LogTypeConsume, 0, 0, "", "", "", 0, 20, 0, "", "", "", 0,
		LogSourceFilters{WebSocket: true},
	)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	require.Len(t, logs, 2)

	logs, total, err = GetAllLogs(
		LogTypeConsume, 0, 0, "", "", "", 0, 20, 0, "", "", "", 0,
		LogSourceFilters{FromTurbo: true},
	)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)

	logs, total, err = GetUserLogs(
		1, LogTypeConsume, 0, 0, "", "", 0, 20, "", "", "",
		LogSourceFilters{WebSocket: true, FromTurbo: true},
	)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)

	stat, err := SumUsedQuota(
		LogTypeConsume, 0, 0, "", "", "", 0, "", 0,
		LogSourceFilters{WebSocket: true, FromTurbo: true},
	)
	require.NoError(t, err)
	require.Equal(t, 1, stat.Rpm)
}

func TestSalesLogsFilterWebSocketAndTurboSources(t *testing.T) {
	db := setupSalesModelTestDB(t)
	seedSalesModelUsers(t, db)
	now := time.Now().Unix()
	require.NoError(t, db.Create(&[]Log{
		{
			UserId: 2, Username: "alice", Type: LogTypeConsume, CreatedAt: now - 2,
			ModelName: "gpt-ws", Quota: 20, PromptTokens: 30, CompletionTokens: 40,
			Other: `{"transport":"websocket","client_source":"turbo"}`,
		},
		{
			UserId: 3, Username: "bob", Type: LogTypeConsume, CreatedAt: now - 1,
			ModelName: "gpt-http", Quota: 50, PromptTokens: 60, CompletionTokens: 70,
			Other: `{"transport":"http"}`,
		},
	}).Error)
	filters := LogSourceFilters{WebSocket: true, FromTurbo: true}

	logs, total, err := GetSalesLogs(1, LogTypeConsume, 0, 0, "", "", "", 0, 20, 0, "", "", "", filters)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Len(t, logs, 1)
	require.Equal(t, "gpt-ws", logs[0].ModelName)

	stat, err := GetSalesLogsStat(1, LogTypeUnknown, 0, 0, "", "", "", 0, "", "", "", filters)
	require.NoError(t, err)
	require.Equal(t, 20, stat.Quota)
	require.Equal(t, 1, stat.Rpm)
	require.Equal(t, 70, stat.Tpm)
}
