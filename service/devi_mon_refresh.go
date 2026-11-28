package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var ErrDeviMonAccountMissing = errors.New("account no longer in the monitor inventory")

type deviMonReading struct {
	Account  *DeviMonAccount `json:"account,omitempty"`
	Unpaired *DeviMonDevin   `json:"unpaired,omitempty"`
	// Devin is a cpa-pro-1 credential flip. It carries no quota, so applying it
	// never replaces the collection's reading of the Codex side.
	Devin *DeviMonDevin `json:"devin,omitempty"`
}

// Only inventory discovery is shared with the full job. Never wait for other
// accounts or allow the client to supply an upstream URL or credential index.
func RefreshDeviMonAccount(ctx context.Context, email string) error {
	key := DeviMonKey()
	if key == "" {
		return errors.New("CPA_MANAGEMENT_KEY is not configured")
	}
	ctx, cancel := context.WithTimeout(ctx, deviMonRequestTimeout)
	defer cancel()
	started := time.Now().UnixMicro()
	email = strings.ToLower(strings.TrimSpace(email))
	files, devins, err := deviMonInventory(ctx, key)
	if err != nil {
		return err
	}
	reading := deviMonReading{}
	for _, file := range files {
		if !strings.EqualFold(deviMonString(file["email"]), email) {
			continue
		}
		if deviMonString(file["auth_index"]) == "" && deviMonFileChannel(file) != model.DeviMonChannelChatGPT {
			return errors.New("credential has no auth index")
		}
		account, candidate := collectDeviMonAccount(ctx, key, file, devins)
		if uniqueDeviMonCodexAccount(files, email) == nil {
			account.ResetKey, candidate = "", nil
		}
		if account.Error != "" {
			return errors.New(account.Error)
		}
		if account.ResetKey != "" {
			policies, err := model.GetDeviMonResetPolicies(ctx)
			if err != nil {
				return err
			}
			policy := policies[account.ResetKey]
			account.setQuotaPolicy(policy)
			if candidate != nil && account.AutoReset && policy.AttemptedWindowEnd < candidate.WindowEnd {
				account.ResetOffer = &candidate.DeviMonResetOffer
			}
			if err := reconcileDeviMonAvailability(ctx, key, &account, devins[email], policy, nil); err != nil {
				return err
			}
		}
		started = account.ObservedAtUS
		reading.Account = &account
		break
	}
	if reading.Account == nil {
		devin, ok := devins[email]
		if !ok {
			return ErrDeviMonAccountMissing
		}
		reading.Unpaired = &devin
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return saveDeviMonReading(ctx, email, started, reading)
}

func saveDeviMonReading(ctx context.Context, email string, started int64, reading deviMonReading) error {
	raw, err := common.Marshal(reading)
	if err != nil {
		return err
	}
	return model.SaveDeviMonAccountRefresh(ctx, &model.DeviMonAccountRefresh{Email: strings.ToLower(strings.TrimSpace(email)), StartedAtUS: started, Data: string(raw)})
}

// Only update members of the last complete inventory. A removed credential
// must not reappear just because it has an old manual reading on another node.
func applyDeviMonRefreshes(snapshot *DeviMonSnapshot) error {
	since := snapshot.StartedAtUS
	if since == 0 {
		since = snapshot.CollectedAt * 1_000_000
	}
	rows, err := model.GetDeviMonRefreshesSince(since)
	if err != nil || len(rows) == 0 {
		return err
	}
	byEmail := make(map[string]model.DeviMonAccountRefresh, len(rows))
	for _, row := range rows {
		byEmail[row.Email] = row
	}
	// A quota reading only wins against the account it re-read. A Devin-only
	// flip says nothing about quota, so it only has to be newer than the
	// inventory the collection read at its start; comparing it against the
	// account's own read time would silently drop a flip that landed mid-run.
	load := func(email string) (*model.DeviMonAccountRefresh, *deviMonReading, error) {
		saved, ok := byEmail[strings.ToLower(email)]
		if !ok {
			return nil, nil, nil
		}
		var reading deviMonReading
		if err := common.UnmarshalJsonStr(saved.Data, &reading); err != nil {
			return nil, nil, err
		}
		return &saved, &reading, nil
	}
	accounts := make([]DeviMonAccount, 0, len(snapshot.Accounts))
	unpaired := make([]DeviMonDevin, 0, len(snapshot.UnpairedDevin))
	for _, acc := range snapshot.Accounts {
		saved, reading, err := load(acc.Email)
		if err != nil {
			return err
		}
		if saved == nil {
			accounts = append(accounts, acc)
			continue
		}
		newer := saved.StartedAtUS >= acc.ObservedAtUS
		switch {
		case reading.Account != nil && newer:
			accounts = append(accounts, *reading.Account)
		case reading.Unpaired != nil && saved.StartedAtUS >= since:
			unpaired = append(unpaired, *reading.Unpaired)
		case reading.Devin != nil && saved.StartedAtUS >= since:
			acc.DevinDisabled = &reading.Devin.Disabled
			acc.DevinRouting = reading.Devin.Routing
			accounts = append(accounts, acc)
		case reading.Account != nil || reading.Unpaired != nil || reading.Devin != nil:
			accounts = append(accounts, acc)
		default:
			return errors.New("empty Devin monitor account reading")
		}
	}
	for _, devin := range snapshot.UnpairedDevin {
		saved, reading, err := load(devin.Email)
		if err != nil {
			return err
		}
		switch {
		case saved == nil || saved.StartedAtUS < since:
			unpaired = append(unpaired, devin)
		case reading.Account != nil:
			accounts = append(accounts, *reading.Account)
		case reading.Unpaired != nil:
			unpaired = append(unpaired, *reading.Unpaired)
		case reading.Devin != nil:
			devin.Disabled = reading.Devin.Disabled
			devin.Routing = reading.Devin.Routing
			unpaired = append(unpaired, devin)
		default:
			return errors.New("empty Devin monitor account reading")
		}
	}
	snapshot.Accounts, snapshot.UnpairedDevin = accounts, unpaired
	return nil
}
