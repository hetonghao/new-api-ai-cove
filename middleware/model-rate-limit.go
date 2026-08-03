package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/common/limiter"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

type ModelRequestRateLimitTicket struct {
	recordSuccess func()
}

func (t *ModelRequestRateLimitTicket) RecordSuccess() {
	if t != nil && t.recordSuccess != nil {
		t.recordSuccess()
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
		duration:        int64(setting.ModelRequestRateLimitDurationMinutes * 60),
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
			limiter.WithCapacity(int64(config.totalMaxCount)*config.duration),
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

func takeMemoryModelRequestRateLimit(config modelRequestRateLimitConfig) (*ModelRequestRateLimitTicket, *types.NewAPIError) {
	inMemoryRateLimiter.Init(time.Duration(setting.ModelRequestRateLimitDurationMinutes) * time.Minute)
	totalKey := ModelRequestRateLimitCountMark + config.userID
	if config.totalMaxCount > 0 && !inMemoryRateLimiter.Request(totalKey, config.totalMaxCount, config.duration) {
		message := fmt.Sprintf("您已达到总请求数限制：%d分钟内最多请求%d次，包括失败次数，请检查您的请求是否正确", setting.ModelRequestRateLimitDurationMinutes, config.totalMaxCount)
		return nil, types.NewErrorWithStatusCode(errors.New(message), types.ErrorCode("rate_limit_exceeded"), http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())
	}

	successKey := ModelRequestRateLimitSuccessCountMark + config.userID
	if !inMemoryRateLimiter.CanRequest(successKey, config.successMaxCount, config.duration) {
		message := fmt.Sprintf("您已达到请求数限制：%d分钟内最多请求%d次", setting.ModelRequestRateLimitDurationMinutes, config.successMaxCount)
		return nil, types.NewErrorWithStatusCode(errors.New(message), types.ErrorCode("rate_limit_exceeded"), http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())
	}

	return &ModelRequestRateLimitTicket{recordSuccess: func() {
		if config.successMaxCount > 0 {
			inMemoryRateLimiter.Request(successKey, config.successMaxCount, config.duration)
		}
	}}, nil
}

func TakeModelRequestRateLimit(c *gin.Context) (*ModelRequestRateLimitTicket, *types.NewAPIError) {
	if !setting.ModelRequestRateLimitEnabled {
		return &ModelRequestRateLimitTicket{}, nil
	}
	config := getModelRequestRateLimitConfig(c)
	if common.RedisEnabled {
		return takeRedisModelRequestRateLimit(c, config)
	}
	return takeMemoryModelRequestRateLimit(config)
}

// ModelRequestRateLimit 模型请求限流中间件
func ModelRequestRateLimit() func(c *gin.Context) {
	return func(c *gin.Context) {
		ticket, apiErr := TakeModelRequestRateLimit(c)
		if apiErr != nil {
			abortWithOpenAiMessage(c, apiErr.StatusCode, apiErr.Error(), apiErr.GetErrorCode())
			return
		}
		c.Next()
		if c.Writer.Status() < http.StatusBadRequest {
			ticket.RecordSuccess()
		}
	}
}
