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
	deviMonUsageURL     = "https://chatgpt.com/backend-api/wham/usage"
	deviMonCardsURL     = "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits"
	deviMonSubscribeURL = "https://chatgpt.com/backend-api/subscriptions"

	deviMonRequestTimeout  = 45 * time.Second
	deviMonMaxResponseSize = 1 << 20
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
	Email         string          `json:"email"`
	UsedPercent   *float64        `json:"used_percent"`
	ResetAt       *int64          `json:"reset_at"`
	PlanType      string          `json:"plan_type"`
	CreditBalance string          `json:"credit_balance"`
	Unlimited     bool            `json:"unlimited"`
	Cards         []DeviMonCard   `json:"cards"`
	RenewAt       string          `json:"renew_at"`
	RenewSource   string          `json:"renew_source"`
	DevinDisabled *bool           `json:"devin_disabled"`
	DevinRouting  *DeviMonRouting `json:"devin_routing,omitempty"`
	Error         string          `json:"error"`
}

// DeviMonRouting holds CPA account overrides. A nil field means no override;
// a nil routing object means these settings were not collected.
type DeviMonRouting struct {
	Priority *int64 `json:"priority"`
	Weight   *int64 `json:"weight"`
}

// DeviMonDevin contains the configuration read from the Devin inventory.
type DeviMonDevin struct {
	Email    string          `json:"email"`
	Disabled bool            `json:"disabled"`
	Routing  *DeviMonRouting `json:"devin_routing,omitempty"`
}

// DeviMonSnapshot is the last completed collection.
type DeviMonSnapshot struct {
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
func (deviMonTask) NewPayload() any { return nil }

func init() { RegisterSystemTaskHandler(deviMonTask{}) }

func (deviMonTask) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	snapshot, err := collectDeviMon(ctx)
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, snapshot, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
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
// counts the interval from completion, including failed attempts.
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
	}
	state.LastError = latest.Error
	if latest.Status != model.SystemTaskStatusSucceeded {
		latest, err = model.GetLatestSuccessfulDeviMonTask(latest.ID)
		if err != nil || latest == nil {
			return state, err
		}
	}
	err = common.UnmarshalJsonStr(latest.Result, &state.Snapshot)
	return state, err
}

// collectDeviMon collects every Codex account once. Accounts are queried one at a
// time with a random gap so the upstream never sees concurrent probes.
func collectDeviMon(ctx context.Context) (*DeviMonSnapshot, error) {
	key := DeviMonKey()
	if key == "" {
		return nil, errors.New("CPA_MANAGEMENT_KEY is not configured")
	}
	started := time.Now()
	snapshot := &DeviMonSnapshot{
		IntervalMinutes: operation_setting.GetDeviMonSettings().IntervalMinutes,
		Accounts:        []DeviMonAccount{},
		UnpairedDevin:   []DeviMonDevin{},
		Errors:          []string{},
	}

	codexFiles, err := deviMonAuthFiles(ctx, key, deviMonCodexBase)
	if err != nil {
		return nil, fmt.Errorf("cpa-pro-2: %w", err)
	}
	devinFiles, err := deviMonAuthFiles(ctx, key, deviMonDevinBase)
	if err != nil {
		return nil, fmt.Errorf("cpa-pro-1: %w", err)
	}

	codexAccounts := filterDeviMonProvider(codexFiles, "codex")
	devinByEmail := map[string]DeviMonDevin{}
	for _, file := range filterDeviMonProvider(devinFiles, "devin") {
		if email := deviMonString(file["email"]); email != "" {
			devinByEmail[strings.ToLower(email)] = DeviMonDevin{
				Email:    email,
				Disabled: deviMonBool(file["disabled"]),
				Routing: &DeviMonRouting{
					Priority: deviMonOptionalInt(file["priority"]),
					Weight:   deviMonOptionalInt(file["weight"]),
				},
			}
		}
	}

	// Pair from both complete credential lists, independently of quota probes.
	paired := make(map[string]bool, len(codexAccounts))
	for _, file := range codexAccounts {
		paired[strings.ToLower(deviMonString(file["email"]))] = true
	}
	for email, devin := range devinByEmail {
		if !paired[email] {
			snapshot.UnpairedDevin = append(snapshot.UnpairedDevin, devin)
		}
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
		snapshot.Accounts = append(snapshot.Accounts, collectDeviMonAccount(ctx, key, file, devinByEmail))
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

func collectDeviMonAccount(ctx context.Context, key string, file map[string]any, devinByEmail map[string]DeviMonDevin) DeviMonAccount {
	email := deviMonString(file["email"])
	account := DeviMonAccount{Email: email, Cards: []DeviMonCard{}}
	if devin, ok := devinByEmail[strings.ToLower(email)]; ok {
		account.DevinDisabled = &devin.Disabled
		account.DevinRouting = devin.Routing
	}

	usage, errUsage := deviMonAPICall(ctx, key, deviMonCodexBase, deviMonString(file["auth_index"]), deviMonUsageURL, nil)
	if errUsage != nil {
		account.Error = errUsage.Error()
		return account
	}
	if rateLimit, ok := usage["rate_limit"].(map[string]any); ok {
		if window, ok := rateLimit["primary_window"].(map[string]any); ok {
			if used, ok := deviMonFloat(window["used_percent"]); ok {
				account.UsedPercent = &used
			}
			if resetAt, ok := deviMonInt(window["reset_at"]); ok {
				account.ResetAt = &resetAt
			}
		}
	}
	account.PlanType = deviMonString(usage["plan_type"])
	if credits, ok := usage["credits"].(map[string]any); ok {
		account.CreditBalance = deviMonString(credits["balance"])
		account.Unlimited = deviMonBool(credits["unlimited"])
	}

	if cards, errCards := deviMonAPICall(ctx, key, deviMonCodexBase, deviMonString(file["auth_index"]), deviMonCardsURL, nil); errCards != nil {
		account.Error = "reset cards: " + errCards.Error()
	} else if list, ok := cards["credits"].([]any); ok {
		for _, entry := range list {
			card, ok := entry.(map[string]any)
			if !ok || deviMonString(card["status"]) != "available" {
				continue
			}
			account.Cards = append(account.Cards, DeviMonCard{ExpiresAt: deviMonString(card["expires_at"])})
		}
	}

	account.RenewAt, account.RenewSource = deviMonRenewal(ctx, key, file)
	return account
}

// deviMonRenewal prefers the live subscription endpoint because the id_token
// claim is only a snapshot taken at sign-in and never reflects a renewal.
func deviMonRenewal(ctx context.Context, key string, file map[string]any) (string, string) {
	idToken, _ := file["id_token"].(map[string]any)
	accountID := deviMonString(idToken["chatgpt_account_id"])
	if accountID != "" {
		body, err := deviMonAPICall(ctx, key, deviMonCodexBase, deviMonString(file["auth_index"]),
			deviMonSubscribeURL+"?account_id="+accountID, map[string]string{"Chatgpt-Account-Id": accountID})
		if err == nil {
			if until := deviMonString(body["active_until"]); until != "" {
				return until, "live"
			}
		}
	}
	if snapshot := deviMonString(idToken["chatgpt_subscription_active_until"]); snapshot != "" {
		return snapshot, "id_token"
	}
	return "", ""
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
	header := map[string]string{"Authorization": "Bearer $TOKEN$"}
	for name, value := range extra {
		header[name] = value
	}
	body, err := common.Marshal(map[string]any{
		"auth_index": authIndex,
		"method":     http.MethodGet,
		"url":        target,
		"header":     header,
	})
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
