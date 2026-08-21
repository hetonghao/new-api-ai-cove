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

var (
	responsesWebSocketEnvelopeFields    = strings.Fields("type sequence_number response rate_limits metadata id object created_at status background error model environment_id output output_text reasoning reasoning_content tool tools tool_calls tool_call function function_call custom_tool_call content item delta audio arguments annotations recipient state usage input_tokens output_tokens total_tokens cached_tokens reasoning_tokens audio_tokens input_tokens_details output_tokens_details audio_tokens_details reasoning_tokens_details input code message param status_code")
	responsesWebSocketApplicationFields = strings.Fields("output output_text reasoning reasoning_content tool tools tool_calls tool_call function function_call custom_tool_call content item delta audio arguments annotations recipient usage state status status_code")
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
		types.ErrOptionWithSkipRetry(),
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
		hasOutput, unknown := responsesWebSocketInspectEnvelope(payload, false)
		return !hasOutput && !unknown
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
	hasOutput, unknown := responsesWebSocketInspectEnvelope(payload, responsesWebSocketTerminalErrorEvent(payload))
	return hasOutput || unknown
}

func responsesWebSocketInspectEnvelope(payload []byte, terminal bool) (bool, bool) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return true, true
	}
	root := gjson.ParseBytes(payload)
	if !root.IsObject() {
		return true, true
	}
	opaqueField := ""
	switch strings.TrimSpace(gjson.GetBytes(payload, "type").String()) {
	case "codex.rate_limits":
		opaqueField = "rate_limits"
	case "codex.response.metadata":
		opaqueField = "metadata"
	}
	return responsesWebSocketInspectEnvelopeValue(root, "", terminal, opaqueField)
}

func responsesWebSocketInspectEnvelopeValue(value gjson.Result, field string, terminal bool, opaqueField string) (bool, bool) {
	field = strings.ToLower(strings.TrimSpace(field))
	if field != "" {
		if field == opaqueField {
			return false, false
		}
		if !responsesWebSocketFieldIn(responsesWebSocketEnvelopeFields, field) {
			return true, true
		}
	}
	if !value.Exists() || value.Type == gjson.Null {
		return false, false
	}
	hasOutput := responsesWebSocketApplicationValue(value, field, terminal)
	var unknown bool
	if value.IsObject() {
		value.ForEach(func(key, child gjson.Result) bool {
			childOutput, childUnknown := responsesWebSocketInspectEnvelopeValue(child, key.String(), terminal, opaqueField)
			hasOutput = hasOutput || childOutput
			unknown = unknown || childUnknown
			return !unknown
		})
	} else if value.IsArray() {
		for _, child := range value.Array() {
			childOutput, childUnknown := responsesWebSocketInspectEnvelopeValue(child, field, terminal, opaqueField)
			hasOutput = hasOutput || childOutput
			unknown = unknown || childUnknown
			if unknown {
				break
			}
		}
	}
	return hasOutput, unknown
}

func responsesWebSocketApplicationValue(value gjson.Result, field string, terminal bool) bool {
	if !responsesWebSocketFieldIn(responsesWebSocketApplicationFields, field) {
		return false
	}
	switch field {
	case "usage":
		return responsesWebSocketJSONValueNonZero(value)
	case "status", "status_code":
		if terminal {
			if value.Type == gjson.Number {
				code := value.Int()
				return code < http.StatusBadRequest || code > 599
			}
			if value.Type == gjson.String {
				state := strings.ToLower(strings.TrimSpace(value.String()))
				return state != "failed" && state != "error" && state != "cancelled" && state != "canceled"
			}
		} else if value.Type == gjson.String && strings.EqualFold(strings.TrimSpace(value.String()), "in_progress") {
			return false
		}
	case "state":
		if value.Type == gjson.String {
			state := strings.ToLower(strings.TrimSpace(value.String()))
			if terminal {
				return state != "failed" && state != "error" && state != "cancelled" && state != "canceled"
			}
			return state != "" && state != "in_progress"
		}
	}
	return responsesWebSocketJSONValueNonEmptyResult(value)
}

func responsesWebSocketFieldIn(fields []string, field string) bool {
	for _, known := range fields {
		if known == field {
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

func responsesWebSocketJSONValueNonEmptyResult(value gjson.Result) bool {
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
	if value.Type == gjson.True {
		return true
	}
	if value.Type == gjson.False {
		return false
	}
	return value.Num != 0
}
