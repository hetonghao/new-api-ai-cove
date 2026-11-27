package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var (
	ErrDeviMonDevinAccountMissing = errors.New("the account has no unique Devin credential in CPA")
	errDeviMonDevinStatusFailed   = errors.New("CPA refused the status change")
	ErrDeviMonInvalidRouting      = errors.New("Invalid Devin routing value")
)

type DeviMonRequestBucket struct {
	Success int64 `json:"success"`
	Failed  int64 `json:"failed"`
}

type DeviMonRequests struct {
	Success int64                  `json:"success"`
	Failed  int64                  `json:"failed"`
	Recent  []DeviMonRequestBucket `json:"recent"`
	AsOf    int64                  `json:"as_of"`
}

// Only expose display data; CPA's raw credential inventory stays on the server.
func deviMonDevinInventory(files []map[string]any) map[string]DeviMonDevin {
	result := make(map[string]DeviMonDevin)
	seen := make(map[string]bool)
	now := time.Now().Truncate(time.Minute).Unix()
	for _, file := range filterDeviMonProvider(files, "devin") {
		email := deviMonString(file["email"])
		id := strings.ToLower(email)
		if id == "" {
			continue
		}
		if seen[id] {
			delete(result, id)
			continue
		}
		seen[id] = true
		devin := DeviMonDevin{Email: email, name: deviMonString(file["name"]), authIndex: deviMonString(file["auth_index"]), Disabled: deviMonBool(file["disabled"]), Routing: &DeviMonRouting{Priority: deviMonOptionalInt(file["priority"]), Weight: deviMonOptionalInt(file["weight"])}}
		if buckets, ok := file["recent_requests"].([]any); ok {
			history := &DeviMonRequests{Success: deviMonIntOr(file["success"]), Failed: deviMonIntOr(file["failed"]), AsOf: now, Recent: []DeviMonRequestBucket{}}
			for _, value := range buckets[max(0, len(buckets)-20):] {
				b, _ := value.(map[string]any)
				history.Recent = append(history.Recent, DeviMonRequestBucket{Success: deviMonIntOr(b["success"]), Failed: deviMonIntOr(b["failed"])})
			}
			devin.Requests = history
		}
		result[id] = devin
	}
	return result
}

func ListDeviMonDevin(ctx context.Context) (map[string]DeviMonDevin, error) {
	key := DeviMonKey()
	if key == "" {
		return nil, errors.New("CPA_MANAGEMENT_KEY is not configured")
	}
	files, err := deviMonAuthFiles(ctx, key, deviMonDevinBase)
	if err != nil {
		return nil, err
	}
	return deviMonDevinInventory(files), nil
}

func resolveDeviMonDevin(ctx context.Context, email string) (DeviMonDevin, error) {
	devins, err := ListDeviMonDevin(ctx)
	if err != nil {
		return DeviMonDevin{}, err
	}
	devin := devins[strings.ToLower(strings.TrimSpace(email))]
	if devin.name == "" {
		return DeviMonDevin{}, ErrDeviMonDevinAccountMissing
	}
	return devin, nil
}

func patchDeviMonDevin(ctx context.Context, key, path string, devin DeviMonDevin, changes map[string]any) error {
	changes["name"] = devin.name
	if path == "status" && devin.authIndex != "" {
		changes["auth_index"] = devin.authIndex
	}
	payload, err := common.Marshal(changes)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, deviMonDevinBase+"/v0/management/auth-files/"+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	_, err = deviMonDo(req)
	if err == nil {
		invalidateDeviMonLiveInventory()
	}
	return err
}

func (d DeviMonDevin) pauseID() string {
	sum := sha256.Sum256([]byte(d.name + "\x00" + d.authIndex))
	return hex.EncodeToString(sum[:])
}

func SetDeviMonDevinDisabled(ctx context.Context, email string, disabled bool) error {
	devin, err := resolveDeviMonDevin(ctx, email)
	if err != nil {
		return err
	}
	// A deliberate status change relinquishes any outstanding automatic resume.
	if err = patchDeviMonDevin(ctx, DeviMonKey(), "status", devin, map[string]any{"disabled": disabled}); err != nil {
		return errDeviMonDevinStatusFailed
	}
	if err = model.ForgetDeviMonQuotaPause(ctx, devin.pauseID()); err != nil {
		return err
	}
	devin.Disabled = disabled
	return saveDeviMonReading(ctx, email, time.Now().UnixMicro(), deviMonReading{Devin: &devin})
}

func SetDeviMonRouting(ctx context.Context, email, field string, value int64) error {
	switch field {
	case "priority":
		if value < -2147483648 || value > 2147483647 {
			return ErrDeviMonInvalidRouting
		}
	case "weight":
		if value < 0 || value > 1_000_000 {
			return ErrDeviMonInvalidRouting
		}
	default:
		return ErrDeviMonInvalidRouting
	}
	devin, err := resolveDeviMonDevin(ctx, email)
	if err != nil {
		return err
	}
	return patchDeviMonDevin(ctx, DeviMonKey(), "fields", devin, map[string]any{field: value})
}

// A disabled account with remaining allowance is not evidence of a reset. Arm
// recovery on a verified exhausted week, including accounts already paused when
// monitoring starts. Manual switches cancel the outstanding recovery.
func reconcileDeviMonAvailability(ctx context.Context, key string, account *DeviMonAccount, devin DeviMonDevin, policy model.DeviMonResetPolicy, task *model.SystemTask) error {
	if account.ResetKey == "" || devin.name == "" || !account.verifiedWeekly || account.UsedPercent == nil || account.ResetAt == nil || *account.ResetAt <= time.Now().Unix() || *account.UsedPercent < 0 {
		return nil
	}
	exhausted := *account.UsedPercent >= 100
	id, end := devin.pauseID(), *account.ResetAt
	afterCard := account.ResetOutcome == deviMonResetApplied && !exhausted
	eligible := func(p model.DeviMonResetPolicy) bool {
		if exhausted {
			if p.HoldExhaustedUsage && end <= p.AttemptedWindowEnd {
				return false
			}
			if !devin.Disabled {
				return !p.Enabled
			}
			return !p.KeepDisabledAfterReset && (p.PausedCredential != id || p.PausedWindowEnd < end)
		}
		return !p.KeepDisabledAfterReset && ((p.PausedCredential == id && end > p.PausedWindowEnd) || (devin.Disabled && afterCard))
	}
	if !eligible(policy) {
		return nil
	}
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return model.UpdateDeviMonQuotaPause(op, account.ResetKey, task, func(p *model.DeviMonResetPolicy) error {
		if !eligible(*p) {
			return nil
		}
		current, err := resolveDeviMonDevin(op, account.Email)
		if err != nil {
			return err
		}
		if current.name != devin.name || current.authIndex != devin.authIndex {
			return ErrDeviMonDevinAccountMissing
		}
		if current.Disabled != exhausted {
			// Card mode never disables, even without an available card.
			if exhausted && p.Enabled {
				return nil
			}
			if err := patchDeviMonDevin(op, key, "status", current, map[string]any{"disabled": exhausted}); err != nil {
				return err
			}
		}
		if exhausted && !p.KeepDisabledAfterReset {
			p.PausedCredential, p.PausedWindowEnd = id, end
		} else {
			p.PausedCredential, p.PausedWindowEnd = "", 0
		}
		account.DevinDisabled = &exhausted
		return nil
	})
}
