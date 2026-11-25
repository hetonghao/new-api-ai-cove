package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestDeviMonRefreshOnlyRequestedAccountAndPreserveSchedule(t *testing.T) {
	truncate(t)
	// The full inventory contains another Codex account. The upstream fixture
	// rejects any quota probe not made with the selected account's credential.
	files := []any{deviMonTestAccount("one@example.test", "idx-one", "acct-one", ""), deviMonTestAccount("two@example.test", "idx-two", "acct-two", ""), map[string]any{"email": "orphan@example.test", "provider": "devin", "disabled": true, "weight": 4}}
	failURL := ""
	probes := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v0/management/auth-files" {
			raw, _ := common.Marshal(map[string]any{"files": files})
			_, _ = w.Write(raw)
			return
		}
		var call struct {
			Index string `json:"auth_index"`
			URL   string `json:"url"`
		}
		if err := common.DecodeJson(r.Body, &call); err != nil || call.Index != "idx-one" {
			t.Errorf("unexpected credential for request: %q", call.Index)
			w.WriteHeader(400)
			return
		}
		probes++
		if call.URL == failURL || (failURL == "renewal" && strings.HasPrefix(call.URL, deviMonSubscribeURL+"?")) {
			w.WriteHeader(502)
			return
		}
		body := "{}"
		if call.URL == deviMonUsageURL {
			body = `{"credits":{"balance":"90"},"rate_limit":{"primary_window":{"used_percent":30}}}`
		}
		raw, _ := common.Marshal(map[string]any{"status_code": 200, "body": body})
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(deviMonReplaceBase(t, srv.URL))
	stale := time.Now().Add(-time.Hour)
	task, err := model.CreateSystemTask(model.SystemTaskTypeDevinMonitor, nil, nil)
	require.NoError(t, err)
	_, ok, err := model.ClaimSystemTask(task.ID, task.Type, "full", time.Now().Unix()+60)
	require.NoError(t, err)
	require.True(t, ok)
	old := DeviMonSnapshot{CollectedAt: stale.Unix(), DurationMS: 3000, Accounts: []DeviMonAccount{{Email: "one@example.test", CreditBalance: "10"}, {Email: "two@example.test", CreditBalance: "20"}}, UnpairedDevin: []DeviMonDevin{{Email: "orphan@example.test"}}}
	require.NoError(t, model.FinishSystemTask(task.TaskID, "full", model.SystemTaskStatusSucceeded, old, ""))
	before, err := GetDeviMonState()
	require.NoError(t, err)
	require.NoError(t, RefreshDeviMonAccount(context.Background(), " ONE@example.test "))
	require.Equal(t, 3, probes)
	require.NoError(t, RefreshDeviMonAccount(context.Background(), "orphan@example.test"))
	require.Equal(t, 3, probes)
	require.ErrorIs(t, RefreshDeviMonAccount(context.Background(), "missing@example.test"), ErrDeviMonAccountMissing)
	require.Equal(t, 3, probes)
	failURL = "renewal"
	require.Error(t, RefreshDeviMonAccount(context.Background(), "one@example.test"))
	failURL = deviMonUsageURL
	require.Error(t, RefreshDeviMonAccount(context.Background(), "one@example.test"))
	after, err := GetDeviMonState()
	require.NoError(t, err)
	require.Equal(t, before.NextRunAt, after.NextRunAt)
	require.Equal(t, before.Snapshot.CollectedAt, after.Snapshot.CollectedAt)
	require.Equal(t, old.DurationMS, after.Snapshot.DurationMS)
	require.Equal(t, "90", after.Snapshot.Accounts[0].CreditBalance)
	require.Equal(t, old.Accounts[1], after.Snapshot.Accounts[1])
	require.True(t, after.Snapshot.UnpairedDevin[0].Disabled)
	require.EqualValues(t, 4, *after.Snapshot.UnpairedDevin[0].Routing.Weight)
	require.EqualValues(t, 1, countSystemTasks(t, task.Type))
}

func TestDeviMonRefreshDoesNotRewindOrResurrectAccounts(t *testing.T) {
	truncate(t)
	save := func(email, balance string, stamp int64) {
		t.Helper()
		raw, err := common.Marshal(deviMonReading{Account: &DeviMonAccount{Email: email, CreditBalance: balance}})
		require.NoError(t, err)
		require.NoError(t, model.SaveDeviMonAccountRefresh(context.Background(), &model.DeviMonAccountRefresh{Email: email, StartedAtUS: stamp, Data: string(raw)}))
	}
	save("a", "newer manual", 20)
	save("a", "late old request", 10)
	save("b", "older manual", 30)
	save("gone", "removed", 40)
	snap := DeviMonSnapshot{StartedAtUS: 5, Accounts: []DeviMonAccount{{Email: "a", ObservedAtUS: 15}, {Email: "b", CreditBalance: "newer full", ObservedAtUS: 35}}}
	require.NoError(t, applyDeviMonRefreshes(&snap))
	require.Len(t, snap.Accounts, 2)
	require.Equal(t, "newer manual", snap.Accounts[0].CreditBalance)
	require.Equal(t, "newer full", snap.Accounts[1].CreditBalance)
	require.NoError(t, model.DeleteDeviMonRefreshesBefore(30))
	left, err := model.GetDeviMonRefreshesSince(0)
	require.NoError(t, err)
	require.Len(t, left, 2)
}
