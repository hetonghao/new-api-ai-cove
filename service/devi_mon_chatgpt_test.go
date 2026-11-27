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

// deviMonTestJWT builds an unsigned token whose claims are readable by the
// monitor. The signature is irrelevant: we never verify our own token.
func deviMonTestJWT(claims map[string]any) string {
	payload, _ := common.Marshal(claims)
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".signature"
}

func deviMonTestIDToken(email, accountID string) string {
	return deviMonTestJWT(map[string]any{
		"exp":                         common.GetTimestamp() + 3600,
		"email":                       email,
		"https://api.openai.com/auth": map[string]any{"chatgpt_account_id": accountID},
	})
}

// deviMonChatGPTUpstream serves the auth and wham endpoints for one sign-in.
func deviMonChatGPTUpstream(t *testing.T, email, accountID string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v0/management/auth-files":
			_, _ = writer.Write([]byte(`{"files":[]}`))
		case "/api/accounts/deviceauth/usercode":
			_, _ = writer.Write([]byte(`{"device_auth_id":"deviceauth_test","user_code":"TEST-CODE","interval":"5","expires_at":"2026-12-01T00:00:00Z"}`))
		case "/api/accounts/deviceauth/token":
			_, _ = writer.Write([]byte(`{"authorization_code":"grant","code_verifier":"verifier"}`))
		case "/oauth/token":
			body, _ := common.Marshal(map[string]string{
				"access_token":  deviMonTestIDToken(email, accountID),
				"refresh_token": "refresh-token",
				"id_token":      deviMonTestIDToken(email, accountID),
				"expires_in":    "3600",
			})
			_, _ = writer.Write(body)
		case "/backend-api/wham/usage":
			assert.Equal(t, "Bearer "+deviMonTestIDToken(email, accountID), request.Header.Get("Authorization"))
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

// deviMonReplaceChatGPTTargets points every auth and wham URL at one server and
// installs the settings the channel reads.
func deviMonReplaceChatGPTTargets(t *testing.T, base string) {
	t.Helper()
	usageURL, cardsURL, subscribeURL := deviMonUsageURL, deviMonCardsURL, deviMonSubscribeURL
	authAPI, tokenURL := deviMonChatGPTAuthAPI, deviMonChatGPTTokenURL
	previous := operation_setting.GetDeviMonSettings()
	deviMonUsageURL, deviMonCardsURL, deviMonSubscribeURL = base+"/backend-api/wham/usage", base+"/backend-api/wham/rate-limit-reset-credits", base+"/backend-api/subscriptions"
	deviMonChatGPTAuthAPI, deviMonChatGPTTokenURL = base+"/api/accounts", base+"/oauth/token"
	require.NoError(t, operation_setting.SetDeviMonSettings(operation_setting.DeviMonSettings{IntervalMinutes: 180}))
	t.Cleanup(func() {
		deviMonUsageURL, deviMonCardsURL, deviMonSubscribeURL = usageURL, cardsURL, subscribeURL
		deviMonChatGPTAuthAPI, deviMonChatGPTTokenURL = authAPI, tokenURL
		require.NoError(t, operation_setting.SetDeviMonSettings(previous))
	})
}

// storeDeviMonTestSignIn writes one encrypted sign-in row the way the device
// flow would.
func storeDeviMonTestSignIn(t *testing.T, email, accountID string) {
	t.Helper()
	access, err := common.EncryptCredential(deviMonTestIDToken(email, accountID))
	require.NoError(t, err)
	refresh, err := common.EncryptCredential("refresh-token")
	require.NoError(t, err)
	idToken, err := common.EncryptCredential(deviMonTestIDToken(email, accountID))
	require.NoError(t, err)
	require.NoError(t, model.SaveDeviMonChatGPTAccount(context.Background(), &model.DeviMonChatGPTAccount{
		Email: email, AccountID: accountID, Channel: model.DeviMonChannelChatGPT,
		AccessToken: access, RefreshToken: refresh, IDToken: idToken,
		ExpiresAt: common.GetTimestamp() + 3600,
	}))
}

// resetDeviMonChatGPTAccounts clears stored sign-ins before a test runs: the
// shared truncate helper only cleans up afterwards.
func resetDeviMonChatGPTAccounts(t *testing.T) {
	t.Helper()
	require.NoError(t, model.DB.Exec("DELETE FROM devi_mon_chat_gpt_accounts").Error)
	t.Cleanup(func() {
		model.DB.Exec("DELETE FROM devi_mon_chat_gpt_accounts")
	})
}

func TestDeviMonChatGPTChannelCollectsAccountWithoutCpa(t *testing.T) {
	truncate(t)
	resetDeviMonChatGPTAccounts(t)
	upstream := deviMonChatGPTUpstream(t, "owned@example.test", "acct-owned")
	t.Cleanup(upstream.Close)
	t.Cleanup(deviMonReplaceBase(t, upstream.URL))
	deviMonReplaceChatGPTTargets(t, upstream.URL)
	storeDeviMonTestSignIn(t, "owned@example.test", "acct-owned")

	snapshot, err := collectDeviMon(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, snapshot.Accounts, 1)
	account := snapshot.Accounts[0]
	assert.Equal(t, "owned@example.test", account.Email)
	assert.Equal(t, model.DeviMonChannelChatGPT, account.Channel)
	require.NotNil(t, account.UsedPercent)
	assert.InDelta(t, 42, *account.UsedPercent, 0.01)
	assert.Equal(t, "1234", account.CreditBalance)
	require.Len(t, account.Cards, 1)
	assert.Equal(t, "2026-12-21T11:07:14Z", account.RenewAt)
	assert.NotEmpty(t, account.ResetKey, "a single sign-in keeps its reset key")
}

// The switch wins: choosing the ChatGPT channel hides the CPA credential with
// the same mailbox, so one account is never listed twice.
func TestDeviMonChatGPTSwitchShadowsTheCpaCredential(t *testing.T) {
	truncate(t)
	resetDeviMonChatGPTAccounts(t)
	upstream := deviMonChatGPTUpstream(t, "both@example.test", "acct-both")
	t.Cleanup(upstream.Close)
	t.Cleanup(deviMonReplaceBase(t, upstream.URL))
	deviMonReplaceChatGPTTargets(t, upstream.URL)
	storeDeviMonTestSignIn(t, "both@example.test", "acct-both")
	// The same mailbox still lives in cpa-pro-2.
	swapInventory(t, []any{deviMonTestAccount("both@example.test", "idx-both", "acct-both", "")})

	snapshot, err := collectDeviMon(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, snapshot.Accounts, 1)
	assert.Equal(t, model.DeviMonChannelChatGPT, snapshot.Accounts[0].Channel)
	assert.InDelta(t, 42, *snapshot.Accounts[0].UsedPercent, 0.01, "the chatgpt channel supplied the reading")

	// Moving the switch back exposes the CPA credential again.
	require.NoError(t, SetDeviMonAccountChannel(context.Background(), "both@example.test", model.DeviMonChannelCPA))
	snapshot, err = collectDeviMon(context.Background(), nil)
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

func TestDeviMonAccountChannelSwitchesOnlyWithAStoredSignIn(t *testing.T) {
	truncate(t)
	resetDeviMonChatGPTAccounts(t)
	ctx := context.Background()
	require.ErrorIs(t, SetDeviMonAccountChannel(ctx, "nobody@example.test", model.DeviMonChannelChatGPT), ErrDeviMonChatGPTAccountMissing)
	require.Error(t, SetDeviMonAccountChannel(ctx, "nobody@example.test", "bogus"))

	storeDeviMonTestSignIn(t, "switch@example.test", "acct-switch")
	require.NoError(t, SetDeviMonAccountChannel(ctx, "switch@example.test", model.DeviMonChannelCPA))
	row, err := model.GetDeviMonChatGPTAccount(ctx, "switch@example.test")
	require.NoError(t, err)
	assert.Equal(t, model.DeviMonChannelCPA, row.Channel)
	assert.NotEmpty(t, row.AccessToken, "switching back to CPA keeps the sign-in")
	require.NoError(t, SetDeviMonAccountChannel(ctx, "switch@example.test", model.DeviMonChannelChatGPT))

	views, err := GetDeviMonChatGPTAccounts(ctx)
	require.NoError(t, err)
	require.Len(t, views, 1)
	assert.True(t, views[0].SignedIn)
	assert.Equal(t, model.DeviMonChannelChatGPT, views[0].Channel)
}
