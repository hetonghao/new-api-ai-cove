package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/google/uuid"
)

const deviMonWeeklySeconds = 7 * 24 * 60 * 60

var ErrDeviMonResetOfferChanged = errors.New("reset offer changed")

type DeviMonResetOffer struct {
	ID        string `json:"id"`
	ExpiresAt string `json:"expires_at"`
}

type DeviMonResetOutcome string

const (
	deviMonResetApplied    DeviMonResetOutcome = "applied"
	deviMonResetRejected   DeviMonResetOutcome = "rejected"
	deviMonResetUnverified DeviMonResetOutcome = "unverified"
)

type deviMonResetCandidate struct {
	DeviMonResetOffer
	CreditID  string
	AccountID string
	WindowEnd int64
}

func deviMonCodexIdentity(file map[string]any) (accountID, accountKey string) {
	claims, _ := file["id_token"].(map[string]any)
	accountID = strings.TrimSpace(deviMonString(claims["chatgpt_account_id"]))
	email := strings.ToLower(strings.TrimSpace(deviMonString(file["email"])))
	if accountID == "" || email == "" || deviMonString(file["auth_index"]) == "" {
		return accountID, ""
	}
	digest := sha256.Sum256([]byte(email + "\x00" + accountID))
	return accountID, hex.EncodeToString(digest[:])
}

func SetDeviMonAccountAutoReset(ctx context.Context, email, expectedKey string, enabled bool) error {
	key := DeviMonKey()
	if key == "" {
		return errors.New("CPA_MANAGEMENT_KEY is not configured")
	}
	files, err := deviMonAuthFiles(ctx, key, deviMonCodexBase)
	if err != nil {
		return err
	}
	file := uniqueDeviMonCodexAccount(filterDeviMonProvider(files, "codex"), email)
	_, accountKey := deviMonCodexIdentity(file)
	if accountKey == "" || accountKey != expectedKey {
		return ErrDeviMonAccountMissing
	}
	return model.SetDeviMonResetPolicy(ctx, accountKey, enabled)
}

func uniqueDeviMonCodexAccount(files []map[string]any, email string) map[string]any {
	var match map[string]any
	for _, file := range files {
		if !strings.EqualFold(strings.TrimSpace(deviMonString(file["email"])), strings.TrimSpace(email)) {
			continue
		}
		if match != nil {
			return nil
		}
		match = file
	}
	return match
}

// ConfirmDeviMonAccountReset rechecks the same account, window and card that
// the administrator saw. A changed offer requires a new refresh and decision.
func ConfirmDeviMonAccountReset(ctx context.Context, email, offerID string) (*DeviMonAccount, error) {
	key := DeviMonKey()
	if key == "" {
		return nil, errors.New("CPA_MANAGEMENT_KEY is not configured")
	}
	checkCtx, cancel := context.WithTimeout(ctx, deviMonRequestTimeout)
	defer cancel()
	files, devins, err := deviMonInventory(checkCtx, key)
	if err != nil {
		return nil, err
	}
	file := uniqueDeviMonCodexAccount(files, email)
	if file == nil {
		return nil, ErrDeviMonResetOfferChanged
	}
	account, candidate := collectDeviMonAccount(checkCtx, key, file, devins)
	if candidate == nil || candidate.ID != offerID {
		return nil, ErrDeviMonResetOfferChanged
	}
	applied, err := redeemDeviMonCard(ctx, key, deviMonString(file["auth_index"]), &account, candidate, nil)
	if err != nil {
		return nil, err
	}
	if !applied {
		return nil, ErrDeviMonResetOfferChanged
	}
	return &account, nil
}

func redeemDeviMonCard(ctx context.Context, key, authIndex string, account *DeviMonAccount, candidate *deviMonResetCandidate, task *model.SystemTask) (bool, error) {
	now := time.Now()
	expiresAt, err := time.Parse(time.RFC3339, candidate.ExpiresAt)
	if err != nil || !expiresAt.After(now) || candidate.WindowEnd <= now.Unix() {
		return false, nil
	}
	payload, err := common.Marshal(map[string]string{
		"credit_id": candidate.CreditID, "redeem_request_id": candidate.ID,
	})
	if err != nil {
		return false, err
	}
	claimed, err := model.ClaimDeviMonReset(ctx, account.ResetKey, candidate.WindowEnd, now.Unix(), task)
	if err != nil || !claimed {
		return false, err
	}
	// Once claimed, finish the accepted operation even if the initiating request disconnects.
	operationCtx := context.WithoutCancel(ctx)
	headers := map[string]string{"ChatGPT-Account-Id": candidate.AccountID, "Accept": "application/json", "Originator": "codex_cli_rs"}
	result, requestErr := deviMonProxyCall(operationCtx, key, deviMonCodexBase, authIndex, http.MethodPost, deviMonCardsURL+"/consume", headers, payload)
	outcome := deviMonResetUnverified
	if requestErr == nil {
		switch deviMonString(result["code"]) {
		case "reset":
			outcome = deviMonResetApplied
		case "nothing_to_reset", "no_credit":
			outcome = deviMonResetRejected
		}
	}

	// A lost response or a disconnected browser doesn't mean the card wasn't used.
	verifyCtx, cancel := context.WithTimeout(operationCtx, deviMonRequestTimeout)
	defer cancel()
	account.ObservedAtUS = time.Now().UnixMicro()
	readDeviMonAllowance(verifyCtx, key, authIndex, candidate.AccountID, account, true)
	if outcome != deviMonResetRejected && account.exhaustedInOldWindow(candidate.WindowEnd) {
		// The upstream can briefly keep returning the pre-reset window.
		account.ObservedAtUS = time.Now().UnixMicro()
		readDeviMonAllowance(verifyCtx, key, authIndex, candidate.AccountID, account, true)
		if account.exhaustedInOldWindow(candidate.WindowEnd) {
			account.UsedPercent, account.ResetAt = nil, nil
		}
	}
	rejected := outcome == deviMonResetRejected
	if account.Error != "" || account.UsedPercent == nil {
		outcome = deviMonResetUnverified
	}
	account.ResetOffer = nil
	account.ResetOutcome = outcome
	logger.LogInfo(ctx, common.LogText("devin monitor: reset card for %s: %s", account.Email, outcome))
	saveCtx, cancelSave := context.WithTimeout(operationCtx, 5*time.Second)
	defer cancelSave()
	if err := saveDeviMonReading(saveCtx, account.Email, account.ObservedAtUS, deviMonReading{Account: account}); err != nil {
		return true, err
	}
	if rejected {
		return true, model.ReleaseDeviMonUsageHold(saveCtx, account.ResetKey, candidate.WindowEnd)
	}
	return true, nil
}

func (account *DeviMonAccount) exhaustedInOldWindow(windowEnd int64) bool {
	return account.UsedPercent != nil && *account.UsedPercent >= 100 && (account.ResetAt == nil || *account.ResetAt <= windowEnd)
}

func deviMonResetCandidateFor(account DeviMonAccount, accountID, cardID, expiresAt string) *deviMonResetCandidate {
	windowEnd := *account.ResetAt
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte("devin-reset/"+account.ResetKey+"/"+strconv.FormatInt(windowEnd, 10)+"/"+cardID)).String()
	return &deviMonResetCandidate{
		DeviMonResetOffer: DeviMonResetOffer{ID: id, ExpiresAt: expiresAt},
		CreditID:          cardID, AccountID: accountID, WindowEnd: windowEnd,
	}
}
