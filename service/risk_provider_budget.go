package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
)

const (
	riskProviderNeuronsBudgetNamespace = "new-api:risk-neurons-budget:v1"
	riskProviderNeuronsResetDelay      = 5 * time.Minute
)

var (
	ErrRiskProviderDailyNeuronsExhausted = errors.New("risk provider daily Neurons limit exhausted")
	ErrRiskProviderBudgetUnavailable     = errors.New("risk provider Neurons budget unavailable")
	ErrRiskProviderDailyNeuronsResponse  = errors.New("risk provider reported daily Neurons exhaustion")
)

var reserveRiskProviderNeuronsScript = redis.NewScript(`
local current_window = redis.call("HGET", KEYS[1], "window")
if current_window ~= ARGV[1] then
  local exhausted = redis.call("HGET", KEYS[1], "exhausted")
  if current_window and exhausted == "1" and tonumber(ARGV[2]) < tonumber(ARGV[3]) then
    return -2
  end
  redis.call("HSET", KEYS[1], "window", ARGV[1], "used", "0", "reserved", "0", "exhausted", "0")
end
if redis.call("HGET", KEYS[1], "exhausted") == "1" then
  return 0
end
local used = tonumber(redis.call("HGET", KEYS[1], "used") or "0")
local reserved = tonumber(redis.call("HGET", KEYS[1], "reserved") or "0")
local estimate = tonumber(ARGV[4])
local limit = tonumber(ARGV[5])
if used + reserved + estimate > limit then
  redis.call("HSET", KEYS[1], "exhausted", "1")
  redis.call("EXPIRE", KEYS[1], 172800)
  return 0
end
redis.call("HINCRBY", KEYS[1], "reserved", estimate)
redis.call("EXPIRE", KEYS[1], 172800)
return 1
`)

var settleRiskProviderNeuronsScript = redis.NewScript(`
if redis.call("HGET", KEYS[1], "window") ~= ARGV[1] then
  return 0
end
local reserved = tonumber(redis.call("HGET", KEYS[1], "reserved") or "0")
local estimate = tonumber(ARGV[2])
local actual = tonumber(ARGV[3])
local used = tonumber(redis.call("HGET", KEYS[1], "used") or "0")
local limit = tonumber(ARGV[4])
reserved = math.max(0, reserved - estimate)
local available = math.max(0, limit - used - reserved)
local applied = math.min(actual, available)
redis.call("HSET", KEYS[1], "reserved", reserved, "used", used + applied)
if actual > applied or used + applied + reserved >= limit then
	redis.call("HSET", KEYS[1], "exhausted", "1")
end
redis.call("EXPIRE", KEYS[1], 172800)
if actual > applied then
	return 2
end
return 1
`)

var exhaustRiskProviderNeuronsScript = redis.NewScript(`
if redis.call("HGET", KEYS[1], "window") ~= ARGV[1] then
  return 0
end
local reserved = tonumber(redis.call("HGET", KEYS[1], "reserved") or "0")
local estimate = tonumber(ARGV[2])
redis.call("HSET", KEYS[1], "reserved", math.max(0, reserved - estimate), "exhausted", "1")
redis.call("EXPIRE", KEYS[1], 172800)
return 1
`)

type riskProviderNeuronsReservation struct {
	Key       string
	Window    string
	Estimated int64
}

type riskProviderNeuronsBudgetState struct {
	Window    string
	Used      int64
	Reserved  int64
	Exhausted bool
	ReadyAt   time.Time
}

type riskProviderNeuronsBudget struct {
	now func() time.Time
}

func newRiskProviderNeuronsBudget(now func() time.Time) *riskProviderNeuronsBudget {
	if now == nil {
		now = time.Now
	}
	return &riskProviderNeuronsBudget{now: now}
}

func (b *riskProviderNeuronsBudget) Reserve(ctx context.Context, provider *model.RiskProvider, estimate int64) (riskProviderNeuronsReservation, error) {
	if b == nil || provider == nil || provider.ProviderType != model.RiskProviderCloudflare || provider.Id < 1 || estimate < 1 {
		return riskProviderNeuronsReservation{}, ErrRiskProviderBudgetUnavailable
	}
	limit := provider.DailyNeuronsLimit
	if limit < 1 {
		limit = model.DefaultRiskProviderDailyNeuronsLimit
	}
	now := b.now().UTC()
	_, window, readyAt, err := riskProviderNeuronsWindow(now, provider.DailyResetTime)
	if err != nil {
		return riskProviderNeuronsReservation{}, err
	}
	reservation := riskProviderNeuronsReservation{
		Key:       fmt.Sprintf("%s:%d", riskProviderNeuronsBudgetNamespace, provider.Id),
		Window:    window,
		Estimated: estimate,
	}
	if !common.RedisEnabled || common.RDB == nil {
		return riskProviderNeuronsReservation{}, ErrRiskProviderBudgetUnavailable
	}
	result, runErr := reserveRiskProviderNeuronsScript.Run(ctx, common.RDB, []string{reservation.Key},
		window, now.Unix(), readyAt.Unix(), estimate, limit).Int64()
	if runErr != nil {
		return riskProviderNeuronsReservation{}, fmt.Errorf("%w: reserve risk provider Neurons: %v", ErrRiskProviderBudgetUnavailable, runErr)
	}
	if result == -2 || result == 0 {
		return riskProviderNeuronsReservation{}, ErrRiskProviderDailyNeuronsExhausted
	}
	return reservation, nil
}

func (b *riskProviderNeuronsBudget) Settle(ctx context.Context, provider *model.RiskProvider, reservation riskProviderNeuronsReservation, actual int64) error {
	if provider == nil || reservation.Key == "" || reservation.Estimated < 1 || actual < 0 {
		return ErrRiskProviderBudgetUnavailable
	}
	limit := provider.DailyNeuronsLimit
	if limit < 1 {
		limit = model.DefaultRiskProviderDailyNeuronsLimit
	}
	if !common.RedisEnabled || common.RDB == nil {
		return ErrRiskProviderBudgetUnavailable
	}
	_, err := settleRiskProviderNeuronsScript.Run(ctx, common.RDB, []string{reservation.Key},
		reservation.Window, reservation.Estimated, actual, limit).Int64()
	if err != nil {
		return fmt.Errorf("%w: settle risk provider Neurons: %v", ErrRiskProviderBudgetUnavailable, err)
	}
	return nil
}

func (b *riskProviderNeuronsBudget) Exhaust(ctx context.Context, provider *model.RiskProvider, reservation riskProviderNeuronsReservation) error {
	if provider == nil || reservation.Key == "" || reservation.Estimated < 1 {
		return ErrRiskProviderBudgetUnavailable
	}
	if !common.RedisEnabled || common.RDB == nil {
		return ErrRiskProviderBudgetUnavailable
	}
	_, err := exhaustRiskProviderNeuronsScript.Run(ctx, common.RDB, []string{reservation.Key}, reservation.Window, reservation.Estimated).Int64()
	if err != nil {
		return fmt.Errorf("%w: exhaust risk provider Neurons budget: %v", ErrRiskProviderBudgetUnavailable, err)
	}
	return nil
}

func (b *riskProviderNeuronsBudget) State(ctx context.Context, provider *model.RiskProvider) (riskProviderNeuronsBudgetState, error) {
	if b == nil || provider == nil || provider.Id < 1 {
		return riskProviderNeuronsBudgetState{}, ErrRiskProviderBudgetUnavailable
	}
	now := b.now().UTC()
	_, window, readyAt, err := riskProviderNeuronsWindow(now, provider.DailyResetTime)
	if err != nil {
		return riskProviderNeuronsBudgetState{}, err
	}
	key := fmt.Sprintf("%s:%d", riskProviderNeuronsBudgetNamespace, provider.Id)
	if !common.RedisEnabled || common.RDB == nil {
		return riskProviderNeuronsBudgetState{}, ErrRiskProviderBudgetUnavailable
	}
	values, getErr := common.RDB.HGetAll(ctx, key).Result()
	if getErr != nil {
		return riskProviderNeuronsBudgetState{}, fmt.Errorf("%w: load risk provider Neurons budget: %v", ErrRiskProviderBudgetUnavailable, getErr)
	}
	if values["window"] != window {
		if values["exhausted"] == "1" && now.Before(readyAt) {
			return riskProviderNeuronsBudgetState{Window: values["window"], Exhausted: true, ReadyAt: readyAt}, nil
		}
		return riskProviderNeuronsBudgetState{Window: window, ReadyAt: readyAt}, nil
	}
	return riskProviderNeuronsBudgetState{
		Window: values["window"], Used: parseInt64(values["used"]),
		Reserved: parseInt64(values["reserved"]), Exhausted: values["exhausted"] == "1", ReadyAt: readyAt,
	}, nil
}

func riskProviderNeuronsWindow(now time.Time, resetTime string) (time.Time, string, time.Time, error) {
	minutes, err := model.ParseRiskProviderDailyResetTime(strings.TrimSpace(resetTime))
	if err != nil {
		return time.Time{}, "", time.Time{}, err
	}
	location := time.FixedZone("UTC+8", 8*60*60)
	localNow := now.In(location)
	reset := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), minutes/60, minutes%60, 0, 0, location)
	if localNow.Before(reset) {
		reset = reset.Add(-24 * time.Hour)
	}
	window := reset.UTC().Format("20060102T150405Z")
	return reset, window, reset.Add(riskProviderNeuronsResetDelay).UTC(), nil
}

var riskProviderNeuronsBudgetService = newRiskProviderNeuronsBudget(time.Now)

func riskProviderNeuronsBudgetForProvider(ctx context.Context, provider *model.RiskProvider, content string) (riskProviderNeuronsReservation, error) {
	return riskProviderNeuronsBudgetService.Reserve(ctx, provider, EstimateCloudflareNeurons(content))
}

func ReviewRiskContentWithBudget(ctx context.Context, provider *model.RiskProvider, content string) (RiskReviewResult, error) {
	if provider == nil || provider.ProviderType != model.RiskProviderCloudflare {
		return ReviewRiskContent(ctx, provider, content)
	}
	reservation, err := riskProviderNeuronsBudgetForProvider(ctx, provider, content)
	if err != nil {
		return RiskReviewResult{}, err
	}
	result, reviewErr := ReviewRiskContent(ctx, provider, content)
	if errors.Is(reviewErr, ErrRiskProviderDailyNeuronsResponse) {
		if exhaustErr := riskProviderNeuronsBudgetService.Exhaust(context.WithoutCancel(ctx), provider, reservation); exhaustErr != nil {
			common.SysLog("failed to exhaust risk provider Neurons budget: " + exhaustErr.Error())
		}
		return result, reviewErr
	}
	if errors.Is(reviewErr, ErrRiskProviderRateLimited) {
		if releaseErr := riskProviderNeuronsBudgetService.Settle(context.WithoutCancel(ctx), provider, reservation, 0); releaseErr != nil {
			common.SysLog("failed to release risk provider Neurons reservation after rate limit: " + releaseErr.Error())
		}
		return result, reviewErr
	}
	if reviewErr != nil {
		if settleErr := riskProviderNeuronsBudgetService.Settle(context.WithoutCancel(ctx), provider, reservation, reservation.Estimated); settleErr != nil {
			common.SysLog("failed to settle risk provider Neurons after provider error: " + settleErr.Error())
		}
		return result, reviewErr
	}
	actual := NormalizeRiskProviderNeurons(result.Usage.Neurons)
	if actual == 0 {
		actual = reservation.Estimated
	}
	if settleErr := riskProviderNeuronsBudgetService.Settle(context.WithoutCancel(ctx), provider, reservation, actual); settleErr != nil {
		common.SysLog("failed to settle risk provider Neurons budget: " + settleErr.Error())
	}
	return result, reviewErr
}

func IsRiskProviderLocalBudgetUnavailable(err error) bool {
	return errors.Is(err, ErrRiskProviderBudgetUnavailable) ||
		(errors.Is(err, ErrRiskProviderDailyNeuronsExhausted) && !errors.Is(err, ErrRiskProviderDailyNeuronsResponse))
}

func isRiskProviderLocalBudgetUnavailable(err error) bool {
	return IsRiskProviderLocalBudgetUnavailable(err)
}

type RiskProviderBudgetSnapshot struct {
	Used      int64
	Reserved  int64
	Exhausted bool
	ReadyAt   time.Time
}

func GetRiskProviderBudgetSnapshot(ctx context.Context, provider *model.RiskProvider) (RiskProviderBudgetSnapshot, error) {
	state, err := riskProviderNeuronsBudgetService.State(ctx, provider)
	if err != nil {
		return RiskProviderBudgetSnapshot{}, err
	}
	return RiskProviderBudgetSnapshot{Used: state.Used, Reserved: state.Reserved, Exhausted: state.Exhausted, ReadyAt: state.ReadyAt}, nil
}
