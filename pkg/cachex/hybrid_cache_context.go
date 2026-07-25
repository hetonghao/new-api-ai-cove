package cachex

import (
	"context"
	"errors"
	"time"

	"github.com/go-redis/redis/v8"
)

func (c *HybridCache[V]) Get(key string) (value V, found bool, err error) {
	return c.GetContext(context.Background(), key)
}

func (c *HybridCache[V]) GetContext(ctx context.Context, key string) (value V, found bool, err error) {
	if err := ctx.Err(); err != nil {
		var zero V
		return zero, false, err
	}
	full := c.ns.FullKey(key)
	if full == "" {
		var zero V
		return zero, false, nil
	}

	if c.redisOn() {
		redisCtx, cancel := context.WithTimeout(ctx, defaultRedisOpTimeout)
		defer cancel()

		raw, getErr := c.redis.Get(redisCtx, full).Result()
		if getErr == nil {
			value, decodeErr := c.redisCodec.Decode(raw)
			if decodeErr != nil {
				var zero V
				return zero, false, decodeErr
			}
			return value, true, nil
		}
		if errors.Is(getErr, redis.Nil) {
			var zero V
			return zero, false, nil
		}
		var zero V
		return zero, false, getErr
	}

	return c.memCache().Get(full)
}

func (c *HybridCache[V]) SetWithTTL(key string, value V, ttl time.Duration) error {
	return c.SetWithTTLContext(context.Background(), key, value, ttl)
}

func (c *HybridCache[V]) SetWithTTLContext(ctx context.Context, key string, value V, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	full := c.ns.FullKey(key)
	if full == "" {
		return nil
	}

	if c.redisOn() {
		raw, err := c.redisCodec.Encode(value)
		if err != nil {
			return err
		}
		redisCtx, cancel := context.WithTimeout(ctx, defaultRedisOpTimeout)
		defer cancel()
		return c.redis.Set(redisCtx, full, raw, ttl).Err()
	}

	c.memCache().SetWithTTL(full, value, ttl)
	return nil
}
