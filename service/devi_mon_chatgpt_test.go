package service

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deviMonTestJWT is an unsigned token whose exp claim the cache logic can read.
func deviMonTestJWT(claims map[string]any) string {
	payload, _ := common.Marshal(claims)
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

// deviMonSessionJSON is what a signed-in browser shows at /api/auth/session.
func deviMonSessionJSON(email, accountID, sessionToken string) []byte {
	raw, _ := common.Marshal(map[string]any{
		"accessToken":  "access-token-from-my-browser",
		"sessionToken": sessionToken,
		"expires":      "2027-02-25T08:49:22.981Z",
		"user":         map[string]any{"email": email},
		"account":      map[string]any{"id": accountID},
	})
	return raw
}

// deviMonChatGPTUpstream serves the session exchange plus the three wham
// endpoints. The session banner echoes whichever cookie the caller sent so the
// chunking behaviour is observable.
func deviMonChatGPTUpstream(t *testing.T, email, accountID string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v0/management/auth-files":
			_, _ = writer.Write([]byte(`{"files":[]}`))
		case "/api/auth/session":
			sent := request.Header.Get("Cookie")
			if !strings.Contains(sent, "__Secure-next-auth.session-token") {
				writer.WriteHeader(http.StatusUnauthorized)
				return
			}
			raw, _ := common.Marshal(map[string]any{
				"accessToken":  "renewed-access-token",
				"sessionToken": "renewed-session-token",
				"expires":      "2027-02-25T08:49:22.981Z",
				"user":         map[string]any{"email": email},
				"account":      map[string]any{"id": accountID},
			})
			_, _ = writer.Write(raw)
		case "/backend-api/wham/usage":
			assert.Equal(t, "Bearer renewed-access-token", request.Header.Get("Authorization"))
			assert.Equal(t, accountID, request.Header.Get("ChatGPT-Account-Id"))
			_, _ = writer.Write([]byte(`{"account_id":"` + accountID + `","plan_type":"pro","rate_limit":{"primary_window":{"used_percent":42,"limit_window_seconds":604800,"reset_at":1795864535}},"credits":{"balance":"1234","unlimited":false}}`))
		case "/backend-api/wham/rate-limit-reset-credits":
			_, _ = writer.Write([]byte(`{"credits":[{"id":"credit-1","status":"available","expires_at":"2026-12-26T06:19:20Z"}]}`))
		case "/backend-api/subscriptions":
			_, _ = writer.Write([]byte(`{"active_until":"2026-12-21T11:07:14Z"}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
}

// deviMonReplaceChatGPTTargets points every read target at one local server.
func deviMonReplaceChatGPTTargets(t *testing.T, base string) {
	t.Helper()
	usageURL, cardsURL, subscribeURL, sessionAPI := deviMonUsageURL, deviMonCardsURL, deviMonSubscribeURL, deviMonChatGPTSessionAPI
	previous := operation_setting.GetDeviMonSettings()
	deviMonUsageURL = base + "/backend-api/wham/usage"
	deviMonCardsURL = base + "/backend-api/wham/rate-limit-reset-credits"
	deviMonSubscribeURL = base + "/backend-api/subscriptions"
	deviMonChatGPTSessionAPI = base + "/api/auth/session"
	require.NoError(t, operation_setting.SetDeviMonSettings(operation_setting.DeviMonSettings{IntervalMinutes: 180}))
	t.Cleanup(func() {
		deviMonUsageURL, deviMonCardsURL, deviMonSubscribeURL, deviMonChatGPTSessionAPI = usageURL, cardsURL, subscribeURL, sessionAPI
		require.NoError(t, operation_setting.SetDeviMonSettings(previous))
	})
}

// resetDeviMonChatGPTAccounts clears stored sessions before a test runs: the
// shared truncate helper only cleans up afterwards.
func resetDeviMonChatGPTAccounts(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.Exec("DELETE FROM devi_mon_chat_gpt_accounts").Error)
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM devi_mon_chat_gpt_accounts")
	})
}

func TestDeviMonSessionIsStoredEncryptedAndReadWithoutCpa(t *testing.T) {
	truncate(t)
	resetDeviMonChatGPTAccounts(t)
	upstream := deviMonChatGPTUpstream(t, "owned@example.test", "acct-owned")
	t.Cleanup(upstream.Close)
	t.Cleanup(deviMonReplaceBase(t, upstream.URL))
	deviMonReplaceChatGPTTargets(t, upstream.URL)

	account, err := DeviMonSaveSession(context.Background(), deviMonSessionJSON("owned@example.test", "acct-owned", "paste-me"))
	require.NoError(t, err)
	assert.Equal(t, "owned@example.test", account.Email)
	assert.Equal(t, "acct-owned", account.AccountID)

	// The stored columns must not hold the plaintext credential.
	row, err := model.GetDeviMonChatGPTAccount(context.Background(), "owned@example.test")
	require.NoError(t, err)
	assert.NotContains(t, row.SessionToken, "paste-me")
	assert.NotContains(t, row.AccessToken, "renewed-access-token")
	assert.Equal(t, model.DeviMonChannelChatGPT, row.Channel)

	snapshot, err := collectDeviMon(context.Background(), nil, false)
	require.NoError(t, err)
	require.Len(t, snapshot.Accounts, 1)
	reading := snapshot.Accounts[0]
	assert.Equal(t, model.DeviMonChannelChatGPT, reading.Channel)
	require.NotNil(t, reading.UsedPercent)
	assert.InDelta(t, 42, *reading.UsedPercent, 0.01)
	assert.Equal(t, "1234", reading.CreditBalance)
	require.Len(t, reading.Cards, 1)
	assert.Equal(t, "2026-12-21T11:07:14Z", reading.RenewAt)
	assert.NotEmpty(t, reading.ResetKey, "a single stored session keeps its reset key")
}

func TestDeviMonSessionRejectsAPasteWithNoIdentity(t *testing.T) {
	_, err := ParseDeviMonSession([]byte(`{"accessToken":"x"}`))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrDeviMonInvalidSession)

	_, err = ParseDeviMonSession([]byte("not json"))
	require.ErrorIs(t, err, ErrDeviMonInvalidSession)
}

// A cookie above 4096 bytes has to be split into .0 / .1 or ChatGPT ignores it.
func TestDeviMonLongSessionIsChunkedIntoTwoCookies(t *testing.T) {
	assert.Equal(t, "__Secure-next-auth.session-token=short", deviMonSessionCookie("short"))

	long := strings.Repeat("a", deviMonSessionChunkSize) + strings.Repeat("b", 560)
	cookie := deviMonSessionCookie(long)
	assert.Contains(t, cookie, "__Secure-next-auth.session-token.0=")
	assert.Contains(t, cookie, "__Secure-next-auth.session-token.1=")
	assert.NotContains(t, cookie, "__Secure-next-auth.session-token=")
	// Reassembling the chunks must reproduce the original token.
	joined := strings.ReplaceAll(strings.ReplaceAll(cookie, "__Secure-next-auth.session-token.0=", ""), "__Secure-next-auth.session-token.1=", "")
	joined = strings.ReplaceAll(joined, "; ", "")
	assert.Equal(t, long, joined)
}

// The enable/disable switch is a Devin-side operation: it flips cpa-pro-1's
// credential, and the reading channel has nothing to do with it.
func TestDeviMonDevinStatusResolvesCredentialByEmail(t *testing.T) {
	truncate(t)
	resetDeviMonChatGPTAccounts(t)

	var patched map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.URL.Path == "/v0/management/auth-files":
			raw, _ := common.Marshal(map[string]any{"files": []any{
				map[string]any{"name": "devin-user-abc.json", "email": "owned@example.test", "provider": "devin", "disabled": false},
				map[string]any{"name": "devin-other.json", "email": "other@example.test", "provider": "devin", "disabled": false},
			}})
			_, _ = writer.Write(raw)
		case request.URL.Path == "/v0/management/auth-files/status" && request.Method == http.MethodPatch:
			require.NoError(t, common.DecodeJson(request.Body, &patched))
			_, _ = writer.Write([]byte(`{"status":"ok","disabled":true}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(upstream.Close)
	t.Cleanup(deviMonReplaceBase(t, upstream.URL))

	require.NoError(t, SetDeviMonDevinDisabled(context.Background(), "OWNED@example.test", true))
	assert.Equal(t, map[string]any{"name": "devin-user-abc.json", "disabled": true}, patched)

	// An unknown mailbox must not touch anything.
	require.ErrorIs(t, SetDeviMonDevinDisabled(context.Background(), "nobody@example.test", true), ErrDeviMonDevinAccountMissing)

	// The flip is recorded as a Devin-only reading: it must not spend a Codex
	// quota probe, and applying it must leave the collection's usage untouched.
	snap := DeviMonSnapshot{StartedAtUS: 1, Accounts: []DeviMonAccount{{Email: "owned@example.test", UsedPercent: ptrFloat(7), CreditBalance: "99"}}}
	require.NoError(t, applyDeviMonRefreshes(&snap))
	require.Len(t, snap.Accounts, 1)
	require.NotNil(t, snap.Accounts[0].DevinDisabled)
	assert.True(t, *snap.Accounts[0].DevinDisabled)
	assert.Equal(t, "99", snap.Accounts[0].CreditBalance)
	require.NotNil(t, snap.Accounts[0].UsedPercent)
	assert.InDelta(t, 7, *snap.Accounts[0].UsedPercent, 0.001)
}

// ptrFloat gives a usage reading a stable address.
func ptrFloat(value float64) *float64 { return &value }

// /api/auth/session reports the SESSION expiry, which outlives the access token
// by weeks. Caching on it served a dead token long after ChatGPT answered 401.
func TestDeviMonExpiredAccessTokenIsRenewedDespiteFarSessionExpiry(t *testing.T) {
	truncate(t)
	resetDeviMonChatGPTAccounts(t)
	upstream := deviMonChatGPTUpstream(t, "stale@example.test", "acct-stale")
	t.Cleanup(upstream.Close)
	t.Cleanup(deviMonReplaceBase(t, upstream.URL))
	deviMonReplaceChatGPTTargets(t, upstream.URL)

	// A token that expired an hour ago, under a session valid until 2027.
	expired := deviMonTestJWT(map[string]any{"exp": common.GetTimestamp() - 3600})
	encryptedSession, err := freshEncrypt("paste-me")
	require.NoError(t, err)
	encryptedAccess, err := freshEncrypt(expired)
	require.NoError(t, err)
	future := common.GetTimestamp() + 90*24*3600
	require.NoError(t, model.SaveDeviMonChatGPTAccount(context.Background(), &model.DeviMonChatGPTAccount{
		Email: "stale@example.test", AccountID: "acct-stale", Channel: model.DeviMonChannelChatGPT,
		SessionToken: encryptedSession, AccessToken: encryptedAccess, ExpiresAt: future,
	}))

	// A fresh token must be fetched, so the read succeeds rather than 401ing.
	require.False(t, deviMonTokenFresh(expired), "an expired token is never fresh")
	snapshot, err := collectDeviMon(context.Background(), nil, false)
	require.NoError(t, err)
	require.Len(t, snapshot.Accounts, 1)
	assert.Equal(t, "", snapshot.Accounts[0].Error)
	require.NotNil(t, snapshot.Accounts[0].UsedPercent)
	assert.InDelta(t, 42, *snapshot.Accounts[0].UsedPercent, 0.01)
}

func freshEncrypt(value string) (string, error) {
	return common.EncryptCredential(value)
}

func TestDeviMonChunkedRenewalCookieIsReassembled(t *testing.T) {
	whole := strings.Repeat("a", deviMonSessionChunkSize) + strings.Repeat("b", 100)
	response := &http.Response{Header: http.Header{}}
	response.Header.Add("Set-Cookie", "__Secure-next-auth.session-token.0="+whole[:deviMonSessionChunkSize]+"; Path=/")
	response.Header.Add("Set-Cookie", "__Secure-next-auth.session-token.1="+whole[deviMonSessionChunkSize:]+"; Path=/")
	assert.Equal(t, whole, deviMonSessionFromResponse(response, "old"), "both chunks must be joined")

	// A response without a session cookie keeps what was already stored.
	assert.Equal(t, "old", deviMonSessionFromResponse(&http.Response{Header: http.Header{}}, "old"))
}

func TestDeviMonSessionChannelYieldsToTheCpaSwitch(t *testing.T) {
	truncate(t)
	resetDeviMonChatGPTAccounts(t)
	upstream := deviMonChatGPTUpstream(t, "both@example.test", "acct-both")
	t.Cleanup(upstream.Close)
	t.Cleanup(deviMonReplaceBase(t, upstream.URL))
	deviMonReplaceChatGPTTargets(t, upstream.URL)
	_, err := DeviMonSaveSession(context.Background(), deviMonSessionJSON("both@example.test", "acct-both", "paste-me"))
	require.NoError(t, err)
	swapInventory(t, []any{deviMonTestAccount("both@example.test", "idx-both", "acct-both", "")})

	// The stored session shadows the CPA credential with the same mailbox.
	snapshot, err := collectDeviMon(context.Background(), nil, false)
	require.NoError(t, err)
	require.Len(t, snapshot.Accounts, 1)
	assert.Equal(t, model.DeviMonChannelChatGPT, snapshot.Accounts[0].Channel)

	// Moving the switch back exposes the CPA credential again.
	require.NoError(t, SetDeviMonAccountChannel(context.Background(), "both@example.test", model.DeviMonChannelCPA))
	snapshot, err = collectDeviMon(context.Background(), nil, false)
	require.NoError(t, err)
	require.Len(t, snapshot.Accounts, 1)
	assert.Equal(t, model.DeviMonChannelCPA, snapshot.Accounts[0].Channel)
}

// swapInventory replaces the CPA credential list and the wham responses CPA
// proxies for one test, so both channels can be served side by side.
func swapInventory(t *testing.T, files []any) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Path == "/v0/management/auth-files" {
			raw, _ := common.Marshal(map[string]any{"files": files})
			_, _ = writer.Write(raw)
			return
		}
		var call struct {
			URL string `json:"url"`
		}
		_ = common.DecodeJson(request.Body, &call)
		body := "{}"
		switch {
		case strings.HasSuffix(call.URL, "/wham/usage"):
			body = `{"plan_type":"pro","account_id":"acct-both","rate_limit":{"primary_window":{"used_percent":10,"limit_window_seconds":604800}}}`
		case strings.Contains(call.URL, "/subscriptions"):
			body = `{"active_until":"2026-12-20T00:00:00Z"}`
		}
		raw, _ := common.Marshal(map[string]any{"status_code": 200, "body": body})
		_, _ = writer.Write(raw)
	}))
	t.Cleanup(server.Close)
	t.Cleanup(deviMonReplaceBase(t, server.URL))
}

func TestDeviMonAccountChannelSwitchesOnlyWithAStoredSession(t *testing.T) {
	truncate(t)
	resetDeviMonChatGPTAccounts(t)
	ctx := context.Background()
	require.ErrorIs(t, SetDeviMonAccountChannel(ctx, "nobody@example.test", model.DeviMonChannelChatGPT), ErrDeviMonChatGPTAccountMissing)
	require.Error(t, SetDeviMonAccountChannel(ctx, "nobody@example.test", "bogus"))

	upstream := deviMonChatGPTUpstream(t, "switch@example.test", "acct-switch")
	t.Cleanup(upstream.Close)
	t.Cleanup(deviMonReplaceBase(t, upstream.URL))
	deviMonReplaceChatGPTTargets(t, upstream.URL)
	_, err := DeviMonSaveSession(ctx, deviMonSessionJSON("switch@example.test", "acct-switch", "paste-me"))
	require.NoError(t, err)

	require.NoError(t, SetDeviMonAccountChannel(ctx, "switch@example.test", model.DeviMonChannelCPA))
	row, err := model.GetDeviMonChatGPTAccount(ctx, "switch@example.test")
	require.NoError(t, err)
	assert.Equal(t, model.DeviMonChannelCPA, row.Channel)
	assert.NotEmpty(t, row.SessionToken, "switching back to CPA keeps the session")
	require.NoError(t, SetDeviMonAccountChannel(ctx, "switch@example.test", model.DeviMonChannelChatGPT))

	views, err := GetDeviMonChatGPTAccounts(ctx)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.True(t, views[0].SignedIn)
	assert.Equal(t, model.DeviMonChannelChatGPT, views[0].Channel)
}
