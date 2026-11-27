package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// The Devin monitor reads two CPA instances through their management API.
// cpa-pro-2 holds the Codex accounts and cpa-pro-1 holds the Devin accounts;
// both are plain hostnames already resolvable from the new-api container.
const (
	deviMonInterruptedRetryDelay = time.Minute
	deviMonRequestTimeout        = 45 * time.Second
	deviMonMaxResponseSize       = 1 << 20
)

// Read targets stay variable so tests can serve them locally.
var (
	deviMonUsageURL     = "https://chatgpt.com/backend-api/wham/usage"
	deviMonCardsURL     = "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits"
	deviMonSubscribeURL = "https://chatgpt.com/backend-api/subscriptions"
)

// Collection targets stay variable so tests can point them at a local server.
// The account gap is deliberately wide: every probe costs a real upstream call.
var (
	deviMonCodexBase     = "http://cpa-pro:8318"
	deviMonDevinBase     = "http://cpa-pro-1:8319"
	deviMonAccountGapMin = 10 * time.Second
	deviMonAccountGapMax = 60 * time.Second
)

// DeviMonCard is one unused Codex rate-limit reset credit.
type DeviMonCard struct {
	ExpiresAt string `json:"expires_at"`
}

// DeviMonAccount is one Codex account with its weekly allowance, credits and
// reset cards, paired with the configuration of the same-email Devin account.
type DeviMonAccount struct {
	ObservedAtUS         int64            `json:"observed_at_us,omitempty"`
	Email                string           `json:"email"`
	UsedPercent          *float64         `json:"used_percent"`
	ResetAt              *int64           `json:"reset_at"`
	PlanType             string           `json:"plan_type"`
	CreditBalance        string           `json:"credit_balance"`
	Unlimited            bool             `json:"unlimited"`
	Cards                []DeviMonCard    `json:"cards"`
	RenewAt              string           `json:"renew_at"`
	RenewSource          string           `json:"renew_source"`
	DevinDisabled        *bool            `json:"devin_disabled"`
	DevinRequests        *DeviMonRequests `json:"devin_requests,omitempty"`
	DevinRouting         *DeviMonRouting  `json:"devin_routing,omitempty"`
	Channel              string           `json:"channel"`
	Error                string           `json:"error"`
	ResetKey             string           `json:"reset_key,omitempty"`
	AutoReset            bool             `json:"auto_reset,omitempty"`
	AutoEnableAfterReset bool             `json:"auto_enable_after_reset"`
	verifiedWeekly       bool
	ResetOffer           *DeviMonResetOffer  `json:"reset_offer,omitempty"`
	ResetOutcome         DeviMonResetOutcome `json:"reset_outcome,omitempty"`
}

// DeviMonRouting holds CPA account overrides. A nil field means no override;
// a nil routing object means these settings were not collected.
type DeviMonRouting struct {
	Priority *int64 `json:"priority"`
	Weight   *int64 `json:"weight"`
}

// DeviMonDevin contains the configuration read from the Devin inventory.
type DeviMonDevin struct {
	name      string
	authIndex string
	Requests  *DeviMonRequests `json:"requests,omitempty"`
	Email     string           `json:"email"`
	Disabled  bool             `json:"disabled"`
	Routing   *DeviMonRouting  `json:"devin_routing,omitempty"`
}

// DeviMonSnapshot is the last completed collection.
type DeviMonSnapshot struct {
	StartedAtUS     int64            `json:"started_at_us,omitempty"`
	CollectedAt     int64            `json:"collected_at"`
	DurationMS      int64            `json:"duration_ms"`
	IntervalMinutes int              `json:"interval_minutes"`
	Accounts        []DeviMonAccount `json:"accounts"`
	UnpairedDevin   []DeviMonDevin   `json:"unpaired_devin"`
	Errors          []string         `json:"errors"`
}

// DeviMonKey returns the management key shared by both CPA instances.
func DeviMonKey() string {
	return strings.TrimSpace(common.GetEnvOrDefaultString("CPA_MANAGEMENT_KEY", ""))
}

type deviMonTask struct{}

func (deviMonTask) Type() string  { return model.SystemTaskTypeDevinMonitor }
func (deviMonTask) Enabled() bool { return DeviMonKey() != "" }
func (deviMonTask) Interval() time.Duration {
	return time.Duration(operation_setting.GetDeviMonSettings().IntervalMinutes) * time.Minute
}

type deviMonTaskPayload struct {
	AutoReset bool   `json:"auto_reset,omitempty"`
	RetryOf   string `json:"retry_of,omitempty"`
}

func (deviMonTask) NewPayload() any { return deviMonTaskPayload{AutoReset: true} }

func (deviMonTask) interruptedRetry(latest *model.SystemTask) (int64, any) {
	var payload deviMonTaskPayload
	if latest.Status != model.SystemTaskStatusFailed || latest.Error != model.SystemTaskLeaseExpiredMessage ||
		latest.DecodePayload(&payload) != nil || payload.RetryOf != "" {
		return 0, nil
	}
	payload.RetryOf = latest.TaskID
	return latest.UpdatedAt + int64(deviMonInterruptedRetryDelay/time.Second), payload
}

func init() { RegisterSystemTaskHandler(deviMonTask{}) }

func (deviMonTask) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	var payload deviMonTaskPayload
	if err := task.DecodePayload(&payload); err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	snapshot, err := collectDeviMon(ctx, task, payload.AutoReset)
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, snapshot, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
		return
	}
	if err := model.DeleteDeviMonRefreshesBefore(snapshot.StartedAtUS); err != nil {
		logger.LogWarn(ctx, common.LogText("devin monitor: could not prune old account refreshes: %v", err))
	}
}

type DeviMonState struct {
	IntervalMinutes int              `json:"interval_minutes"`
	Snapshot        *DeviMonSnapshot `json:"snapshot"`
	Collecting      bool             `json:"collecting"`
	NextRunAt       int64            `json:"next_run_at"`
	Configured      bool             `json:"configured"`
	LastError       string           `json:"last_error,omitempty"`
}

// The primary writes task results; every gateway reads them. The scheduler
// and this view use the same recovery time for an interrupted collection.
func GetDeviMonState() (DeviMonState, error) {
	handler := deviMonTask{}
	state := DeviMonState{Configured: handler.Enabled(), IntervalMinutes: operation_setting.GetDeviMonSettings().IntervalMinutes}
	latest, err := model.GetLatestSystemTask(handler.Type())
	if err != nil || latest == nil {
		return state, err
	}
	state.Collecting = latest.Status == model.SystemTaskStatusPending || latest.Status == model.SystemTaskStatusRunning
	if state.Configured && !state.Collecting {
		state.NextRunAt = latest.UpdatedAt + int64(state.IntervalMinutes)*60
		if retryAt, payload := handler.interruptedRetry(latest); payload != nil {
			state.NextRunAt = retryAt
		}
	}
	state.LastError = latest.Error
	if latest.Status != model.SystemTaskStatusSucceeded {
		latest, err = model.GetLatestSuccessfulDeviMonTask(latest.ID)
		if err != nil || latest == nil {
			return state, err
		}
	}
	if err := common.UnmarshalJsonStr(latest.Result, &state.Snapshot); err != nil || state.Snapshot == nil {
		return state, err
	}
	if err := applyDeviMonRefreshes(state.Snapshot); err != nil {
		return state, err
	}
	policies, err := model.GetDeviMonResetPolicies(context.Background())
	if err != nil {
		return state, err
	}
	now := common.GetTimestamp()
	for i := range state.Snapshot.Accounts {
		account := &state.Snapshot.Accounts[i]
		policy := policies[account.ResetKey]
		account.AutoReset = policy.Enabled
		account.AutoEnableAfterReset = account.ResetKey != "" && !policy.KeepDisabledAfterReset
		if !policy.Enabled || account.ResetAt == nil || *account.ResetAt <= max(policy.AttemptedWindowEnd, now) {
			account.ResetOffer = nil
		}
		if policy.HoldExhaustedUsage && account.exhaustedInOldWindow(policy.AttemptedWindowEnd) {
			account.UsedPercent, account.ResetAt = nil, nil
			account.ResetOutcome = deviMonResetUnverified
		}
	}
	return state, nil
}

// collectDeviMon collects every Codex account once. Accounts are queried one at a
// time with a random gap so the upstream never sees concurrent probes.
func collectDeviMon(ctx context.Context, task *model.SystemTask, autoUseCard bool) (*DeviMonSnapshot, error) {
	key := DeviMonKey()
	if key == "" {
		return nil, errors.New("CPA_MANAGEMENT_KEY is not configured")
	}
	started := time.Now()
	snapshot := &DeviMonSnapshot{
		StartedAtUS:     started.UnixMicro(),
		IntervalMinutes: operation_setting.GetDeviMonSettings().IntervalMinutes,
		Accounts:        []DeviMonAccount{},
		UnpairedDevin:   []DeviMonDevin{},
		Errors:          []string{},
	}

	codexAccounts, devinByEmail, err := deviMonInventory(ctx, key)
	if err != nil {
		return nil, err
	}
	// Pair from both complete credential lists, independently of quota probes.
	paired := make(map[string]int, len(codexAccounts))
	for _, file := range codexAccounts {
		paired[strings.ToLower(strings.TrimSpace(deviMonString(file["email"])))]++
	}
	for email, devin := range devinByEmail {
		if paired[email] == 0 {
			snapshot.UnpairedDevin = append(snapshot.UnpairedDevin, devin)
		}
	}
	policies, err := model.GetDeviMonResetPolicies(ctx)
	if err != nil {
		return nil, err
	}
	for i, file := range codexAccounts {
		if i > 0 {
			if gap := deviMonNextGap(); gap > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(gap):
				}
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		account, candidate := collectDeviMonAccount(ctx, key, file, devinByEmail)
		if paired[strings.ToLower(strings.TrimSpace(account.Email))] != 1 {
			account.ResetKey = ""
		}
		devin := devinByEmail[strings.ToLower(account.Email)]
		if policy := policies[account.ResetKey]; policy.Enabled && candidate != nil && policy.AttemptedWindowEnd < candidate.WindowEnd {
			if autoUseCard && task != nil {
				if _, err := redeemDeviMonCard(ctx, key, file, &account, candidate, devin, task); err != nil {
					logger.LogWarn(ctx, common.LogText("devin monitor: could not record reset for %s: %v", account.Email, err))
				}
			} else {
				account.ResetOffer = &candidate.DeviMonResetOffer
			}
		}
		if account.ResetOutcome == "" {
			if err := reconcileDeviMonAvailability(ctx, key, &account, devin, policies[account.ResetKey], task); err != nil {
				snapshot.Errors = append(snapshot.Errors, account.Email+": "+errDeviMonDevinStatusFailed.Error())
			}
		}
		snapshot.Accounts = append(snapshot.Accounts, account)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.Slice(snapshot.UnpairedDevin, func(i, j int) bool {
		return snapshot.UnpairedDevin[i].Email < snapshot.UnpairedDevin[j].Email
	})
	sortDeviMonAccounts(snapshot.Accounts)

	snapshot.DurationMS = time.Since(started).Milliseconds()
	snapshot.CollectedAt = time.Now().Unix()
	return snapshot, nil
}

func deviMonInventory(ctx context.Context, key string) ([]map[string]any, map[string]DeviMonDevin, error) {
	codexFiles, err := deviMonAuthFiles(ctx, key, deviMonCodexBase)
	if err != nil {
		return nil, nil, fmt.Errorf("cpa-pro-2: %w", err)
	}
	devinFiles, err := deviMonAuthFiles(ctx, key, deviMonDevinBase)
	if err != nil {
		return nil, nil, fmt.Errorf("cpa-pro-1: %w", err)
	}

	// Accounts stored on the gateway join the CPA inventory and are read
	// through their own channel; a CPA credential with the same email is
	// shadowed so the switch never shows the same mailbox twice.
	owned, err := model.GetDeviMonChatGPTAccounts(ctx)
	if err != nil {
		return nil, nil, err
	}
	codexAccounts := deviMonMergeOwnedAccounts(filterDeviMonProvider(codexFiles, "codex"), owned)
	return codexAccounts, deviMonDevinInventory(devinFiles), nil
}

// deviMonNextGap returns the pause before the next account, in [min, max).
// A non-positive span makes the pause constant instead of panicking.
func deviMonNextGap() time.Duration {
	span := deviMonAccountGapMax - deviMonAccountGapMin
	if span <= 0 {
		return max(deviMonAccountGapMin, 0)
	}
	return deviMonAccountGapMin + time.Duration(rand.Int64N(int64(span)))
}

// sortDeviMonAccounts orders by weekly reset time, earliest first; accounts
// without a reset time come last. Email breaks ties across inventory refreshes.
func sortDeviMonAccounts(accounts []DeviMonAccount) {
	sort.SliceStable(accounts, func(i, j int) bool {
		left, right := accounts[i].ResetAt, accounts[j].ResetAt
		if left != nil && right != nil && *left != *right {
			return *left < *right
		}
		if (left == nil) != (right == nil) {
			return left != nil
		}
		return accounts[i].Email < accounts[j].Email
	})
}

func collectDeviMonAccount(ctx context.Context, key string, file map[string]any, devinByEmail map[string]DeviMonDevin) (DeviMonAccount, *deviMonResetCandidate) {
	email := strings.TrimSpace(deviMonString(file["email"]))
	accountID, accountKey := deviMonCodexIdentity(file)
	account := DeviMonAccount{
		Email:        email,
		ResetKey:     accountKey,
		ObservedAtUS: time.Now().UnixMicro(),
		Channel:      deviMonFileChannel(file),
	}
	if devin, ok := devinByEmail[strings.ToLower(email)]; ok {
		account.DevinDisabled = &devin.Disabled
		account.DevinRouting = devin.Routing
		account.DevinRequests = devin.Requests
	}
	candidate := readDeviMonAllowance(ctx, key, file, accountID, &account, false)
	if deviMonBool(file["disabled"]) {
		candidate = nil
	}
	var renewalErr error
	account.RenewAt, account.RenewSource, renewalErr = deviMonRenewal(ctx, key, file)
	if renewalErr != nil && account.Error == "" {
		account.Error = "subscription: " + renewalErr.Error()
	}
	return account, candidate
}

// DeviMonFileChannel reports which channel reads this credential entry.
func deviMonFileChannel(file map[string]any) string {
	if deviMonString(file["channel"]) == model.DeviMonChannelChatGPT {
		return model.DeviMonChannelChatGPT
	}
	return model.DeviMonChannelCPA
}

// deviMonMergeOwnedAccounts appends the gateway-stored sign-ins to the CPA
// inventory, dropping a CPA credential whose mailbox is served by the switch.
func deviMonMergeOwnedAccounts(codexAccounts []map[string]any, owned map[string]model.DeviMonChatGPTAccount) []map[string]any {
	if len(owned) == 0 {
		return codexAccounts
	}
	merged := make([]map[string]any, 0, len(codexAccounts)+len(owned))
	for _, file := range codexAccounts {
		email := strings.ToLower(strings.TrimSpace(deviMonString(file["email"])))
		if row, ok := owned[email]; ok && row.Channel == model.DeviMonChannelChatGPT {
			continue
		}
		merged = append(merged, file)
	}
	for _, row := range owned {
		if row.Channel != model.DeviMonChannelChatGPT {
			continue
		}
		merged = append(merged, deviMonChatGPTFile(row))
	}
	// A stable order keeps snapshots diffable across runs.
	sort.SliceStable(merged, func(i, j int) bool {
		return deviMonString(merged[i]["email"]) < deviMonString(merged[j]["email"])
	})
	return merged
}

func readDeviMonAllowance(ctx context.Context, key string, file map[string]any, accountID string, account *DeviMonAccount, verifyReset bool) *deviMonResetCandidate {
	account.UsedPercent, account.ResetAt = nil, nil
	account.Cards = []DeviMonCard{}
	account.verifiedWeekly = false
	account.Error = ""
	var headers map[string]string
	if accountID != "" {
		headers = map[string]string{"ChatGPT-Account-Id": accountID}
	}
	usageCtx := ctx
	if verifyReset {
		var cancel context.CancelFunc
		usageCtx, cancel = context.WithTimeout(ctx, deviMonRequestTimeout/2)
		defer cancel()
	}
	usage, usageErr := deviMonCredentialCall(usageCtx, key, file, deviMonUsageURL, headers)
	weekly := false
	if usageErr != nil {
		account.Error = usageErr.Error()
	} else if reportedID := deviMonString(usage["account_id"]); accountID != "" && reportedID != "" && reportedID != accountID {
		account.Error = "Codex usage account mismatch"
	} else {
		weekly = applyDeviMonUsage(account, usage)
		account.verifiedWeekly = weekly
		if verifyReset && (!weekly || account.ResetAt == nil || *account.ResetAt <= 0) {
			account.UsedPercent, account.ResetAt = nil, nil
		}
	}
	cards, err := deviMonCredentialCall(ctx, key, file, deviMonCardsURL, headers)
	if err != nil {
		if account.Error == "" {
			account.Error = "reset cards: " + err.Error()
		}
		return nil
	}
	list, _ := cards["credits"].([]any)
	var earliest time.Time
	var creditID, expiry string
	now := time.Now()
	canReset := !verifyReset && weekly && account.ResetKey != "" && account.UsedPercent != nil &&
		*account.UsedPercent >= 100 && account.ResetAt != nil && *account.ResetAt > now.Unix()
	for _, entry := range list {
		card, ok := entry.(map[string]any)
		if !ok || deviMonString(card["status"]) != "available" {
			continue
		}
		expiresAt := deviMonString(card["expires_at"])
		account.Cards = append(account.Cards, DeviMonCard{ExpiresAt: expiresAt})
		if !canReset {
			continue
		}
		id := deviMonString(card["id"])
		when, err := time.Parse(time.RFC3339, expiresAt)
		if id != "" && err == nil && when.After(now) &&
			(earliest.IsZero() || when.Before(earliest) || (when.Equal(earliest) && id < creditID)) {
			earliest, creditID, expiry = when, id, expiresAt
		}
	}
	if creditID == "" {
		return nil
	}
	return deviMonResetCandidateFor(*account, accountID, creditID, expiry)
}

func applyDeviMonUsage(account *DeviMonAccount, usage map[string]any) bool {
	account.PlanType = deviMonString(usage["plan_type"])
	credits, _ := usage["credits"].(map[string]any)
	account.CreditBalance = deviMonString(credits["balance"])
	account.Unlimited = deviMonBool(credits["unlimited"])

	rateLimit, _ := usage["rate_limit"].(map[string]any)
	primary, _ := rateLimit["primary_window"].(map[string]any)
	secondary, _ := rateLimit["secondary_window"].(map[string]any)
	window := primary
	primarySeconds, primaryKnown := deviMonInt(primary["limit_window_seconds"])
	secondarySeconds, _ := deviMonInt(secondary["limit_window_seconds"])
	weekly := primarySeconds == deviMonWeeklySeconds || secondarySeconds == deviMonWeeklySeconds
	if primarySeconds != deviMonWeeklySeconds && secondarySeconds == deviMonWeeklySeconds {
		window = secondary
	} else if primaryKnown && !weekly {
		return false
	}
	if used, ok := deviMonFloat(window["used_percent"]); ok {
		account.UsedPercent = &used
	}
	if resetAt, ok := deviMonInt(window["reset_at"]); ok {
		account.ResetAt = &resetAt
	}
	return weekly
}

// deviMonRenewal prefers the live subscription endpoint because the id_token
// claim is only a snapshot taken at sign-in and never reflects a renewal.
func deviMonRenewal(ctx context.Context, key string, file map[string]any) (string, string, error) {
	var requestErr error
	idToken, _ := file["id_token"].(map[string]any)
	accountID := deviMonString(idToken["chatgpt_account_id"])
	if accountID != "" {
		body, err := deviMonCredentialCall(ctx, key, file,
			deviMonSubscribeURL+"?account_id="+accountID, map[string]string{"Chatgpt-Account-Id": accountID})
		if err == nil {
			if until := deviMonString(body["active_until"]); until != "" {
				return until, "live", nil
			}
		}
		requestErr = err
	}
	if snapshot := deviMonString(idToken["chatgpt_subscription_active_until"]); snapshot != "" {
		return snapshot, "id_token", nil
	}
	return "", "", requestErr
}

func deviMonAuthFiles(ctx context.Context, key string, base string) ([]map[string]any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v0/management/auth-files", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	payload, err := deviMonDo(request)
	if err != nil {
		return nil, err
	}
	files, ok := payload["files"].([]any)
	if !ok {
		return nil, errors.New("unexpected credential list")
	}
	result := make([]map[string]any, 0, len(files))
	for _, entry := range files {
		if file, ok := entry.(map[string]any); ok {
			result = append(result, file)
		}
	}
	return result, nil
}

// deviMonAPICall asks CPA to run one upstream request with a credential token.
func deviMonAPICall(ctx context.Context, key string, base string, authIndex string, target string, extra map[string]string) (map[string]any, error) {
	return deviMonProxyCall(ctx, key, base, authIndex, http.MethodGet, target, extra, nil)
}

// deviMonCredentialCall routes one read through the channel that owns the
// credential: CPA proxies it, or the gateway calls ChatGPT itself.
func deviMonCredentialCall(ctx context.Context, key string, file map[string]any, target string, extra map[string]string) (map[string]any, error) {
	return deviMonCredentialRequest(ctx, key, file, http.MethodGet, target, extra, nil)
}

// deviMonCredentialRequest is the channel dispatch behind every monitor
// request, including the reset-card consume POST.
func deviMonCredentialRequest(ctx context.Context, key string, file map[string]any, method, target string, extra map[string]string, data []byte) (map[string]any, error) {
	if deviMonFileChannel(file) != model.DeviMonChannelChatGPT {
		return deviMonProxyCall(ctx, key, deviMonCodexBase, deviMonString(file["auth_index"]), method, target, extra, data)
	}
	row, err := model.GetDeviMonChatGPTAccount(ctx, deviMonString(file["email"]))
	if err != nil {
		return nil, err
	}
	client, err := deviMonChatGPTClient(DeviMonProxyURL(row))
	if err != nil {
		return nil, err
	}
	accessToken, err := deviMonSessionAccess(ctx, row)
	if err != nil {
		return nil, err
	}
	header := deviMonChatGPTHeaders(accessToken, row.AccountID)
	for name, value := range extra {
		header[name] = value
	}
	payload, _, err := deviMonChatGPTCall(ctx, client, method, target, data, header)
	return payload, err
}

func deviMonProxyCall(ctx context.Context, key, base, authIndex, method, target string, extra map[string]string, data []byte) (map[string]any, error) {
	header := map[string]string{"Authorization": "Bearer $TOKEN$"}
	for name, value := range extra {
		header[name] = value
	}
	call := map[string]any{"auth_index": authIndex, "method": method, "url": target, "header": header}
	if data != nil {
		header["Content-Type"] = "application/json"
		call["data"] = string(data)
	}
	body, err := common.Marshal(call)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v0/management/api-call", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	payload, err := deviMonDo(request)
	if err != nil {
		return nil, err
	}
	if status, ok := deviMonInt(payload["status_code"]); !ok || status != http.StatusOK {
		return nil, errors.New("upstream returned status " + strconv.FormatInt(deviMonIntOr(payload["status_code"]), 10))
	}
	text := deviMonString(payload["body"])
	var result map[string]any
	if err := common.UnmarshalJsonStr(text, &result); err != nil {
		return nil, errors.New("upstream returned an unreadable body")
	}
	return result, nil
}

func deviMonDo(request *http.Request) (map[string]any, error) {
	client := http.Client{Timeout: deviMonRequestTimeout}
	if shared := GetHttpClient(); shared != nil {
		client = *shared
		client.Timeout = deviMonRequestTimeout
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		return nil, errors.New("request failed")
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			logger.LogWarn(context.Background(), common.LogText("devin monitor: close response body failed: %v", err))
		}
	}()
	description := "HTTP " + strconv.Itoa(response.StatusCode)
	if response.StatusCode != http.StatusOK {
		return nil, errors.New(description)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, deviMonMaxResponseSize))
	if err != nil {
		return nil, errors.New("read response failed")
	}
	var payload map[string]any
	if err := common.Unmarshal(raw, &payload); err != nil {
		return nil, errors.New("unreadable response")
	}
	return payload, nil
}

func filterDeviMonProvider(files []map[string]any, provider string) []map[string]any {
	result := make([]map[string]any, 0, len(files))
	for _, file := range files {
		if deviMonString(file["provider"]) == provider {
			result = append(result, file)
		}
	}
	return result
}

func deviMonString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func deviMonBool(value any) bool {
	flag, _ := value.(bool)
	return flag
}

func deviMonFloat(value any) (float64, bool) {
	number, ok := value.(float64)
	return number, ok
}

func deviMonInt(value any) (int64, bool) {
	number, ok := value.(float64)
	return int64(number), ok
}

func deviMonOptionalInt(value any) *int64 {
	n, ok := deviMonInt(value)
	if !ok {
		return nil
	}
	return &n
}

func deviMonIntOr(value any) int64 {
	number, _ := deviMonInt(value)
	return number
}
