package middleware

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// ModelRequestRateLimitTicket is one admitted request. HTTP requests hold an
// in-memory reservation until their outcome is known; WebSocket sessions take a
// non-reserving ticket because their many exit paths cannot guarantee a release.
type ModelRequestRateLimitTicket struct {
	recordSuccess func()
	reservation   *common.RateLimitReservation
}

func (t *ModelRequestRateLimitTicket) RecordSuccess() {
	if t == nil {
		return
	}
	if t.reservation != nil {
		t.reservation.Complete(true)
		return
	}
	if t.recordSuccess != nil {
		t.recordSuccess()
	}
}

// Release frees a reservation whose request failed; it is a no-op for
// non-reserving tickets and after RecordSuccess.
func (t *ModelRequestRateLimitTicket) Release() {
	if t != nil && t.reservation != nil {
		t.reservation.Complete(false)
	}
}

type modelRequestRateLimitConfig struct {
	duration        int64
	totalMaxCount   int
	successMaxCount int
	userID          string
}

const (
	ModelRequestRateLimitCountMark        = "MRRL"
	ModelRequestRateLimitSuccessCountMark = "MRRLS"
	modelRateLimitTimeFormat              = "2006-01-02T15:04:05.000Z"
)

// 检查Redis中的请求限制
func checkRedisRateLimit(ctx context.Context, rdb *redis.Client, key string, maxCount int, duration int64) (bool, error) {
	// 如果maxCount为0，表示不限制
	if maxCount == 0 {
		return true, nil
	}

	// 获取当前计数
	length, err := rdb.LLen(ctx, key).Result()
	if err != nil {
		return false, err
	}

	// 如果未达到限制，允许请求
	if length < int64(maxCount) {
		return true, nil
	}

	// 检查时间窗口
	oldTimeStr, _ := rdb.LIndex(ctx, key, -1).Result()
	oldTime, err := time.Parse(modelRateLimitTimeFormat, oldTimeStr)
	if err != nil {
		return false, err
	}

	nowTimeStr := time.Now().UTC().Format(modelRateLimitTimeFormat)
	nowTime, err := time.Parse(modelRateLimitTimeFormat, nowTimeStr)
	if err != nil {
		return false, err
	}
	// 如果在时间窗口内已达到限制，拒绝请求
	subTime := nowTime.Sub(oldTime).Seconds()
	if int64(subTime) < duration {
		rdb.Expire(ctx, key, time.Duration(setting.ModelRequestRateLimitDurationMinutes)*time.Minute)
		return false, nil
	}

	return true, nil
}

// 记录Redis请求
func recordRedisRequest(ctx context.Context, rdb *redis.Client, key string, maxCount int) {
	// 如果maxCount为0，不记录请求
	if maxCount == 0 {
		return
	}

	now := time.Now().UTC().Format(modelRateLimitTimeFormat)
	rdb.LPush(ctx, key, now)
	rdb.LTrim(ctx, key, 0, int64(maxCount-1))
	rdb.Expire(ctx, key, time.Duration(setting.ModelRequestRateLimitDurationMinutes)*time.Minute)
}

func getModelRequestRateLimitConfig(c *gin.Context) modelRequestRateLimitConfig {
	config := modelRequestRateLimitConfig{
		duration:        rateLimitDurationSeconds(setting.ModelRequestRateLimitDurationMinutes),
		totalMaxCount:   setting.ModelRequestRateLimitCount,
		successMaxCount: setting.ModelRequestRateLimitSuccessCount,
		userID:          strconv.Itoa(c.GetInt("id")),
	}
	group := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	if group == "" {
		group = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	if totalCount, successCount, found := setting.GetGroupRateLimit(group); found {
		config.totalMaxCount = totalCount
		config.successMaxCount = successCount
	}
	return config
}

func takeRedisModelRequestRateLimit(c *gin.Context, config modelRequestRateLimitConfig) (*ModelRequestRateLimitTicket, *types.NewAPIError) {
	ctx := c.Request.Context()
	successKey := fmt.Sprintf("rateLimit:%s:%s", ModelRequestRateLimitSuccessCountMark, config.userID)
	allowed, err := checkRedisRateLimit(ctx, common.RDB, successKey, config.successMaxCount, config.duration)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCode("rate_limit_check_failed"), http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
	}
	if !allowed {
		message := fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, config.successMaxCount)
		return nil, types.NewErrorWithStatusCode(errors.New(message), types.ErrorCode("rate_limit_exceeded"), http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())
	}

	if config.totalMaxCount > 0 {
		totalKey := fmt.Sprintf("rateLimit:%s", config.userID)
		tokenBucket := limiter.New(ctx, common.RDB)
		allowed, err = tokenBucket.Allow(
			ctx,
			totalKey,
			limiter.WithCapacity(rateLimitCapacity(config.totalMaxCount, config.duration)),
			limiter.WithRate(int64(config.totalMaxCount)),
			limiter.WithRequested(config.duration),
		)
		if err != nil {
			return nil, types.NewErrorWithStatusCode(err, types.ErrorCode("rate_limit_check_failed"), http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
		}
		if !allowed {
			message := fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", setting.ModelRequestRateLimitDurationMinutes, config.totalMaxCount)
			return nil, types.NewErrorWithStatusCode(errors.New(message), types.ErrorCode("rate_limit_exceeded"), http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())
		}
	}

	return &ModelRequestRateLimitTicket{recordSuccess: func() {
		recordRedisRequest(ctx, common.RDB, successKey, config.successMaxCount)
	}}, nil
}

func takeMemoryModelRequestRateLimit(config modelRequestRateLimitConfig, reserve bool) (*ModelRequestRateLimitTicket, *types.NewAPIError) {
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)
	totalKey := ModelRequestRateLimitCountMark + config.userID
	if config.totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, config.totalMaxCount, config.duration) {
		message := fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", setting.ModelRequestRateLimitDurationMinutes, config.totalMaxCount)
		return nil, types.NewErrorWithStatusCode(errors.New(message), types.ErrorCode("rate_limit_exceeded"), http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())
	}

	successKey := ModelRequestRateLimitSuccessCountMark + config.userID
	successLimited := fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, config.successMaxCount)
	if reserve && config.successMaxCount > 0 {
		reservation := inMemoryRateLimiter.Reserve(successKey, config.successMaxCount, config.duration)
		if reservation == nil {
			return nil, types.NewErrorWithStatusCode(errors.New(successLimited), types.ErrorCode("rate_limit_exceeded"), http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())
		}
		return &ModelRequestRateLimitTicket{reservation: reservation}, nil
	}
	if !inMemoryRateLimiter.CanRequest(successKey, config.successMaxCount, config.duration) {
		return nil, types.NewErrorWithStatusCode(errors.New(successLimited), types.ErrorCode("rate_limit_exceeded"), http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())
	}

	return &ModelRequestRateLimitTicket{recordSuccess: func() {
		if config.successMaxCount > 0 {
			inMemoryRateLimiter.Request(successKey, config.successMaxCount, config.duration)
		}
	}}, nil
}

// TakeModelRequestRateLimit admits one WebSocket-driven request without holding
// an in-memory reservation; the caller records success explicitly.
func TakeModelRequestRateLimit(c *gin.Context) (*ModelRequestRateLimitTicket, *types.NewAPIError) {
	if !setting.ModelRequestRateLimitEnabled {
		return &ModelRequestRateLimitTicket{}, nil
	}
	config := getModelRequestRateLimitConfig(c)
	if common.RedisEnabled {
		return takeRedisModelRequestRateLimit(c, config)
	}
	return takeMemoryModelRequestRateLimit(config, false)
}

func modelRequestSucceeded(c *gin.Context) bool {
	status, _ := common.GetContextKeyType[*relaycommon.StreamStatus](c, constant.ContextKeyResponseStreamStatus)
	return c.Writer.Status() < http.StatusBadRequest && !status.ResponseFailed()
}

// modelRequestRateLimitHandler admits one HTTP request against config; the
// in-memory backend holds a reservation until the response outcome is known.
func modelRequestRateLimitHandler(config modelRequestRateLimitConfig, useRedis bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		config.userID = strconv.Itoa(c.GetInt("id"))
		var ticket *ModelRequestRateLimitTicket
		var apiErr *types.NewAPIError
		if useRedis {
			ticket, apiErr = takeRedisModelRequestRateLimit(c, config)
		} else {
			ticket, apiErr = takeMemoryModelRequestRateLimit(config, true)
		}
		if apiErr != nil {
			abortWithOpenAiMessage(c, apiErr.StatusCode, apiErr.Error(), apiErr.GetErrorCode())
			return
		}
		defer ticket.Release()
		c.Next()
		if modelRequestSucceeded(c) {
			ticket.RecordSuccess()
		}
	}
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		if !setting.ModelRequestRateLimitEnabled {
			c.Next()
			return
		}
		modelRequestRateLimitHandler(getModelRequestRateLimitConfig(c), common.RedisEnabled)(c)
	}
}

func rateLimitDurationSeconds(durationMinutes int) int64 {
	if durationMinutes <= 0 {
		return 0
	}
	minutes := int64(durationMinutes)
	if minutes > math.MaxInt64/60 {
		return math.MaxInt64
	}
	return minutes * 60
}

func rateLimitCapacity(count int, durationSeconds int64) int64 {
	if count <= 0 || durationSeconds <= 0 {
		return 0
	}
	c := int64(count)
	if c > math.MaxInt64/durationSeconds {
		return math.MaxInt64
	}
	return c * durationSeconds
}
