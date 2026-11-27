package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"
)

// Each gateway shares one short-lived CPA reading across its open monitors.
// Mutations always resolve the current inventory directly, never through this cache.
var deviMonLive struct {
	sync.Mutex
	reading *deviMonLiveReading
}

type deviMonLiveReading struct {
	base    string
	keyHash [32]byte
	done    chan struct{}
	until   time.Time
	data    map[string]DeviMonDevin
	err     error
}

func GetDeviMonLiveInventory(ctx context.Context) (map[string]DeviMonDevin, error) {
	key, base := DeviMonKey(), deviMonDevinBase
	if key == "" {
		return nil, errors.New("CPA_MANAGEMENT_KEY is not configured")
	}
	hash := sha256.Sum256([]byte(key))
	deviMonLive.Lock()
	r := deviMonLive.reading
	if r == nil || r.base != base || r.keyHash != hash || (!r.until.IsZero() && !time.Now().Before(r.until)) {
		r = &deviMonLiveReading{base: base, keyHash: hash, done: make(chan struct{})}
		deviMonLive.reading = r
		go func(r *deviMonLiveReading) {
			started := time.Now()
			fetch, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			files, err := deviMonAuthFiles(fetch, key, base)
			var data map[string]DeviMonDevin
			until := started.Add(time.Second)
			if err == nil {
				data = deviMonDevinInventory(files)
			} else {
				until = time.Now().Add(5 * time.Second)
			}
			deviMonLive.Lock()
			r.data, r.err, r.until = data, err, until
			close(r.done)
			deviMonLive.Unlock()
		}(r)
	}
	deviMonLive.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-r.done:
		return r.data, r.err
	}
}

func invalidateDeviMonLiveInventory() {
	deviMonLive.Lock()
	deviMonLive.reading = nil
	deviMonLive.Unlock()
}
