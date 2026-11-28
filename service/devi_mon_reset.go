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
	// A CPA credential is addressed by auth_index; a stored ChatGPT sign-in is
	// addressed by its email row, so it carries no auth_index.
	stored := deviMonFileChannel(file) == model.DeviMonChannelChatGPT
	if accountID == "" || email == "" || (!stored && deviMonString(file["auth_index"]) == "") {
		return accountID, ""
	}
	digest := sha256.Sum256([]byte(email + "\x00" + accountID))
	return accountID, hex.EncodeToString(digest[:])
}

func (account *DeviMonAccount) setQuotaPolicy(policy model.DeviMonResetPolicy) {
	account.ExhaustionAction = ""
	if account.ResetKey != "" {
		account.ExhaustionAction = policy.ExhaustionAction()
	}
	account.AutoReset = account.ExhaustionAction == model.DeviMonUseCardOnExhaustion
	account.AutoEnableAfterReset = account.ResetKey != "" && !policy.KeepDisabledAfterReset
}

func SetDeviMonAccountPolicy(ctx context.Context, email, expectedKey string, action model.DeviMonExhaustionAction, resume *bool) error {
	if (resume != nil && action != "") || (resume == nil && !action.Valid()) {
		return model.ErrDeviMonInvalidPolicy
	}
	key := DeviMonKey()
	if key == "" {
		return errors.New("CPA_MANAGEMENT_KEY is not configured")
	}
	files, _, err := deviMonInventory(ctx, key)
	if err != nil {
		return err
	}
	file := uniqueDeviMonCodexAccount(files, email)
	_, accountKey := deviMonCodexIdentity(file)
	if accountKey == "" || accountKey != expectedKey {
		return ErrDeviMonAccountMissing
	}
	if resume != nil {
		return model.SetDeviMonResumeAfterReset(ctx, accountKey, *resume)
	}
	return model.SetDeviMonResetPolicy(ctx, accountKey, action)
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
	applied, err := redeemDeviMonCard(ctx, key, file, &account, candidate, devins[strings.ToLower(account.Email)], nil)
	if err != nil {
		return nil, err
	}
	if !applied {
		return nil, ErrDeviMonResetOfferChanged
	}
	return &account, nil
}

func redeemDeviMonCard(ctx context.Context, key string, file map[string]any, account *DeviMonAccount, candidate *deviMonResetCandidate, devin DeviMonDevin, task *model.SystemTask) (bool, error) {
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
	result, requestErr := deviMonCredentialRequest(operationCtx, key, file, http.MethodPost, deviMonCardsURL+"/consume", headers, payload)
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
	readDeviMonAllowance(verifyCtx, key, file, candidate.AccountID, account, true)
	if outcome != deviMonResetRejected && account.exhaustedInOldWindow(candidate.WindowEnd) {
		// The upstream can briefly keep returning the pre-reset window.
		account.ObservedAtUS = time.Now().UnixMicro()
		readDeviMonAllowance(verifyCtx, key, file, candidate.AccountID, account, true)
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
	policies, policyErr := model.GetDeviMonResetPolicies(operationCtx)
	if policyErr == nil {
		policy := policies[account.ResetKey]
		account.setQuotaPolicy(policy)
		policyErr = reconcileDeviMonAvailability(operationCtx, key, account, devin, policy, task)
	}
	// Store the verified quota and its resulting Devin state together. A manual
	// overlay must not restore the pre-reset status after the full task finishes.
	saveCtx, cancelSave := context.WithTimeout(operationCtx, 5*time.Second)
	defer cancelSave()
	if err := saveDeviMonReading(saveCtx, account.Email, account.ObservedAtUS, deviMonReading{Account: account}); err != nil {
		return true, errors.Join(policyErr, err)
	}
	if rejected {
		return true, errors.Join(policyErr, model.ReleaseDeviMonUsageHold(saveCtx, account.ResetKey, candidate.WindowEnd))
	}
	return true, policyErr
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
