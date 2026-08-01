// allow: SIZE_OK -- cache keying, local reuse, and distributed flight coordination share one correctness boundary.
package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/go-redis/redis/v8"
	"github.com/samber/hot"
	"golang.org/x/sync/singleflight"
)

const (
	riskReviewCacheNamespace             = "new-api:risk-review:v1"
	riskReviewCacheHMACDomain            = "ai-cove:risk-review-cache-hmac:v1"
	riskReviewCacheTTL                   = 24 * time.Hour
	riskReviewCacheCapacity              = 4096
	riskReviewDistributedFlightNamespace = "new-api:risk-review-flight:v1"
	riskReviewDistributedFlightTTL       = 2 * time.Minute
	riskReviewDistributedResultTTL       = 30 * time.Second
	riskReviewDistributedPollInterval    = 10 * time.Millisecond
)

var (
	ErrInvalidRiskReviewCacheInput            = errors.New("invalid risk review cache input")
	ErrRiskReviewCacheInternal                = errors.New("risk review cache internal error")
	ErrRiskReviewDistributedFlightFailed      = errors.New("risk review distributed flight failed")
	ErrRiskReviewDistributedFlightUnavailable = errors.New("risk review distributed flight unavailable")
)

type RiskReviewSource string

const (
	RiskReviewSourceProvider RiskReviewSource = "provider"
	RiskReviewSourceCache    RiskReviewSource = "cache"
	RiskReviewSourceInflight RiskReviewSource = "inflight"
)

type RiskReviewCacheInput struct {
	Content       string
	PolicyVersion string
}

type RiskReviewOutcome struct {
	Result RiskReviewResult
	Source RiskReviewSource
}

type RiskReviewCall func(context.Context) (RiskReviewResult, error)

type riskReviewCacheStore interface {
	Get(context.Context, string) (RiskReviewResult, bool, error)
	Set(context.Context, string, RiskReviewResult, time.Duration) error
}

type hybridRiskReviewCacheStore struct {
	cache *cachex.HybridCache[RiskReviewResult]
}

func (s hybridRiskReviewCacheStore) Get(ctx context.Context, key string) (RiskReviewResult, bool, error) {
	return s.cache.GetContext(ctx, key)
}

func (s hybridRiskReviewCacheStore) Set(ctx context.Context, key string, result RiskReviewResult, ttl time.Duration) error {
	return s.cache.SetWithTTLContext(ctx, key, result, ttl)
}

type RiskReviewCacheService struct {
	store        riskReviewCacheStore
	hmacKey      []byte
	redis        *redis.Client
	redisEnabled func() bool
	flights      singleflight.Group
}

var releaseRiskReviewDistributedFlightScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)

func NewRiskReviewCacheService() *RiskReviewCacheService {
	cache := cachex.NewHybridCache[RiskReviewResult](cachex.HybridCacheConfig[RiskReviewResult]{
		Namespace: cachex.Namespace(riskReviewCacheNamespace),
		Redis:     common.RDB,
		RedisEnabled: func() bool {
			return common.RedisEnabled && common.RDB != nil
		},
		RedisCodec: cachex.JSONCodec[RiskReviewResult]{},
		Memory: func() *hot.HotCache[string, RiskReviewResult] {
			return hot.NewHotCache[string, RiskReviewResult](hot.LRU, riskReviewCacheCapacity).Build()
		},
	})
	return newRiskReviewCacheServiceWithRedis(
		hybridRiskReviewCacheStore{cache: cache}, common.CryptoSecret, common.RDB,
		func() bool { return common.RedisEnabled && common.RDB != nil },
	)
}

func newRiskReviewCacheService(store riskReviewCacheStore, secret string) *RiskReviewCacheService {
	return newRiskReviewCacheServiceWithRedis(store, secret, nil, nil)
}

func newRiskReviewCacheServiceWithRedis(
	store riskReviewCacheStore, secret string, redisClient *redis.Client, redisEnabled func() bool,
) *RiskReviewCacheService {
	derivedKey := common.HmacSha256Raw([]byte(riskReviewCacheHMACDomain), []byte(secret))
	return &RiskReviewCacheService{
		store: store, hmacKey: derivedKey, redis: redisClient, redisEnabled: redisEnabled,
	}
}

func (s *RiskReviewCacheService) CacheKey(input RiskReviewCacheInput) (string, error) {
	policyVersion := strings.TrimSpace(input.PolicyVersion)
	normalizedContent := NormalizeRiskText(input.Content)
	if policyVersion == "" || normalizedContent == "" {
		return "", ErrInvalidRiskReviewCacheInput
	}
	version := base64.RawURLEncoding.EncodeToString([]byte(policyVersion))
	digest := common.GenerateHMACWithKey(s.hmacKey, normalizedContent)
	return cachex.Namespace(riskReviewCacheNamespace).FullKey(version + ":" + digest), nil
}

func (s *RiskReviewCacheService) Review(ctx context.Context, input RiskReviewCacheInput, review RiskReviewCall) (RiskReviewOutcome, error) {
	if err := ctx.Err(); err != nil {
		return RiskReviewOutcome{}, err
	}
	if review == nil {
		return RiskReviewOutcome{}, ErrInvalidRiskReviewCacheInput
	}
	key, err := s.CacheKey(input)
	if err != nil {
		return RiskReviewOutcome{}, err
	}
	if cached, found, cacheErr := s.store.Get(ctx, key); cacheErr == nil && found && cacheableRiskReview(cached) {
		return RiskReviewOutcome{Result: sanitizeRiskReviewResult(cached), Source: RiskReviewSourceCache}, nil
	}
	if err := ctx.Err(); err != nil {
		return RiskReviewOutcome{}, err
	}

	if s.distributedFlightEnabled() {
		return s.reviewWithDistributedFlight(ctx, key, review)
	}
	return s.reviewWithLocalFlight(ctx, key, review)
}

func (s *RiskReviewCacheService) reviewWithLocalFlight(ctx context.Context, key string, review RiskReviewCall) (RiskReviewOutcome, error) {
	executed := false
	resultChannel := s.flights.DoChan(key, func() (any, error) {
		executed = true
		result, reviewErr := review(ctx)
		if reviewErr != nil {
			return cloneRiskReviewResult(result), reviewErr
		}
		cacheResult := sanitizeRiskReviewResult(result)
		if cacheableRiskReview(cacheResult) {
			_ = s.store.Set(ctx, key, cacheResult, riskReviewCacheTTL)
		}
		return cacheResult, nil
	})

	select {
	case <-ctx.Done():
		return RiskReviewOutcome{}, ctx.Err()
	case flight := <-resultChannel:
		source := RiskReviewSourceInflight
		if executed {
			source = RiskReviewSourceProvider
		}
		outcome := RiskReviewOutcome{Source: source}
		if result, ok := flight.Val.(RiskReviewResult); ok {
			outcome.Result = cloneRiskReviewResult(result)
		}
		if flight.Err != nil {
			if source == RiskReviewSourceInflight {
				outcome.Result = sanitizeRiskReviewResult(outcome.Result)
			}
			return outcome, fmt.Errorf("review risk content: %w", flight.Err)
		}
		result, ok := flight.Val.(RiskReviewResult)
		if !ok {
			return RiskReviewOutcome{}, ErrRiskReviewCacheInternal
		}
		return RiskReviewOutcome{Result: sanitizeRiskReviewResult(result), Source: source}, nil
	}
}

func (s *RiskReviewCacheService) distributedFlightEnabled() bool {
	return s != nil && s.redis != nil && (s.redisEnabled == nil || s.redisEnabled())
}

func (s *RiskReviewCacheService) reviewWithDistributedFlight(ctx context.Context, key string, review RiskReviewCall) (RiskReviewOutcome, error) {
	for {
		token := common.GetUUID()
		acquired, err := s.redis.SetNX(ctx, riskReviewDistributedLockKey(key), token, riskReviewDistributedFlightTTL).Result()
		if err != nil {
			return s.reviewWithLocalFlight(ctx, key, review)
		}
		if !acquired {
			result, found, waitErr := s.waitForDistributedFlight(ctx, key)
			if waitErr != nil {
				if errors.Is(waitErr, ErrRiskReviewDistributedFlightFailed) {
					return RiskReviewOutcome{Source: RiskReviewSourceInflight}, fmt.Errorf("review risk content: %w", waitErr)
				}
				return RiskReviewOutcome{}, fmt.Errorf("%w: %w", ErrRiskReviewDistributedFlightUnavailable, waitErr)
			}
			if found {
				return RiskReviewOutcome{Result: result, Source: RiskReviewSourceInflight}, nil
			}
			continue
		}

		defer s.releaseDistributedFlight(key, token)
		s.clearDistributedFlightSignals(ctx, key)
		if cached, found, cacheErr := s.store.Get(ctx, key); cacheErr == nil && found && cacheableRiskReview(cached) {
			return RiskReviewOutcome{Result: sanitizeRiskReviewResult(cached), Source: RiskReviewSourceCache}, nil
		}

		result, reviewErr := review(ctx)
		if reviewErr != nil {
			s.publishDistributedFlightFailure(key)
			return RiskReviewOutcome{Result: cloneRiskReviewResult(result), Source: RiskReviewSourceProvider}, fmt.Errorf("review risk content: %w", reviewErr)
		}
		cacheResult := sanitizeRiskReviewResult(result)
		if cacheableRiskReview(cacheResult) {
			_ = s.store.Set(ctx, key, cacheResult, riskReviewCacheTTL)
		}
		_ = s.publishDistributedFlightResult(key, cacheResult)
		return RiskReviewOutcome{Result: cacheResult, Source: RiskReviewSourceProvider}, nil
	}
}

func riskReviewDistributedLockKey(key string) string {
	return fmt.Sprintf("%s:lock:%s", riskReviewDistributedFlightNamespace, key)
}

func riskReviewDistributedResultKey(key string) string {
	return fmt.Sprintf("%s:result:%s", riskReviewDistributedFlightNamespace, key)
}

func riskReviewDistributedFailureKey(key string) string {
	return fmt.Sprintf("%s:failure:%s", riskReviewDistributedFlightNamespace, key)
}

func (s *RiskReviewCacheService) waitForDistributedFlight(ctx context.Context, key string) (RiskReviewResult, bool, error) {
	ticker := time.NewTicker(riskReviewDistributedPollInterval)
	defer ticker.Stop()
	for {
		if result, found, cacheErr := s.store.Get(ctx, key); cacheErr == nil && found && cacheableRiskReview(result) {
			return sanitizeRiskReviewResult(result), true, nil
		}
		if result, found, resultErr := s.loadDistributedFlightResult(ctx, key); resultErr != nil {
			return RiskReviewResult{}, false, resultErr
		} else if found {
			return result, true, nil
		}
		failure, failureErr := s.redis.Get(ctx, riskReviewDistributedFailureKey(key)).Result()
		if failureErr == nil && failure != "" {
			return RiskReviewResult{}, false, ErrRiskReviewDistributedFlightFailed
		}
		if failureErr != nil && !errors.Is(failureErr, redis.Nil) {
			return RiskReviewResult{}, false, failureErr
		}
		locked, lockErr := s.redis.Exists(ctx, riskReviewDistributedLockKey(key)).Result()
		if lockErr != nil {
			return RiskReviewResult{}, false, lockErr
		}
		if locked == 0 {
			return RiskReviewResult{}, false, nil
		}
		select {
		case <-ctx.Done():
			return RiskReviewResult{}, false, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *RiskReviewCacheService) loadDistributedFlightResult(ctx context.Context, key string) (RiskReviewResult, bool, error) {
	raw, err := s.redis.Get(ctx, riskReviewDistributedResultKey(key)).Bytes()
	if errors.Is(err, redis.Nil) {
		return RiskReviewResult{}, false, nil
	}
	if err != nil {
		return RiskReviewResult{}, false, err
	}
	var result RiskReviewResult
	if err := common.Unmarshal(raw, &result); err != nil {
		return RiskReviewResult{}, false, err
	}
	return sanitizeRiskReviewResult(result), true, nil
}

func (s *RiskReviewCacheService) publishDistributedFlightResult(key string, result RiskReviewResult) error {
	raw, err := common.Marshal(sanitizeRiskReviewResult(result))
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	return s.redis.Set(ctx, riskReviewDistributedResultKey(key), raw, riskReviewDistributedResultTTL).Err()
}

func (s *RiskReviewCacheService) publishDistributedFlightFailure(key string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = s.redis.Set(ctx, riskReviewDistributedFailureKey(key), "1", riskReviewDistributedResultTTL).Err()
}

func (s *RiskReviewCacheService) clearDistributedFlightSignals(ctx context.Context, key string) {
	clearCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	_ = s.redis.Del(clearCtx, riskReviewDistributedResultKey(key), riskReviewDistributedFailureKey(key)).Err()
}

func (s *RiskReviewCacheService) releaseDistributedFlight(key, token string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = releaseRiskReviewDistributedFlightScript.Run(ctx, s.redis, []string{riskReviewDistributedLockKey(key)}, token).Result()
}

func cacheableRiskReview(result RiskReviewResult) bool {
	switch result.Status {
	case RiskReviewSafe, RiskReviewUnsafe:
		return true
	default:
		return false
	}
}

func cloneRiskReviewResult(result RiskReviewResult) RiskReviewResult {
	result.Categories = append([]string(nil), result.Categories...)
	return result
}

func sanitizeRiskReviewResult(result RiskReviewResult) RiskReviewResult {
	result = cloneRiskReviewResult(result)
	result.ProviderID = 0
	result.ProviderName = ""
	result.ProviderType = ""
	result.Usage = RiskReviewUsage{}
	return result
}
