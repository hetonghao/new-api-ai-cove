package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	snapshot, err := RunDeviMon(context.Background())
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

	// The live subscription endpoint wins over the stale id_token snapshot:
	// late@example.com carries 2026-11-18 in id_token but renews on 2026-12-20.
	assert.Equal(t, "2026-12-20T05:49:34Z", snapshot.Accounts[1].RenewAt)
	assert.Equal(t, "live", snapshot.Accounts[1].RenewSource)
}

func TestDeviMonListsDevinAccountsWithoutCodexTwin(t *testing.T) {
	upstream := deviMonFakeUpstream(t, map[string]any{
		"files": []any{
			deviMonTestAccount("paired@example.com", "idx-1", "acct-1", ""),
			map[string]any{"email": "paired@example.com", "auth_index": "d-1", "provider": "devin", "disabled": true},
			map[string]any{"email": "orphan@example.com", "auth_index": "d-2", "provider": "devin", "disabled": false},
		},
		"idx-1": map[string]string{"usage": `{}`, "cards": `{}`, "subscription": `{}`},
	})
	defer upstream.Close()

	t.Cleanup(deviMonReplaceBase(t, upstream.URL))
	snapshot, err := RunDeviMon(context.Background())
	require.NoError(t, err)
	require.Len(t, snapshot.Accounts, 1)
	require.NotNil(t, snapshot.Accounts[0].DevinDisabled)
	assert.True(t, *snapshot.Accounts[0].DevinDisabled)
	require.Len(t, snapshot.UnpairedDevin, 1)
	assert.Equal(t, "orphan@example.com", snapshot.UnpairedDevin[0].Email)
	assert.False(t, snapshot.UnpairedDevin[0].Disabled)
}

func TestDeviMonRequiresManagementKey(t *testing.T) {
	t.Setenv("CPA_MANAGEMENT_KEY", "")
	_, err := RunDeviMon(context.Background())
	require.Error(t, err)
}
