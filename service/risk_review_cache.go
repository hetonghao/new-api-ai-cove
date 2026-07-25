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
	"github.com/samber/hot"
	"golang.org/x/sync/singleflight"
)

const (
	riskReviewCacheNamespace  = "new-api:risk-review:v1"
	riskReviewCacheHMACDomain = "ai-cove:risk-review-cache-hmac:v1"
	riskReviewCacheTTL        = 24 * time.Hour
	riskReviewCacheCapacity   = 4096
)

var (
	ErrInvalidRiskReviewCacheInput = errors.New("invalid risk review cache input")
	ErrRiskReviewCacheInternal     = errors.New("risk review cache internal error")
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

func (s hybridRiskReviewCacheStore) Get(_ context.Context, key string) (RiskReviewResult, bool, error) {
	return s.cache.Get(key)
}

func (s hybridRiskReviewCacheStore) Set(_ context.Context, key string, result RiskReviewResult, ttl time.Duration) error {
	return s.cache.SetWithTTL(key, result, ttl)
}

type RiskReviewCacheService struct {
	store   riskReviewCacheStore
	hmacKey []byte
	flights singleflight.Group
}

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
	return newRiskReviewCacheService(hybridRiskReviewCacheStore{cache: cache}, common.CryptoSecret)
}

func newRiskReviewCacheService(store riskReviewCacheStore, secret string) *RiskReviewCacheService {
	derivedKey := common.HmacSha256Raw([]byte(riskReviewCacheHMACDomain), []byte(secret))
	return &RiskReviewCacheService{store: store, hmacKey: derivedKey}
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
		return RiskReviewOutcome{Result: cloneRiskReviewResult(cached), Source: RiskReviewSourceCache}, nil
	}

	executed := false
	resultChannel := s.flights.DoChan(key, func() (any, error) {
		executed = true
		result, reviewErr := review(ctx)
		if reviewErr != nil {
			return nil, reviewErr
		}
		result = cloneRiskReviewResult(result)
		if cacheableRiskReview(result) {
			_ = s.store.Set(ctx, key, result, riskReviewCacheTTL)
		}
		return result, nil
	})

	select {
	case <-ctx.Done():
		return RiskReviewOutcome{}, ctx.Err()
	case flight := <-resultChannel:
		if flight.Err != nil {
			return RiskReviewOutcome{}, fmt.Errorf("review risk content: %w", flight.Err)
		}
		result, ok := flight.Val.(RiskReviewResult)
		if !ok {
			return RiskReviewOutcome{}, ErrRiskReviewCacheInternal
		}
		source := RiskReviewSourceInflight
		if executed {
			source = RiskReviewSourceProvider
		}
		return RiskReviewOutcome{Result: cloneRiskReviewResult(result), Source: source}, nil
	}
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
