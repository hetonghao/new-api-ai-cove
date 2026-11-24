package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deviMonReplaceBase points the collector at a local server and removes the
// inter-account gap, returning a restore function for t.Cleanup.
func deviMonReplaceBase(t *testing.T, base string) func() {
	t.Helper()
	t.Setenv("CPA_MANAGEMENT_KEY", "test-key")
	codexBase, devinBase := deviMonCodexBase, deviMonDevinBase
	gapMin, gapMax := deviMonAccountGapMin, deviMonAccountGapMax
	deviMonCodexBase, deviMonDevinBase = base, base
	deviMonAccountGapMin, deviMonAccountGapMax = 0, 0
	return func() {
		deviMonCodexBase, deviMonDevinBase = codexBase, devinBase
		deviMonAccountGapMin, deviMonAccountGapMax = gapMin, gapMax
	}
}

// deviMonTestAccount builds one Codex credential entry as CPA lists it.
func deviMonTestAccount(email string, index string, accountID string, snapshotUntil string) map[string]any {
	idToken := map[string]any{"chatgpt_account_id": accountID}
	if snapshotUntil != "" {
		idToken["chatgpt_subscription_active_until"] = snapshotUntil
	}
	return map[string]any{
		"email":      email,
		"auth_index": index,
		"provider":   "codex",
		"disabled":   false,
		"id_token":   idToken,
	}
}

// deviMonFakeUpstream serves the CPA management API plus the Codex endpoints
// that api-call proxies, so the collector runs end to end without a network.
func deviMonFakeUpstream(t *testing.T, usage map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v0/management/auth-files":
			assert.Equal(t, "Bearer test-key", request.Header.Get("Authorization"))
			_ = json.NewEncoder(writer).Encode(map[string]any{"files": usage["files"]})
		case "/v0/management/api-call":
			var body map[string]any
			require.NoError(t, json.NewDecoder(request.Body).Decode(&body))
			target, _ := body["url"].(string)
			index, _ := body["auth_index"].(string)
			reply := map[string]any{"status_code": 200, "body": "{}"}
			// Per-account bodies keyed by auth_index keep both accounts distinct.
			perAccount, _ := usage[index].(map[string]string)
			switch {
			case strings.HasSuffix(target, "/wham/usage"):
				reply["body"] = perAccount["usage"]
			case strings.HasSuffix(target, "/wham/rate-limit-reset-credits"):
				reply["body"] = perAccount["cards"]
			case strings.Contains(target, "/subscriptions"):
				reply["body"] = perAccount["subscription"]
			}
			if reply["body"] == "" {
				reply["body"] = "{}"
			}
			_ = json.NewEncoder(writer).Encode(reply)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestDeviMonCollectsAllowanceCardsAndDevinState(t *testing.T) {
	upstream := deviMonFakeUpstream(t, map[string]any{
		"files": []any{
			deviMonTestAccount("late@example.com", "idx-late", "acct-late", "2026-11-18T05:49:34+00:00"),
			deviMonTestAccount("early@example.com", "idx-early", "acct-early", ""),
			map[string]any{"email": "early@example.com", "auth_index": "d-early", "provider": "devin", "disabled": true},
		},
		"idx-early": map[string]string{
			"usage":        `{"plan_type":"pro","account_id":"acct-early","rate_limit":{"primary_window":{"used_percent":25,"reset_at":1795865529}},"credits":{"balance":"22824.03","unlimited":false}}`,
			"cards":        `{"credits":[{"status":"available","expires_at":"2026-12-13T06:39:00Z"},{"status":"redeemed","expires_at":"2026-12-01T04:03:00Z"}]}`,
			"subscription": `{"active_until":"2026-12-18T05:49:34Z"}`,
		},
		"idx-late": map[string]string{
			"usage":        `{"plan_type":"pro","account_id":"acct-late","rate_limit":{"primary_window":{"used_percent":73,"reset_at":1796066400}},"credits":{"balance":"15873.43","unlimited":false}}`,
			"cards":        `{}`,
			"subscription": `{"active_until":"2026-12-20T05:49:34Z"}`,
		},
	})
	defer upstream.Close()

	t.Cleanup(deviMonReplaceBase(t, upstream.URL))
	snapshot, err := collectDeviMon(context.Background())
	require.NoError(t, err)
	require.Len(t, snapshot.Accounts, 2)

	// Ordered by weekly reset time, earliest first.
	assert.Equal(t, "early@example.com", snapshot.Accounts[0].Email)
	assert.Equal(t, "late@example.com", snapshot.Accounts[1].Email)
	assert.Equal(t, "", snapshot.Accounts[0].Error)
	require.NotNil(t, snapshot.Accounts[0].UsedPercent)
	assert.InDelta(t, 25, *snapshot.Accounts[0].UsedPercent, 0.01)
	assert.Equal(t, "22824.03", snapshot.Accounts[0].CreditBalance)
	require.Len(t, snapshot.Accounts[0].Cards, 1)
	assert.Equal(t, "2026-12-13T06:39:00Z", snapshot.Accounts[0].Cards[0].ExpiresAt)
	require.NotNil(t, snapshot.Accounts[0].DevinDisabled)
	assert.True(t, *snapshot.Accounts[0].DevinDisabled)
	assert.Equal(t, &DeviMonRouting{}, snapshot.Accounts[0].DevinRouting)
	assert.Nil(t, snapshot.Accounts[1].DevinRouting)

	// The live subscription endpoint wins over the stale id_token snapshot:
	// late@example.com carries 2026-11-18 in id_token but renews on 2026-12-20.
	assert.Equal(t, "2026-12-20T05:49:34Z", snapshot.Accounts[1].RenewAt)
	assert.Equal(t, "live", snapshot.Accounts[1].RenewSource)
}

func TestDeviMonListsDevinAccountsWithoutCodexTwin(t *testing.T) {
	codex := deviMonTestAccount("paired@example.com", "idx-1", "acct-1", "")
	codex["priority"], codex["weight"] = 99, 88
	upstream := deviMonFakeUpstream(t, map[string]any{
		"files": []any{
			codex,
			map[string]any{"email": " Paired@Example.com ", "auth_index": "d-1", "provider": "devin", "disabled": true, "priority": -4, "weight": 0},
			map[string]any{"email": "orphan@example.com", "auth_index": "d-2", "provider": "devin", "disabled": false, "weight": 25},
		},
		"idx-1": map[string]string{"usage": `{}`, "cards": `{}`, "subscription": `{}`},
	})
	defer upstream.Close()

	t.Cleanup(deviMonReplaceBase(t, upstream.URL))
	snapshot, err := collectDeviMon(context.Background())
	require.NoError(t, err)
	require.Len(t, snapshot.Accounts, 1)
	require.NotNil(t, snapshot.Accounts[0].DevinDisabled)
	assert.True(t, *snapshot.Accounts[0].DevinDisabled)
	paired := snapshot.Accounts[0].DevinRouting
	require.NotNil(t, paired)
	require.NotNil(t, paired.Priority)
	require.NotNil(t, paired.Weight)
	assert.EqualValues(t, -4, *paired.Priority)
	assert.Zero(t, *paired.Weight)
	require.Len(t, snapshot.UnpairedDevin, 1)
	assert.Equal(t, "orphan@example.com", snapshot.UnpairedDevin[0].Email)
	assert.False(t, snapshot.UnpairedDevin[0].Disabled)
	orphan := snapshot.UnpairedDevin[0].Routing
	require.NotNil(t, orphan)
	assert.Nil(t, orphan.Priority)
	require.NotNil(t, orphan.Weight)
	assert.EqualValues(t, 25, *orphan.Weight)
}

func TestDeviMonRequiresManagementKey(t *testing.T) {
	t.Setenv("CPA_MANAGEMENT_KEY", "")
	_, err := collectDeviMon(context.Background())
	require.Error(t, err)
}

func TestDeviMonInterruptedCollectionKeepsLastCompleteInventoryAndSharedSchedule(t *testing.T) {
	truncate(t)
	handler := deviMonTask{}
	withSystemTaskRegistry(t, handler)
	var interrupt atomic.Pointer[context.CancelFunc]
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v0/management/auth-files" {
			_, _ = w.Write([]byte(`{"files":[{"provider":"codex","email":"first@example.com","auth_index":"1"},{"provider":"codex","email":"user888@example.com","auth_index":"2"},{"provider":"devin","email":"user888@example.com","disabled":true}]}`))
			return
		}
		if cancel := interrupt.Load(); cancel != nil {
			(*cancel)()
		}
		_, _ = w.Write([]byte(`{"status_code":200,"body":"{}"}`))
	}))
	defer upstream.Close()
	t.Cleanup(deviMonReplaceBase(t, upstream.URL))

	runSystemTaskScheduler()
	queued, created, err := EnqueueSystemTask(handler.Type(), nil)
	require.NoError(t, err)
	require.False(t, created, "manual and scheduled collection share one job")
	claim := func(task *model.SystemTask) *model.SystemTask {
		t.Helper()
		claimed, ok, err := model.ClaimSystemTask(task.ID, task.Type, "primary", common.GetTimestamp()+60)
		require.NoError(t, err)
		require.True(t, ok)
		return claimed
	}
	handler.Run(context.Background(), claim(queued), "primary")
	state, err := GetDeviMonState()
	require.NoError(t, err)
	require.NotNil(t, state.Snapshot)
	require.Len(t, state.Snapshot.Accounts, 2)
	require.Equal(t, "user888@example.com", state.Snapshot.Accounts[1].Email)
	require.Empty(t, state.Snapshot.UnpairedDevin)
	complete := state.Snapshot

	queued, created, err = EnqueueSystemTask(handler.Type(), nil)
	require.NoError(t, err)
	require.True(t, created)
	state, err = GetDeviMonState()
	require.NoError(t, err)
	require.True(t, state.Collecting)
	require.Equal(t, complete, state.Snapshot)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	interrupt.Store(&cancel)
	deviMonAccountGapMin, deviMonAccountGapMax = time.Hour, time.Hour
	handler.Run(ctx, claim(queued), "primary")

	terminal, err := model.GetSystemTaskByTaskID(queued.TaskID)
	require.NoError(t, err)
	require.Equal(t, model.SystemTaskStatusFailed, terminal.Status)
	oldSettings := operation_setting.GetDeviMonSettings()
	t.Cleanup(func() { require.NoError(t, operation_setting.SetDeviMonSettings(oldSettings)) })
	require.NoError(t, operation_setting.SetDeviMonSettings(operation_setting.DeviMonSettings{IntervalMinutes: 23}))
	state, err = GetDeviMonState()
	require.NoError(t, err)
	require.False(t, state.Collecting)
	require.NotEmpty(t, state.LastError)
	require.Equal(t, terminal.UpdatedAt+23*60, state.NextRunAt)
	require.Equal(t, complete, state.Snapshot)
}

func TestDeviMonCannotPublishUnpairedAccountsFromFailedInventory(t *testing.T) {
	upstream := deviMonFakeUpstream(t, map[string]any{"files": []any{map[string]any{"email": "paired@example.com", "provider": "devin"}}})
	defer upstream.Close()
	t.Cleanup(deviMonReplaceBase(t, upstream.URL))
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer broken.Close()
	deviMonCodexBase = broken.URL
	snapshot, err := collectDeviMon(context.Background())
	require.ErrorContains(t, err, "cpa-pro-2: HTTP 502")
	require.Nil(t, snapshot)
}
