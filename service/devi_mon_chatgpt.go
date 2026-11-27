package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// Second subscription reading channel: the gateway signs in to ChatGPT itself
// and calls the same wham endpoints CPA proxies, so every account shape the
// monitor already understands keeps working without a CPA credential.
const (
	deviMonChatGPTClientID  = "app_EMoamEEZ73f0CkXaXp7hrann"
	deviMonChatGPTDeviceURL = "https://auth.openai.com/codex/device"
	deviMonChatGPTCallback  = "https://auth.openai.com/deviceauth/callback"
	deviMonAccessTokenSlack = 60 * time.Second
)

var (
	// Variable so tests can point every auth host at one local server.
	deviMonChatGPTAuthAPI  = "https://auth.openai.com/api/accounts"
	deviMonChatGPTTokenURL = "https://auth.openai.com/oauth/token"

	errDeviMonNoRefreshToken = errors.New("the ChatGPT sign-in has no refresh token")
)

// DeviMonDeviceCode is one pending sign-in. Nothing secret is shared: the code
// is useless without the operator approving it in a browser.
type DeviMonDeviceCode struct {
	DeviceAuthID    string `json:"device_auth_id"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	Interval        int    `json:"interval"`
	ExpiresAt       string `json:"expires_at"`
}

type deviMonChatGPTTokens struct {
	AccessToken  string
	RefreshToken string
	IDToken      string
	ExpiresIn    int
}

// DeviMonProxyURL is the egress proxy for the ChatGPT channel, as configured.
func DeviMonProxyURL() string {
	return strings.TrimSpace(operation_setting.GetDeviMonSettings().ChatGPTProxyURL)
}

func deviMonChatGPTProxy() string {
	return DeviMonProxyURL()
}

// deviMonChatGPTClient builds the direct client. An unset proxy keeps the
// process default so a deployment can route through its own egress.
func deviMonChatGPTClient(proxyURL string) (*http.Client, error) {
	parsed, err := common.ParseProxyURLStrict(proxyURL)
	if err != nil {
		return nil, errors.New("invalid proxy URL")
	}
	transport := newRelayHTTPTransport()
	client := &http.Client{Timeout: deviMonRequestTimeout, Transport: transport}
	if parsed == nil {
		return client, nil
	}
	if errConfigure := configureProxyTransport(transport, parsed); errConfigure != nil {
		return nil, errors.New("invalid proxy URL")
	}
	return client, nil
}

func deviMonChatGPTCall(ctx context.Context, client *http.Client, method, target string, body []byte, header map[string]string) (map[string]any, int, error) {
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, 0, errors.New("request failed")
	}
	for name, value := range header {
		if value != "" {
			request.Header.Set(name, value)
		}
	}
	if body != nil && request.Header.Get("Content-Type") == "" {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, 0, errors.New("request failed")
	}
	defer func() {
		_ = response.Body.Close()
	}()
	raw, err := io.ReadAll(io.LimitReader(response.Body, deviMonMaxResponseSize))
	if err != nil {
		return nil, response.StatusCode, errors.New("read response failed")
	}
	var payload map[string]any
	if len(raw) > 0 {
		if err := common.Unmarshal(raw, &payload); err != nil {
			payload = nil
		}
	}
	if response.StatusCode != http.StatusOK {
		return payload, response.StatusCode, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if payload == nil {
		return nil, response.StatusCode, errors.New("unreadable response")
	}
	return payload, response.StatusCode, nil
}

// --- device code sign-in -------------------------------------------------

// StartDeviMonChatGPTLogin asks OpenAI for a one-time code the operator enters
// on the Codex device page. Nothing is stored until the code is approved.
func StartDeviMonChatGPTLogin(ctx context.Context, proxyURL string) (DeviMonDeviceCode, error) {
	client, err := deviMonChatGPTClient(proxyURL)
	if err != nil {
		return DeviMonDeviceCode{}, err
	}
	body, err := common.Marshal(map[string]string{"client_id": deviMonChatGPTClientID})
	if err != nil {
		return DeviMonDeviceCode{}, err
	}
	payload, _, err := deviMonChatGPTCall(ctx, client, http.MethodPost,
		deviMonChatGPTAuthAPI+"/deviceauth/usercode", body, nil)
	if err != nil {
		return DeviMonDeviceCode{}, err
	}
	code := DeviMonDeviceCode{
		DeviceAuthID:    deviMonString(payload["device_auth_id"]),
		UserCode:        deviMonString(payload["user_code"]),
		VerificationURL: deviMonChatGPTDeviceURL,
		ExpiresAt:       deviMonString(payload["expires_at"]),
	}
	if code.DeviceAuthID == "" || code.UserCode == "" {
		return DeviMonDeviceCode{}, errors.New("device code request rejected")
	}
	code.Interval, _ = strconv.Atoi(deviMonString(payload["interval"]))
	if code.Interval < 1 {
		code.Interval = 5
	}
	return code, nil
}

// PollDeviMonChatGPTLogin reports done=false while the operator has not
// approved the code yet. An approved sign-in is stored immediately.
func PollDeviMonChatGPTLogin(ctx context.Context, proxyURL, deviceAuthID, userCode string) (bool, string, error) {
	client, err := deviMonChatGPTClient(proxyURL)
	if err != nil {
		return false, "", err
	}
	body, err := common.Marshal(map[string]string{"device_auth_id": deviceAuthID, "user_code": userCode})
	if err != nil {
		return false, "", err
	}
	payload, status, err := deviMonChatGPTCall(ctx, client, http.MethodPost,
		deviMonChatGPTAuthAPI+"/deviceauth/token", body, nil)
	// Both statuses mean "not approved yet" and are expected while polling.
	if status == http.StatusForbidden || status == http.StatusNotFound {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	grant, verifier := deviMonString(payload["authorization_code"]), deviMonString(payload["code_verifier"])
	if grant == "" || verifier == "" {
		return false, "", errors.New("device code was not approved")
	}
	tokens, err := deviMonChatGPTGrant(ctx, client, grant, verifier)
	if err != nil {
		return false, "", err
	}
	email, accountID, err := deviMonChatGPTIdentity(tokens)
	if err != nil {
		return false, "", err
	}
	row := model.DeviMonChatGPTAccount{
		Email:     email,
		AccountID: accountID,
		Channel:   model.DeviMonChannelChatGPT,
		ExpiresAt: deviMonTokenExpiry(tokens),
		CreatedAt: common.GetTimestamp(),
		UpdatedAt: common.GetTimestamp(),
	}
	if err := deviMonStoreChatGPTTokens(ctx, &row, tokens); err != nil {
		return false, "", err
	}
	return true, email, nil
}

func deviMonChatGPTGrant(ctx context.Context, client *http.Client, grant, verifier string) (deviMonChatGPTTokens, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {deviMonChatGPTClientID},
		"code":          {grant},
		"redirect_uri":  {deviMonChatGPTCallback},
		"code_verifier": {verifier},
	}
	return deviMonChatGPTTokenRequest(ctx, client, form)
}

func deviMonChatGPTRefresh(ctx context.Context, client *http.Client, refreshToken string) (deviMonChatGPTTokens, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {deviMonChatGPTClientID},
		"refresh_token": {refreshToken},
		"scope":         {"openid profile email"},
	}
	return deviMonChatGPTTokenRequest(ctx, client, form)
}

func deviMonChatGPTTokenRequest(ctx context.Context, client *http.Client, form url.Values) (deviMonChatGPTTokens, error) {
	payload, _, err := deviMonChatGPTCall(ctx, client, http.MethodPost, deviMonChatGPTTokenURL,
		[]byte(form.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"})
	if err != nil {
		return deviMonChatGPTTokens{}, err
	}
	tokens := deviMonChatGPTTokens{
		AccessToken:  deviMonString(payload["access_token"]),
		RefreshToken: deviMonString(payload["refresh_token"]),
		IDToken:      deviMonString(payload["id_token"]),
		ExpiresIn:    int(deviMonIntOr(payload["expires_in"])),
	}
	if tokens.AccessToken == "" {
		return deviMonChatGPTTokens{}, errors.New("the sign-in returned no access token")
	}
	return tokens, nil
}

// --- stored sign-in ------------------------------------------------------

func deviMonStoreChatGPTTokens(ctx context.Context, row *model.DeviMonChatGPTAccount, tokens deviMonChatGPTTokens) error {
	accessToken, err := common.EncryptCredential(tokens.AccessToken)
	if err != nil {
		return err
	}
	refreshToken := ""
	if tokens.RefreshToken != "" {
		if refreshToken, err = common.EncryptCredential(tokens.RefreshToken); err != nil {
			return err
		}
	}
	idToken := ""
	if tokens.IDToken != "" {
		if idToken, err = common.EncryptCredential(tokens.IDToken); err != nil {
			return err
		}
	}
	row.AccessToken, row.RefreshToken, row.IDToken = accessToken, refreshToken, idToken
	return model.SaveDeviMonChatGPTAccount(ctx, row)
}

// deviMonChatGPTAccess returns a usable access token, refreshing it first when
// it is about to expire. A rotated refresh token is written back.
func deviMonChatGPTAccess(ctx context.Context, row model.DeviMonChatGPTAccount) (string, error) {
	accessToken, err := common.DecryptCredential(row.AccessToken)
	if err != nil {
		return "", errors.New("the stored ChatGPT sign-in is unreadable")
	}
	if row.ExpiresAt > time.Now().Add(deviMonAccessTokenSlack).Unix() {
		return accessToken, nil
	}
	refreshToken, err := common.DecryptCredential(row.RefreshToken)
	if err != nil || refreshToken == "" {
		return "", errDeviMonNoRefreshToken
	}
	client, err := deviMonChatGPTClient(deviMonChatGPTProxy())
	if err != nil {
		return "", err
	}
	tokens, err := deviMonChatGPTRefresh(ctx, client, refreshToken)
	if err != nil {
		return "", err
	}
	if tokens.RefreshToken == "" {
		return "", errors.New("token refresh returned no refresh token")
	}
	idToken := row.IDToken
	if tokens.IDToken != "" {
		if idToken, err = common.EncryptCredential(tokens.IDToken); err != nil {
			return "", err
		}
	}
	encryptedAccess, err := common.EncryptCredential(tokens.AccessToken)
	if err != nil {
		return "", err
	}
	encryptedRefresh, err := common.EncryptCredential(tokens.RefreshToken)
	if err != nil {
		return "", err
	}
	if err := model.UpdateDeviMonChatGPTTokens(ctx, row.Email, encryptedAccess, encryptedRefresh, idToken, deviMonTokenExpiry(tokens)); err != nil {
		return "", err
	}
	return tokens.AccessToken, nil
}

// deviMonChatGPTIdentity reads the email and account id from the id_token.
func deviMonChatGPTIdentity(tokens deviMonChatGPTTokens) (string, string, error) {
	claims, err := deviMonJWTClaims(tokens.IDToken)
	if err != nil {
		if claims, err = deviMonJWTClaims(tokens.AccessToken); err != nil {
			return "", "", errors.New("the sign-in returned no identity")
		}
	}
	email := ""
	if profile, ok := claims["https://api.openai.com/profile"].(map[string]any); ok {
		email = deviMonString(profile["email"])
	}
	if email == "" {
		email = deviMonString(claims["email"])
	}
	if email == "" {
		return "", "", errors.New("the sign-in returned no email address")
	}
	accountID := ""
	if auth, ok := claims["https://api.openai.com/auth"].(map[string]any); ok {
		accountID = deviMonString(auth["chatgpt_account_id"])
	}
	return model.DeviMonChatGPTEmail(email), accountID, nil
}

func deviMonJWTClaims(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return nil, errors.New("not a JWT")
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil, errors.New("not a JWT")
	}
	var claims map[string]any
	if err := common.Unmarshal(raw, &claims); err != nil {
		return nil, errors.New("not a JWT")
	}
	return claims, nil
}

func deviMonTokenExpiry(tokens deviMonChatGPTTokens) int64 {
	if claims, err := deviMonJWTClaims(tokens.AccessToken); err == nil {
		if exp, ok := deviMonInt(claims["exp"]); ok && exp > 0 {
			return exp
		}
	}
	if tokens.ExpiresIn > 0 {
		return time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).Unix()
	}
	return 0
}

// --- reading the account through the ChatGPT channel --------------------

// deviMonChatGPTFile builds a credential entry shaped like a CPA Codex file, so
// the collector, the reset flow and the snapshot keep one code path.
func deviMonChatGPTFile(row model.DeviMonChatGPTAccount) map[string]any {
	return map[string]any{
		"email":      row.Email,
		"provider":   "codex",
		"channel":    model.DeviMonChannelChatGPT,
		"disabled":   false,
		"id_token":   map[string]any{"chatgpt_account_id": row.AccountID},
		"auth_index": "",
	}
}

func deviMonChatGPTHeaders(accessToken, accountID string) map[string]string {
	return map[string]string{
		"Authorization":      "Bearer " + accessToken,
		"ChatGPT-Account-Id": accountID,
		"Originator":         "codex_cli_rs",
		"User-Agent":         "codex_cli_rs",
	}
}
