package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

const (
	responsesWebSocketCapacityCloseCode    = 4409
	responsesWebSocketCapacityCloseReason  = "ai-cove-capacity/v1;state=rejected;phase=pre_output;code=server_is_overloaded"
	responsesWebSocketCapacityReasonPrefix = "ai-cove-capacity/v1;state=rejected;phase=pre_output;code="
	responsesWebSocketPendingFrameMax      = 16
	responsesWebSocketPendingBytesMax      = 64 << 10
)

func responsesWebSocketCapacityCode(err error) (string, bool) {
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != responsesWebSocketCapacityCloseCode || len(closeErr.Text) > responsesWebSocketCloseMax || !utf8.ValidString(closeErr.Text) || !strings.HasPrefix(closeErr.Text, responsesWebSocketCapacityReasonPrefix) {
		return "", false
	}
	code := strings.TrimPrefix(closeErr.Text, responsesWebSocketCapacityReasonPrefix)
	switch code {
	case "server_is_overloaded", "slow_down", "model_capacity":
		return code, true
	default:
		return "", false
	}
}

func newResponsesWebSocketCapacityError(code string) *types.NewAPIError {
	if code == "" {
		code = "server_is_overloaded"
	}
	return types.NewErrorWithStatusCode(
		errors.New("upstream capacity rejected the request"),
		types.ErrorCode(code),
		http.StatusServiceUnavailable,
	)
}

func mergeResponsesWebSocketCapacityCode(current, candidate string) string {
	if candidate == "" {
		return current
	}
	if current == "" || responsesWebSocketCapacityCodeRank(candidate) > responsesWebSocketCapacityCodeRank(current) {
		return candidate
	}
	return current
}

func responsesWebSocketCapacityFallbackError(code string, fallback *types.NewAPIError) *types.NewAPIError {
	if fallback != nil && responsesWebSocketIsClientRequestError(fallback) {
		return fallback
	}
	if code != "" {
		return newResponsesWebSocketCapacityError(code)
	}
	return fallback
}

func responsesWebSocketIsClientRequestError(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if err.StatusCode >= 400 && err.StatusCode < 500 {
		return true
	}
	switch err.GetErrorCode() {
	case types.ErrorCodeInvalidRequest,
		types.ErrorCodeAccessDenied,
		types.ErrorCodeReadRequestBodyFailed,
		types.ErrorCodeBadRequestBody,
		types.ErrorCodeConvertRequestFailed,
		types.ErrorCodeSensitiveWordsDetected,
		types.ErrorCodeContentPolicyViolation,
		types.ErrorCodeInsufficientUserQuota,
		types.ErrorCodePreConsumeTokenQuotaFailed:
		return true
	default:
		return false
	}
}

func responsesWebSocketCapacityCodeRank(code string) int {
	switch code {
	case "server_is_overloaded":
		return 3
	case "model_capacity":
		return 2
	case "slow_down":
		return 1
	default:
		return 0
	}
}

func responsesWebSocketHasSpecificChannel(c *gin.Context) bool {
	if c == nil {
		return false
	}
	_, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId)
	return ok
}

func responsesWebSocketEventAllowsCapacityRetry(payload []byte) bool {
	eventType := gjson.GetBytes(payload, "type").String()
	switch eventType {
	case "response.created", "response.in_progress", "codex.rate_limits", "codex.response.metadata":
		for _, path := range []string{
			"response.output",
			"response.output_text",
			"response.reasoning",
			"response.tool_calls",
			"response.tool_call",
			"response.function_call",
			"response.content",
			"response.item",
			"output",
			"tool",
			"tool_calls",
			"function_call",
			"arguments",
			"item",
			"delta",
			"audio",
		} {
			if responsesWebSocketJSONValueNonEmpty(payload, path) {
				return false
			}
		}
		for _, path := range []string{"response.usage", "usage"} {
			if responsesWebSocketJSONValueNonZero(gjson.GetBytes(payload, path)) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func responsesWebSocketTerminalErrorEvent(payload []byte) bool {
	switch gjson.GetBytes(payload, "type").String() {
	case "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "error":
		return true
	default:
		return false
	}
}

func responsesWebSocketEventHasApplicationOutput(payload []byte) bool {
	for _, path := range []string{
		"response.output",
		"response.output_text",
		"response.reasoning",
		"response.tool_calls",
		"response.tool_call",
		"response.content",
		"response.item",
		"response.function_call",
		"output",
		"tool",
		"tool_calls",
		"function_call",
		"arguments",
		"item",
		"delta",
		"audio",
	} {
		if responsesWebSocketJSONValueNonEmpty(payload, path) {
			return true
		}
	}
	for _, path := range []string{"response.usage", "usage"} {
		if responsesWebSocketJSONValueNonZero(gjson.GetBytes(payload, path)) {
			return true
		}
	}
	return false
}

func responsesWebSocketJSONValueNonZero(value gjson.Result) bool {
	if !value.Exists() {
		return false
	}
	if value.IsArray() {
		for _, item := range value.Array() {
			if responsesWebSocketJSONValueNonZero(item) {
				return true
			}
		}
		return false
	}
	switch value.Type {
	case gjson.Number:
		return value.Num != 0
	case gjson.String:
		raw := strings.TrimSpace(value.String())
		if raw == "" || raw == "0" {
			return false
		}
		parsed, err := strconv.ParseFloat(raw, 64)
		return err != nil || parsed != 0
	case gjson.True:
		return value.Bool()
	case gjson.False:
		return false
	case gjson.JSON:
		nonZero := false
		value.ForEach(func(_, item gjson.Result) bool {
			nonZero = responsesWebSocketJSONValueNonZero(item)
			return !nonZero
		})
		return nonZero
	default:
		return false
	}
}

func responsesWebSocketJSONValueNonEmpty(payload []byte, path string) bool {
	value := gjson.GetBytes(payload, path)
	if !value.Exists() {
		return false
	}
	if value.IsArray() {
		return len(value.Array()) > 0
	}
	if value.IsObject() {
		return strings.TrimSpace(value.Raw) != "{}"
	}
	if value.Type == gjson.String {
		return strings.TrimSpace(value.String()) != ""
	}
	return value.Num != 0
}
