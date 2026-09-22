package service

import (
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

const relayAttemptErrorsKey = "relay_attempt_errors"
const relayAttemptErrorHistoryMax = 32

// RecordRelayAttemptError keeps a bounded history of every failed attempt so the
// final error can fall back to earlier capacity evidence and the error log can
// list the whole attempt chain for operators.
func RecordRelayAttemptError(c *gin.Context, err *types.NewAPIError) {
	if c == nil || err == nil {
		return
	}
	history := RelayAttemptErrors(c)
	if len(history) >= relayAttemptErrorHistoryMax {
		return
	}
	c.Set(relayAttemptErrorsKey, append(history, err))
}

func RelayAttemptErrors(c *gin.Context) []*types.NewAPIError {
	if c == nil {
		return nil
	}
	records, _ := c.Get(relayAttemptErrorsKey)
	history, _ := records.([]*types.NewAPIError)
	return history
}

// DecideRelayRetry is the single retry decision for relay attempts. The reason
// is recorded in the request policy decision events of the log details.
func DecideRelayRetry(c *gin.Context, err *types.NewAPIError, retryTimes int) PolicyDecision {
	if err == nil {
		return PolicyDecision{Action: "stop", Reason: "request_completed", Source: "system"}
	}
	if ShouldSkipRetryAfterChannelAffinityFailure(c) {
		source := RequestPolicy(c).SessionModeSource
		if source == "" {
			source = "session_rule"
		}
		return PolicyDecision{Action: "stop", Reason: "strict_session", Source: source}
	}
	if GetChannelConstraints(c).SuppressesRetry() {
		return PolicyDecision{Action: "stop", Reason: "pinned_channel", Source: "channel_constraint"}
	}
	// 预算耗尽或显式 skip-retry 优先于渠道错误：容量重试与计费重绑不能越过这两条安全边界。
	if retryTimes <= 0 {
		return PolicyDecision{Action: "stop", Reason: "attempt_budget_exhausted", Source: "global"}
	}
	if types.IsSkipRetryError(err) {
		return PolicyDecision{Action: "stop", Reason: "non_retryable_error", Source: "system"}
	}
	if types.IsChannelError(err) {
		return PolicyDecision{Action: "retry", Reason: "channel_error", Source: "system"}
	}
	code := err.StatusCode
	if code >= 200 && code < 300 {
		return PolicyDecision{Action: "stop", Reason: "system_retry_exclusion", Source: "system"}
	}
	if code < 100 || code > 599 {
		return PolicyDecision{Action: "retry", Reason: "unrecognized_status", Source: "system"}
	}
	if operation_setting.IsAlwaysSkipRetryCode(err.GetErrorCode()) || operation_setting.IsAlwaysSkipRetryStatusCode(code) {
		return PolicyDecision{Action: "stop", Reason: "system_retry_exclusion", Source: "system"}
	}
	if operation_setting.ShouldRetryByStatusCode(code) {
		return PolicyDecision{Action: "retry", Reason: "retry_status_matched", Source: "global"}
	}
	return PolicyDecision{Action: "stop", Reason: "status_not_retryable", Source: "global"}
}

func ShouldRetryRelayError(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	return DecideRelayRetry(c, openaiErr, retryTimes).Action == "retry"
}

func ProcessChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, relayInfo *relaycommon.RelayInfo) {
	if err == nil {
		return
	}
	ProcessChannelFailure(c, channelError, err)
	RecordRelayErrorLog(c, err, channelError.ChannelId, relayInfo)
}

// ProcessChannelFailure logs the failed attempt and schedules channel auto-disable
// without writing an error log row; WebSocket sessions record their own logs.
func ProcessChannelFailure(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
	if err == nil {
		return
	}
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.MaskSensitiveErrorWithStatusCode())))
	if c != nil && c.Request != nil && c.GetHeader(common.QualityInspectionHeader) != "" {
		return
	}
	if ShouldDisableChannel(err) && channelError.AutoBan {
		reason := err.MaskSensitiveErrorWithStatusCode()
		gopool.Go(func() {
			DisableChannel(channelError, reason)
		})
	}
}

// RecordRelayErrorLog persists the error log row for one failed relay attempt.
func RecordRelayErrorLog(c *gin.Context, err *types.NewAPIError, channelId int, relayInfo *relaycommon.RelayInfo) {
	if err == nil || !constant.ErrorLogEnabled || !types.IsRecordErrorLog(err) {
		return
	}
	userId := c.GetInt("id")
	tokenName := c.GetString("token_name")
	modelName := c.GetString("original_model")
	tokenId := c.GetInt("token_id")
	userGroup := c.GetString("group")
	other := model.NewLogOther()
	if c.Request != nil && c.Request.URL != nil {
		other.SetPublic("request_path", c.Request.URL.Path)
	}
	other.SetPublic("error_type", err.GetErrorType())
	other.SetPublic("error_code", err.GetErrorCode())
	other.SetPublic("status_code", err.StatusCode)
	if history := RelayAttemptErrors(c); len(history) > 0 {
		attemptErrors := make([]map[string]any, 0, len(history))
		for _, attempt := range history {
			attemptErrors = append(attemptErrors, map[string]any{"code": attempt.GetErrorCode(), "status": attempt.StatusCode})
		}
		other.SetAdmin("attempt_errors", attemptErrors)
	}
	AppendRelayLogAdminInfo(c, relayInfo, other)
	AppendResponseModelLogInfo(relayInfo, other)
	AppendTaskPluginContextAuditInfo(c, other)
	AppendRelayTransportLogInfo(c, other)
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}
	useTimeSeconds := int(time.Since(startTime).Seconds())
	model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveErrorWithStatusCode(), tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), userGroup, other)
}
