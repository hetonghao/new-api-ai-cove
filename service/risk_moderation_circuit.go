package service

import (
	"errors"
	"sync"
	"time"
)

var ErrRiskModerationCircuitOpen = errors.New("risk moderation circuit is open")

type riskModerationCircuitState struct {
	failures  int
	openUntil time.Time
	probing   bool
}

type riskModerationCircuitPermit struct {
	key       string
	probe     bool
	threshold int
	cooldown  time.Duration
}

type riskModerationCircuit struct {
	mu     sync.Mutex
	states map[string]*riskModerationCircuitState
	now    func() time.Time
}

func newRiskModerationCircuit(now func() time.Time) *riskModerationCircuit {
	if now == nil {
		now = time.Now
	}
	return &riskModerationCircuit{states: make(map[string]*riskModerationCircuitState), now: now}
}

func (c *riskModerationCircuit) Allow(key string, threshold int, cooldown time.Duration) (riskModerationCircuitPermit, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	permit := riskModerationCircuitPermit{key: key, threshold: threshold, cooldown: cooldown}
	state := c.states[key]
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

func (c *riskModerationCircuit) Success(permit riskModerationCircuitPermit) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.states, permit.key)
}

func (c *riskModerationCircuit) Abandon(permit riskModerationCircuitPermit) {
	if !permit.probe {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if state := c.states[permit.key]; state != nil {
		state.probing = false
	}
}

func (c *riskModerationCircuit) Failure(permit riskModerationCircuitPermit) {
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
