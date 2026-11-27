package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// Enabling or disabling is a Devin-side operation: it flips the credential
// inside cpa-pro-1, which is what the "Devin enabled" badge has always shown.
// It is independent of the reading channel: a ChatGPT session only decides where
// quota numbers come from, so both channels keep this switch.
var (
	ErrDeviMonDevinAccountMissing = errors.New("the account has no Devin credential in CPA")
	errDeviMonDevinStatusFailed   = errors.New("CPA refused the status change")
)

// SetDeviMonDevinDisabled mirrors the enable/disable toggle of cpa-pro-1. The
// credential file name is resolved from the same inventory the monitor reads,
// so no caller needs to know CPA's naming.
func SetDeviMonDevinDisabled(ctx context.Context, email string, disabled bool) error {
	key := DeviMonKey()
	if key == "" {
		return errors.New("CPA_MANAGEMENT_KEY is not configured")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	files, err := deviMonAuthFiles(ctx, key, deviMonDevinBase)
	if err != nil {
		return err
	}
	var captured map[string]any
	for _, file := range filterDeviMonProvider(files, "devin") {
		if !strings.EqualFold(strings.TrimSpace(deviMonString(file["email"])), email) {
			continue
		}
		if captured != nil {
			// Two credentials for one mailbox: refusing beats disabling the
			// wrong one.
			return errors.New("more than one Devin credential uses this email")
		}
		captured = file
	}
	name := ""
	if captured != nil {
		name = deviMonString(captured["name"])
	}
	if name == "" {
		return ErrDeviMonDevinAccountMissing
	}
	payload, err := common.Marshal(map[string]any{"name": name, "disabled": disabled})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPatch,
		deviMonDevinBase+"/v0/management/auth-files/status", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	request.Header.Set("Content-Type", "application/json")
	if _, err := deviMonDo(request); err != nil {
		return errDeviMonDevinStatusFailed
	}
	// Record the flip for the monitor. The reading carries the credential state
	// only: re-reading the Codex side here would spend a quota probe to learn
	// something this request already knows.
	return saveDeviMonReading(ctx, email, time.Now().UnixMicro(), deviMonReading{
		Devin: &DeviMonDevin{
			Email:    email,
			Disabled: disabled,
			Routing: &DeviMonRouting{
				Priority: deviMonOptionalInt(captured["priority"]),
				Weight:   deviMonOptionalInt(captured["weight"]),
			},
		},
	})
}
