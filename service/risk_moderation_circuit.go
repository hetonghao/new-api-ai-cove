package service

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
)

var ErrRiskModerationCircuitOpen = errors.New("risk moderation circuit is open")

const (
	RiskProviderStatusNormal         = "normal"
	RiskProviderStatusCircuitOpen    = "circuit_open"
	RiskProviderStatusDailyExhausted = "daily_exhausted"
)

type riskModerationCircuitState struct {
	failures  int
	openUntil time.Time
	probing   bool
}

type riskModerationCircuitPermit struct {
	key       string
	probe     bool
	shared    bool
	threshold int
	cooldown  time.Duration
}

type riskModerationCircuit struct {
	mu     sync.Mutex
	states map[string]*riskModerationCircuitState
	now    func() time.Time
}

var riskModerationProductionCircuit = newRiskModerationCircuit(time.Now)

var (
	riskModerationCircuitAllowScript = redis.NewScript(`
local open_until = tonumber(redis.call("HGET", KEYS[1], "open_until") or "0")
local probing = redis.call("HGET", KEYS[1], "probing") or "0"
if open_until == 0 then
  return 1
end
if tonumber(ARGV[1]) < open_until or probing == "1" then
  return 0
end
redis.call("HSET", KEYS[1], "probing", "1")
redis.call("EXPIRE", KEYS[1], math.max(tonumber(ARGV[2]) * 2, 60))
return 2
`)
	riskModerationCircuitSuccessScript = redis.NewScript(`
redis.call("DEL", KEYS[1])
return 1
`)
	riskModerationCircuitAbandonScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 1 then
  redis.call("HSET", KEYS[1], "probing", "0")
end
return 1
`)
	riskModerationCircuitFailureScript = redis.NewScript(`
local failures = tonumber(redis.call("HGET", KEYS[1], "failures") or "0") + 1
local threshold = tonumber(ARGV[2])
if ARGV[3] == "1" or failures >= threshold then
  failures = threshold
  redis.call("HSET", KEYS[1], "failures", failures, "open_until", tonumber(ARGV[1]) + tonumber(ARGV[4]), "probing", "0")
else
  redis.call("HSET", KEYS[1], "failures", failures, "probing", "0")
end
redis.call("EXPIRE", KEYS[1], math.max(tonumber(ARGV[4]) * 2, 60))
return failures
`)
	riskModerationCircuitStatusScript = redis.NewScript(`
local open_until = tonumber(redis.call("HGET", KEYS[1], "open_until") or "0")
local probing = redis.call("HGET", KEYS[1], "probing") or "0"
if open_until > tonumber(ARGV[1]) or probing == "1" then
  return 1
end
return 0
`)
)

func newRiskModerationCircuit(now func() time.Time) *riskModerationCircuit {
	if now == nil {
		now = time.Now
	}
	return &riskModerationCircuit{states: make(map[string]*riskModerationCircuitState), now: now}
}

func (c *riskModerationCircuit) Allow(ctx context.Context, key string, threshold int, cooldown time.Duration) (riskModerationCircuitPermit, error) {
	ctx = riskModerationCircuitContext(ctx)
	permit := riskModerationCircuitPermit{key: key, threshold: threshold, cooldown: cooldown}
	if c.sharedStateAvailable() {
		result, err := riskModerationCircuitAllowScript.Run(ctx, common.RDB, []string{key}, c.now().Unix(), int64(cooldown/time.Second)).Int()
		if err == nil {
			if result == 0 {
				return riskModerationCircuitPermit{}, ErrRiskModerationCircuitOpen
			}
			permit.probe = result == 2
			permit.shared = true
			return permit, nil
		}
	}
	return c.allowLocal(permit)
}

func (c *riskModerationCircuit) allowLocal(permit riskModerationCircuitPermit) (riskModerationCircuitPermit, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.states[permit.key]
	if state == nil || state.openUntil.IsZero() {
		return permit, nil
	}
	if c.now().Before(state.openUntil) || state.probing {
		return riskModerationCircuitPermit{}, ErrRiskModerationCircuitOpen
	}
	state.probing = true
	permit.probe = true
	return permit, nil
}

func (c *riskModerationCircuit) Success(ctx context.Context, permit riskModerationCircuitPermit) {
	if permit.shared && c.sharedStateAvailable() {
		_ = riskModerationCircuitSuccessScript.Run(riskModerationCircuitContext(ctx), common.RDB, []string{permit.key}).Err()
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.states, permit.key)
}

func (c *riskModerationCircuit) Abandon(ctx context.Context, permit riskModerationCircuitPermit) {
	if !permit.probe {
		return
	}
	if permit.shared && c.sharedStateAvailable() {
		_ = riskModerationCircuitAbandonScript.Run(riskModerationCircuitContext(ctx), common.RDB, []string{permit.key}).Err()
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if state := c.states[permit.key]; state != nil {
		state.probing = false
	}
}

func (c *riskModerationCircuit) Failure(ctx context.Context, permit riskModerationCircuitPermit) {
	if permit.shared && c.sharedStateAvailable() {
		_, _ = riskModerationCircuitFailureScript.Run(riskModerationCircuitContext(ctx), common.RDB, []string{permit.key}, c.now().Unix(), permit.threshold, boolToInt(permit.probe), int64(permit.cooldown/time.Second)).Int()
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.states[permit.key]
	if state == nil {
		state = &riskModerationCircuitState{}
		c.states[permit.key] = state
	}
	state.failures++
	if permit.probe || state.failures >= permit.threshold {
		state.failures = permit.threshold
		state.openUntil = c.now().Add(permit.cooldown)
		state.probing = false
	}
}

func (c *riskModerationCircuit) IsOpen(ctx context.Context, key string) bool {
	if c == nil {
		return false
	}
	if c.sharedStateAvailable() {
		result, err := riskModerationCircuitStatusScript.Run(riskModerationCircuitContext(ctx), common.RDB, []string{key}, c.now().Unix()).Int()
		if err == nil {
			return result == 1
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.states[key]
	return state != nil && (!state.openUntil.IsZero() && (c.now().Before(state.openUntil) || state.probing))
}

func (c *riskModerationCircuit) sharedStateAvailable() bool {
	return c != nil && common.RedisEnabled && common.RDB != nil
}

func riskModerationCircuitContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func RiskProviderCircuitOpen(providerID int) bool {
	return riskModerationProductionCircuit.IsOpen(context.Background(), riskModerationProviderCircuitKey(&model.RiskProvider{Id: providerID}))
}
