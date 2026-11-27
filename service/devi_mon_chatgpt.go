package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// Second subscription reading channel: the operator pastes the JSON that
// https://chatgpt.com/api/auth/session returns in an already signed-in browser,
// and the gateway exchanges that session cookie for an access token itself.
//
// Why a cookie and not an OAuth sign-in: the device-code flow still demands the
// account's email verification code, which is exactly what an account that
// cannot sign in to Codex is missing. The session cookie is already issued, so
// it sidesteps the login step entirely.
const (
	deviMonChatGPTSessionURL = "https://chatgpt.com/api/auth/session"
	deviMonAccessTokenSlack  = 60 * time.Second
	// NextAuth splits cookies above 4096 bytes into .0 / .1 chunks. The pasted
	// JSON carries the whole value, so it has to be re-chunked per request.
	deviMonSessionChunkSize = 3933
)

var (
	// Variable so tests can serve the session endpoint locally.
	deviMonChatGPTSessionAPI = deviMonChatGPTSessionURL

	errDeviMonNoSession = errors.New("the stored ChatGPT session is unreadable")
)

// ErrDeviMonInvalidSession wraps the failures the operator can fix by pasting
// again: malformed JSON, a missing field, or a token ChatGPT no longer accepts.
var ErrDeviMonInvalidSession = errors.New("the pasted session is not usable")

// DeviMonSession is the subset of the pasted JSON the gateway keeps.
type DeviMonSession struct {
	Email        string
	AccountID    string
	SessionToken string
	AccessToken  string
	// ProxyURL applies to this account only; empty means direct.
	ProxyURL  string
	ExpiresAt int64
}

// ParseDeviMonSession accepts the JSON an operator copies from
// /api/auth/session. Only the fields the monitor needs are read, and the
// response's own access token is kept so the first read needs no extra call.
func ParseDeviMonSession(raw []byte) (DeviMonSession, error) {
	var payload struct {
		AccessToken  string `json:"accessToken"`
		SessionToken string `json:"sessionToken"`
		Expires      string `json:"expires"`
		User         struct {
			Email string `json:"email"`
		} `json:"user"`
		Account struct {
			ID string `json:"id"`
		} `json:"account"`
	}
	if err := common.Unmarshal(raw, &payload); err != nil {
		return DeviMonSession{}, fmt.Errorf("%w: that is not the session JSON", ErrDeviMonInvalidSession)
	}
	session := DeviMonSession{
		Email:        model.DeviMonChatGPTEmail(payload.User.Email),
		AccountID:    strings.TrimSpace(payload.Account.ID),
		SessionToken: strings.TrimSpace(payload.SessionToken),
		AccessToken:  strings.TrimSpace(payload.AccessToken),
	}
	if session.Email == "" || session.SessionToken == "" {
		return DeviMonSession{}, fmt.Errorf("%w: it is missing the email or the session token", ErrDeviMonInvalidSession)
	}
	if when, err := time.Parse(time.RFC3339, payload.Expires); err == nil {
		session.ExpiresAt = when.Unix()
	}
	return session, nil
}

// DeviMonProxyURL is the account's own egress proxy. Empty means direct.
func DeviMonProxyURL(row model.DeviMonChatGPTAccount) string {
	return strings.TrimSpace(row.ProxyURL)
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
		// Keep the transport reason: "no egress" and "bad token" are different
		// problems and a bare "request failed" cannot tell them apart.
		return nil, 0, fmt.Errorf("request failed: %w", err)
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

// deviMonSessionCookie re-chunks the stored token the way a browser sends it.
func deviMonSessionCookie(sessionToken string) string {
	if len(sessionToken) <= deviMonSessionChunkSize {
		return "__Secure-next-auth.session-token=" + sessionToken
	}
	parts := make([]string, 0, 2)
	for index, offset := 0, 0; offset < len(sessionToken); index, offset = index+1, offset+deviMonSessionChunkSize {
		end := min(offset+deviMonSessionChunkSize, len(sessionToken))
		parts = append(parts, fmt.Sprintf("__Secure-next-auth.session-token.%d=%s", index, sessionToken[offset:end]))
	}
	return strings.Join(parts, "; ")
}

// deviMonExchangeSession turns a stored session cookie into an access token.
// ChatGPT renews the cookie on every call, so the caller writes back both.
func deviMonExchangeSession(ctx context.Context, proxyURL, sessionToken string) (accessToken, accountID, renewed string, expiresAt int64, err error) {
	client, err := deviMonChatGPTClient(proxyURL)
	if err != nil {
		return "", "", "", 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, deviMonChatGPTSessionAPI, nil)
	if err != nil {
		return "", "", "", 0, errors.New("request failed")
	}
	request.Header.Set("Cookie", deviMonSessionCookie(sessionToken))
	request.Header.Set("User-Agent", "Mozilla/5.0")
	response, err := client.Do(request)
	if err != nil {
		return "", "", "", 0, errors.New("request failed")
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return "", "", "", 0, fmt.Errorf("%w: ChatGPT rejected it, sign in again and copy a fresh one", ErrDeviMonInvalidSession)
	}
	if response.StatusCode != http.StatusOK {
		return "", "", "", 0, fmt.Errorf("session exchange returned HTTP %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, deviMonMaxResponseSize))
	if err != nil {
		return "", "", "", 0, errors.New("read response failed")
	}
	session, err := ParseDeviMonSession(raw)
	if err != nil {
		return "", "", "", 0, fmt.Errorf("%w: ChatGPT returned no session, sign in again", ErrDeviMonInvalidSession)
	}
	if session.AccessToken == "" {
		return "", "", "", 0, errors.New("the ChatGPT session returned no access token")
	}
	// A renewal may hand back a new cookie. A chunked token arrives as .0 / .1,
	// so the pieces are reassembled instead of dropping one and storing half a
	// credential.
	renewed = deviMonSessionFromResponse(response, sessionToken)
	return session.AccessToken, session.AccountID, renewed, session.ExpiresAt, nil
}

func deviMonStoreSession(ctx context.Context, session DeviMonSession) error {
	encryptedSession, err := common.EncryptCredential(session.SessionToken)
	if err != nil {
		return err
	}
	encryptedAccess, err := common.EncryptCredential(session.AccessToken)
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	return model.SaveDeviMonChatGPTAccount(ctx, &model.DeviMonChatGPTAccount{
		Email:        session.Email,
		AccountID:    session.AccountID,
		Channel:      model.DeviMonChannelChatGPT,
		SessionToken: encryptedSession,
		AccessToken:  encryptedAccess,
		ProxyURL:     strings.TrimSpace(session.ProxyURL),
		ExpiresAt:    session.ExpiresAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
}

// DeviMonSaveSession stores a pasted session. The token is verified against
// ChatGPT before anything is written, so an expired paste is rejected instead
// of silently breaking the next collection.
func DeviMonSaveSession(ctx context.Context, raw []byte) (model.DeviMonChatGPTAccount, error) {
	pasted, err := ParseDeviMonSession(raw)
	if err != nil {
		return model.DeviMonChatGPTAccount{}, err
	}
	return deviMonStoreVerifiedSession(ctx, pasted)
}

// deviMonStoreVerifiedSession re-checks the session against ChatGPT before it is
// written, so a stale paste fails now instead of breaking the next collection.
func deviMonStoreVerifiedSession(ctx context.Context, pasted DeviMonSession) (model.DeviMonChatGPTAccount, error) {
	accessToken, accountID, renewed, expiresAt, err := deviMonExchangeSession(ctx, pasted.ProxyURL, pasted.SessionToken)
	if err != nil {
		return model.DeviMonChatGPTAccount{}, err
	}
	pasted.AccessToken = accessToken
	pasted.SessionToken = renewed
	if accountID != "" {
		pasted.AccountID = accountID
	}
	if expiresAt > 0 {
		pasted.ExpiresAt = expiresAt
	}
	if err := deviMonStoreSession(ctx, pasted); err != nil {
		return model.DeviMonChatGPTAccount{}, err
	}
	return model.GetDeviMonChatGPTAccount(ctx, pasted.Email)
}

// deviMonSessionAccess returns a usable access token, renewing from the stored
// cookie when the cached one is about to expire.
//
// The freshness check reads the cached token's own exp claim, not the row's
// ExpiresAt: /api/auth/session returns the session's expiry, which outlives the
// access token by weeks. Trusting it would keep serving a dead token until long
// after ChatGPT started answering 401.
func deviMonSessionAccess(ctx context.Context, row model.DeviMonChatGPTAccount) (string, error) {
	cached, decryptErr := common.DecryptCredential(row.AccessToken)
	if decryptErr == nil && cached != "" && deviMonTokenFresh(cached) {
		return cached, nil
	}
	sessionToken, err := common.DecryptCredential(row.SessionToken)
	if err != nil || sessionToken == "" {
		return "", errDeviMonNoSession
	}
	accessToken, accountID, renewed, expiresAt, err := deviMonExchangeSession(ctx, DeviMonProxyURL(row), sessionToken)
	if err != nil {
		return "", err
	}
	encryptedSession, err := common.EncryptCredential(renewed)
	if err != nil {
		return "", err
	}
	encryptedAccess, err := common.EncryptCredential(accessToken)
	if err != nil {
		return "", err
	}
	if err := model.UpdateDeviMonChatGPTSession(ctx, row.Email, encryptedSession, encryptedAccess, accountID, expiresAt); err != nil {
		return "", err
	}
	return accessToken, nil
}

// deviMonJWTExp reads the exp claim an access token carries.
func deviMonJWTExp(token string) (int64, bool) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return 0, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return 0, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := common.Unmarshal(raw, &claims); err != nil || claims.Exp <= 0 {
		return 0, false
	}
	return claims.Exp, true
}

// deviMonTokenFresh reports whether a cached access token still has usable
// life. A token whose exp cannot be read is treated as stale, so a malformed
// cache costs one renewal instead of silently serving garbage.
func deviMonTokenFresh(token string) bool {
	exp, ok := deviMonJWTExp(token)
	return ok && exp > time.Now().Add(deviMonAccessTokenSlack).Unix()
}

// deviMonSessionFromResponse picks the renewed session cookie out of a
// response, reassembling NextAuth's .0 / .1 chunks when it was split.
func deviMonSessionFromResponse(response *http.Response, current string) string {
	whole := ""
	chunks := map[int]string{}
	for _, cookie := range response.Cookies() {
		if cookie.Value == "" {
			continue
		}
		switch name := cookie.Name; {
		case name == "__Secure-next-auth.session-token":
			whole = cookie.Value
		case strings.HasPrefix(name, "__Secure-next-auth.session-token."):
			var index int
			if _, err := fmt.Sscanf(strings.TrimPrefix(name, "__Secure-next-auth.session-token."), "%d", &index); err == nil {
				chunks[index] = cookie.Value
			}
		}
	}
	if len(chunks) > 0 {
		var builder strings.Builder
		for index := 0; index < len(chunks); index++ {
			piece, ok := chunks[index]
			if !ok {
				return current
			}
			builder.WriteString(piece)
		}
		return builder.String()
	}
	if whole != "" {
		return whole
	}
	return current
}

// deviMonChatGPTHeaders are the headers a Codex client sends to the wham
// endpoints; the account id selects the workspace.
func deviMonChatGPTHeaders(accessToken, accountID string) map[string]string {
	return map[string]string{
		"Authorization":      "Bearer " + accessToken,
		"ChatGPT-Account-Id": accountID,
		"Originator":         "codex_cli_rs",
		"User-Agent":         "codex_cli_rs",
	}
}

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
