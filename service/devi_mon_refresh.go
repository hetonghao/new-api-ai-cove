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
		if deviMonString(file["auth_index"]) == "" {
			return errors.New("credential has no auth index")
		}
		account := collectDeviMonAccount(ctx, key, file, devins)
		if account.Error != "" {
			return errors.New(account.Error)
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
	raw, err := common.Marshal(reading)
	if err != nil {
		return err
	}
	return model.SaveDeviMonAccountRefresh(ctx, &model.DeviMonAccountRefresh{Email: email, StartedAtUS: started, Data: string(raw)})
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
	accounts := make([]DeviMonAccount, 0, len(snapshot.Accounts))
	unpaired := make([]DeviMonDevin, 0, len(snapshot.UnpairedDevin))
	apply := func(email string, observed int64) (bool, error) {
		saved, ok := byEmail[strings.ToLower(email)]
		if !ok || saved.StartedAtUS < observed {
			return false, nil
		}
		var reading deviMonReading
		if err := common.UnmarshalJsonStr(saved.Data, &reading); err != nil {
			return false, err
		}
		switch {
		case reading.Account != nil:
			accounts = append(accounts, *reading.Account)
		case reading.Unpaired != nil:
			unpaired = append(unpaired, *reading.Unpaired)
		default:
			return false, errors.New("empty Devin monitor account reading")
		}
		return true, nil
	}
	for _, acc := range snapshot.Accounts {
		updated, err := apply(acc.Email, acc.ObservedAtUS)
		if err != nil {
			return err
		}
		if !updated {
			accounts = append(accounts, acc)
		}
	}
	for _, devin := range snapshot.UnpairedDevin {
		updated, err := apply(devin.Email, since)
		if err != nil {
			return err
		}
		if !updated {
			unpaired = append(unpaired, devin)
		}
	}
	snapshot.Accounts, snapshot.UnpairedDevin = accounts, unpaired
	return nil
}
