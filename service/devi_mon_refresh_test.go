package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
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

func TestDeviMonResetRechecksUsageAndDoesNotSpendAnotherCard(t *testing.T) {
	for _, tc := range []struct {
		name       string
		manual     bool
		code       int
		staleUsage bool
		outcome    DeviMonResetOutcome
	}{
		{name: "manual refresh requires confirmation", manual: true, code: 200, outcome: deviMonResetApplied},
		{name: "scheduler refreshes after a lost redemption response", code: 502, outcome: deviMonResetUnverified},
		{name: "scheduler hides the stale reading after a reset", code: 200, staleUsage: true, outcome: deviMonResetUnverified},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			ctx := context.Background()
			handler := deviMonTask{}
			withSystemTaskRegistry(t, handler)
			now := time.Now()
			end := now.Add(time.Hour).Unix()
			const enabled = "selected@example.test"
			const other = "opt-out@example.test"
			var posts, usageAfter, cardsAfter atomic.Int32
			var paused atomic.Bool
			paused.Store(true)
			files := []any{deviMonTestAccount(enabled, "selected", "acct-selected", ""), deviMonTestAccount(other, "opt-out", "acct-other", "")}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v0/management/auth-files/status" {
					var body struct {
						Name     string
						Disabled bool
					}
					require.NoError(t, common.DecodeJson(r.Body, &body))
					require.Equal(t, "selected-devin.json", body.Name)
					require.False(t, body.Disabled)
					paused.Store(false)
					_, _ = w.Write([]byte(`{"disabled":false}`))
					return
				}
				if r.URL.Path == "/v0/management/auth-files" {
					devin := map[string]any{"provider": "devin", "name": "selected-devin.json", "auth_index": "devin-selected", "email": enabled, "disabled": paused.Load()}
					body, err := common.Marshal(map[string]any{"files": append(files, devin)})
					if assert.NoError(t, err) {
						_, _ = w.Write(body)
					}
					return
				}
				assert.Equal(t, "/v0/management/api-call", r.URL.Path)
				var call struct {
					Index  string            `json:"auth_index"`
					URL    string            `json:"url"`
					Method string            `json:"method"`
					Header map[string]string `json:"header"`
					Data   string            `json:"data"`
				}
				if err := common.DecodeJson(r.Body, &call); !assert.NoError(t, err) {
					w.WriteHeader(400)
					return
				}
				didSpend := posts.Load() > 0 && call.Index == "selected"
				status := 200
				body := `{}`
				switch call.URL {
				case deviMonUsageURL:
					used, resetAt := 100, end
					if didSpend {
						usageAfter.Add(1)
						if !tc.staleUsage {
							used, resetAt = 21, end+deviMonWeeklySeconds
						}
					}
					body = fmt.Sprintf(`{"rate_limit":{"primary_window":{"used_percent":17,"limit_window_seconds":18000},"secondary_window":{"used_percent":%d,"reset_at":%d,"limit_window_seconds":604800}}}`, used, resetAt)
				case deviMonCardsURL:
					count := `{"id":"late","status":"available","expires_at":%q}`
					cards := fmt.Sprintf(count, now.Add(4*time.Hour).Format(time.RFC3339))
					if didSpend {
						cardsAfter.Add(1)
					} else {
						cards += fmt.Sprintf(`,{"id":"expired","status":"available","expires_at":%q},{"id":"used","status":"redeemed","expires_at":%q},{"id":"first","status":"available","expires_at":%q}`,
							now.Add(-time.Hour).Format(time.RFC3339), now.Add(time.Minute).Format(time.RFC3339), now.Add(2*time.Hour).Format(time.RFC3339))
					}
					body = `{"credits":[` + cards + `]}`
				case deviMonCardsURL + "/consume":
					posts.Add(1)
					assert.Equal(t, http.MethodPost, call.Method)
					assert.Equal(t, "selected", call.Index)
					assert.Equal(t, "acct-selected", call.Header["ChatGPT-Account-Id"])
					var request map[string]string
					assert.NoError(t, common.UnmarshalJsonStr(call.Data, &request))
					assert.Equal(t, "first", request["credit_id"])
					assert.NotEmpty(t, request["redeem_request_id"])
					status, body = tc.code, `{"code":"reset","windows_reset":1}`
				}
				raw, err := common.Marshal(map[string]any{"status_code": status, "body": body})
				if assert.NoError(t, err) {
					_, _ = w.Write(raw)
				}
			}))
			t.Cleanup(srv.Close)
			t.Cleanup(deviMonReplaceBase(t, srv.URL))
			run := func(task *model.SystemTask) {
				t.Helper()
				claimed, ok, err := model.ClaimSystemTask(task.ID, task.Type, "test", common.GetTimestamp()+60)
				require.NoError(t, err)
				require.True(t, ok)
				handler.Run(ctx, claimed, "test")
			}
			find := func(email string) DeviMonAccount {
				t.Helper()
				state, err := GetDeviMonState()
				require.NoError(t, err)
				require.NotNil(t, state.Snapshot)
				for _, account := range state.Snapshot.Accounts {
					if account.Email == email {
						return account
					}
				}
				t.Fatalf("%s is missing from snapshot", email)
				return DeviMonAccount{}
			}
			task, _, err := EnqueueSystemTask(handler.Type(), nil)
			require.NoError(t, err)
			run(task)
			account := find(enabled)
			require.False(t, account.AutoReset)
			require.NoError(t, SetDeviMonAccountAutoReset(ctx, enabled, account.ResetKey, true))
			task, _, err = EnqueueSystemTask(handler.Type(), nil)
			require.NoError(t, err)
			run(task)
			require.NotNil(t, find(enabled).ResetOffer, "manual full collection presents an offer")
			require.Nil(t, find(other).ResetOffer, "the other exhausted account was not opted in")
			require.Zero(t, posts.Load(), "neither enabling nor manual collection consumes a card")
			if tc.manual {
				require.NoError(t, RefreshDeviMonAccount(ctx, enabled))
				offer := find(enabled).ResetOffer
				require.NotNil(t, offer)
				require.Zero(t, posts.Load(), "manual refresh must not consume a card")
				require.NoError(t, SetDeviMonAccountAutoReset(ctx, enabled, account.ResetKey, false))
				_, err := ConfirmDeviMonAccountReset(ctx, enabled, offer.ID)
				require.ErrorIs(t, err, ErrDeviMonResetOfferChanged)
				require.Zero(t, posts.Load(), "a disabled preference must invalidate confirmation")
				require.NoError(t, SetDeviMonAccountAutoReset(ctx, enabled, account.ResetKey, true))
				updated, err := ConfirmDeviMonAccountReset(ctx, enabled, offer.ID)
				require.NoError(t, err)
				require.Equal(t, tc.outcome, updated.ResetOutcome)
				require.Equal(t, 21.0, *updated.UsedPercent)
			} else {
				require.NoError(t, model.DB.Model(task).Update("updated_at", now.Add(-24*time.Hour).Unix()).Error)
				runSystemTaskScheduler(common.GetTimestamp())
				scheduled, err := model.GetActiveSystemTask(handler.Type())
				require.NoError(t, err)
				require.NotNil(t, scheduled)
				run(scheduled)
			}
			updated := find(enabled)
			require.Equal(t, tc.staleUsage, paused.Load(), "resume only with a verified reset")
			require.NotNil(t, updated.DevinDisabled)
			require.Equal(t, paused.Load(), *updated.DevinDisabled, "stored post-card reading must match CPA")
			require.Equal(t, tc.outcome, updated.ResetOutcome)
			require.Nil(t, updated.ResetOffer)
			require.Len(t, updated.Cards, 1)
			if tc.staleUsage {
				require.Nil(t, updated.UsedPercent, "pre-reset usage must not be published as a new reading")
			} else {
				require.NotNil(t, updated.UsedPercent)
				require.Equal(t, 21.0, *updated.UsedPercent)
			}
			require.Positive(t, usageAfter.Load())
			require.Positive(t, cardsAfter.Load())
			require.EqualValues(t, 1, posts.Load())
			require.NoError(t, RefreshDeviMonAccount(ctx, enabled))
			refreshed := find(enabled)
			require.Nil(t, refreshed.ResetOffer, "the same window cannot offer a second card")
			if tc.staleUsage {
				require.Nil(t, refreshed.UsedPercent, "another poll of the old window must not restore stale 0%")
				require.Equal(t, deviMonResetUnverified, refreshed.ResetOutcome)
			}
		})
	}
}

// Exercise real collection, independently stored policies, and CPA management
// writes. Only the local HTTP boundary is replaced.
func TestDeviMonQuotaPolicyTargetsPairedDevinAndWaitsForReset(t *testing.T) {
	truncate(t)
	const email = "quota-policy@example.test"
	ctx := context.Background()
	end := time.Now().Add(time.Hour).Unix()
	var disabled atomic.Bool
	var used, window, seconds atomic.Int64
	used.Store(100)
	window.Store(end)
	seconds.Store(deviMonWeeklySeconds)
	var statusWrites, cardsSpent atomic.Int32
	codex := deviMonTestAccount(email, "codex-index", "codex-id", "")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var data any
		switch r.URL.Path {
		case "/v0/management/auth-files":
			data = map[string]any{"files": []any{codex, map[string]any{"email": email, "provider": "devin", "name": "devin-one.json", "auth_index": "devin-one", "disabled": disabled.Load()}}}
		case "/v0/management/auth-files/status":
			var body struct {
				Name      string
				AuthIndex string `json:"auth_index"`
				Disabled  bool
			}
			require.NoError(t, common.DecodeJson(r.Body, &body))
			assert.Equal(t, "devin-one.json", body.Name)
			assert.Equal(t, "devin-one", body.AuthIndex)
			disabled.Store(body.Disabled)
			statusWrites.Add(1)
			data = map[string]bool{"disabled": body.Disabled}
		case "/v0/management/api-call":
			var call struct{ URL string }
			require.NoError(t, common.DecodeJson(r.Body, &call))
			body := "{}"
			if call.URL == deviMonUsageURL {
				body = fmt.Sprintf(`{"rate_limit":{"primary_window":{"limit_window_seconds":%d,"used_percent":%d,"reset_at":%d}}}`, seconds.Load(), used.Load(), window.Load())
			}
			if call.URL == deviMonCardsURL+"/consume" {
				cardsSpent.Add(1)
			}
			data = map[string]any{"status_code": 200, "body": body}
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(400)
			return
		}
		raw, err := common.Marshal(data)
		require.NoError(t, err)
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(deviMonReplaceBase(t, srv.URL))
	_, accountKey := deviMonCodexIdentity(codex)
	refresh := func() { t.Helper(); require.NoError(t, RefreshDeviMonAccount(ctx, email)) }
	// The displayed fallback for an unspecified interval is never sufficient to change routing.
	seconds.Store(0)
	refresh()
	require.Zero(t, statusWrites.Load())
	seconds.Store(deviMonWeeklySeconds)
	refresh()
	require.True(t, disabled.Load())
	require.EqualValues(t, 1, statusWrites.Load())
	refresh()
	require.EqualValues(t, 1, statusWrites.Load())
	used.Store(20)
	refresh()
	require.True(t, disabled.Load(), "a better reading in the same window is not a reset")
	window.Store(end + deviMonWeeklySeconds)
	refresh()
	require.False(t, disabled.Load())
	require.EqualValues(t, 2, statusWrites.Load())
	require.NoError(t, SetDeviMonAccountAutoReset(ctx, email, accountKey, true))
	used.Store(100)
	refresh()
	require.False(t, disabled.Load(), "card mode without a card never falls through to disable")
	require.NoError(t, SetDeviMonAccountAutoReset(ctx, email, accountKey, false))
	off := false
	require.NoError(t, SetDeviMonAccountPolicy(ctx, email, accountKey, nil, &off))
	refresh()
	require.True(t, disabled.Load())
	used.Store(0)
	window.Add(deviMonWeeklySeconds)
	refresh()
	require.True(t, disabled.Load(), "resume was turned off")
	on := true
	require.NoError(t, SetDeviMonAccountPolicy(ctx, email, accountKey, nil, &on))
	refresh()
	require.True(t, disabled.Load(), "enabling the preference is not a reset")
	used.Store(100)
	refresh()
	used.Store(0)
	window.Add(deviMonWeeklySeconds)
	refresh()
	require.False(t, disabled.Load(), "also recover a previously disabled account after observing its exhausted week reset")
	require.Zero(t, cardsSpent.Load())
}

func TestDeviMonRoutingUsesOnlyUniqueDevinAndKeepsRequestsSeparate(t *testing.T) {
	const email = "routing@example.test"
	ctx := context.Background()
	var duplicate atomic.Bool
	var writes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v0/management/auth-files":
			d := map[string]any{"provider": "devin", "email": email, "name": "devin.json", "priority": -7, "weight": 0, "success": 101, "failed": 9, "recent_requests": []any{map[string]int{"success": 5, "failed": 5}}}
			files := []any{map[string]any{"provider": "codex", "email": email, "name": "codex.json", "success": 2500}, d}
			if duplicate.Load() {
				files = append(files, d)
			}
			raw, err := common.Marshal(map[string]any{"files": files})
			require.NoError(t, err)
			_, _ = w.Write(raw)
		case "/v0/management/auth-files/fields":
			var body map[string]any
			require.NoError(t, common.DecodeJson(r.Body, &body))
			assert.Equal(t, http.MethodPatch, r.Method)
			assert.Equal(t, map[string]any{"name": "devin.json", "weight": float64(0)}, body)
			writes.Add(1)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			t.Errorf("unexpected call %s", r.URL.Path)
			w.WriteHeader(400)
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(deviMonReplaceBase(t, srv.URL))
	data, err := ListDeviMonDevin(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 101, data[email].Requests.Success)
	require.EqualValues(t, 9, data[email].Requests.Failed)
	require.Equal(t, []DeviMonRequestBucket{{Success: 5, Failed: 5}}, data[email].Requests.Recent)
	require.NoError(t, SetDeviMonRouting(ctx, strings.ToUpper(email), "weight", 0))
	require.ErrorIs(t, SetDeviMonRouting(ctx, email, "weight", 1_000_001), ErrDeviMonInvalidRouting)
	require.ErrorIs(t, SetDeviMonRouting(ctx, email, "disabled", 1), ErrDeviMonInvalidRouting)
	duplicate.Store(true)
	require.ErrorIs(t, SetDeviMonRouting(ctx, email, "priority", -2), ErrDeviMonDevinAccountMissing)
	require.EqualValues(t, 1, writes.Load())
}
