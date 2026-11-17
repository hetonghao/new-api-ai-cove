package openai

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// ponytail: hold at most 64 KiB of startup metadata; larger or unknown envelopes
// pass through without transparent retry. Extend only for documented metadata.
const responsesRetryPrefixMax = 64 << 10

// scanResponsesStream keeps the existing scanner's timeout, cancellation and
// writer serialization. Only an explicit, unused rate-limit rejection can leave
// it as a retryable error; no failed-attempt response IDs reach the client.
func scanResponsesStream(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, handle func(string, *helper.StreamResult)) (retryErr *types.NewAPIError, committed, retryRestricted bool) {
	buffering := responsesStreamReplayable(c, resp, info)
	retryRestricted = info.ChannelMeta != nil && info.GetChannelType() == constant.ChannelTypeOpenAI && !buffering
	guarded := buffering
	var pending []string
	pendingBytes := 0
	var lastResult *helper.StreamResult
	flush := func(sr *helper.StreamResult) {
		for _, data := range pending {
			handle(data, sr)
			if sr.IsStopped() {
				break
			}
		}
		pending = nil
	}
	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		lastResult = sr
		if guarded && !buffering {
			committed = true // A later event cannot reopen a released pre-output window.
		}
		if buffering {
			event := gjson.Parse(data)
			kind := event.Get("type").String()
			startup := kind == "response.created" || kind == "response.in_progress"
			terminal := kind == "response.failed" || kind == "response.error" || kind == "error"
			unused := len(data) <= responsesRetryPrefixMax && gjson.Valid(data) &&
				(startup || terminal) && responsesRetryEnvelope(event, false, terminal)
			if terminal && unused && c.Request.Context().Err() == nil {
				errValue := event.Get("response.error")
				if !errValue.IsObject() {
					errValue = event.Get("error")
				}
				if !errValue.IsObject() {
					errValue = event
				}
				if errValue.Get("code").String() == "rate_limit_exceeded" {
					var upstreamError types.OpenAIError
					if common.UnmarshalJsonStr(errValue.Raw, &upstreamError) == nil && upstreamError.Code == "rate_limit_exceeded" {
						retryErr = types.WithOpenAIError(upstreamError, http.StatusTooManyRequests)
						info.StreamStatus.MarkFailed("rate_limit_exceeded", upstreamError.Type, http.StatusTooManyRequests)
						pending = nil
						sr.Stop(nil) // Keep upstream messages out of public stream_status diagnostics.
						return
					}
				}
			}
			if startup && unused && pendingBytes+len(data) <= responsesRetryPrefixMax {
				pending = append(pending, data)
				pendingBytes += len(data)
				return
			}
			buffering = false
			committed = !terminal || !unused
			flush(sr)
			if sr.IsStopped() {
				return
			}
		}
		handle(data, sr)
	})
	// A bare EOF is not a retry signal. Preserve buffered metadata verbatim, but
	// never revive a cancelled, timed-out or fatally stopped stream.
	if retryErr == nil && len(pending) != 0 && lastResult != nil && !lastResult.IsStopped() &&
		c.Request.Context().Err() == nil && !info.StreamStatus.HasErrors() &&
		(info.StreamStatus.EndReason == relaycommon.StreamEndReasonEOF || info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone) {
		flush(lastResult)
	}
	if retryErr != nil {
		end := info.StreamStatus.EndReason
		if c.Request.Context().Err() != nil ||
			end != relaycommon.StreamEndReasonHandlerStop && end != relaycommon.StreamEndReasonEOF && end != relaycommon.StreamEndReasonDone {
			types.ErrOptionWithSkipRetry()(retryErr)
		}
	}
	return retryErr, committed, retryRestricted
}

func responsesStreamReplayable(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) bool {
	if info.ChannelMeta == nil || info.Request == nil || info.GetChannelType() != constant.ChannelTypeOpenAI ||
		model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled ||
		resp.Header.Get("X-Codex-Turn-State") != "" || resp.Header.Get("X-Reasoning-Included") != "" ||
		c.Request.Header.Get("X-Codex-Turn-State") != "" {
		return false
	}
	if request, ok := info.Request.(*dto.OpenAIResponsesRequest); ok {
		if request.PreviousResponseID != "" || len(request.Conversation) != 0 && strings.TrimSpace(string(request.Conversation)) != "null" {
			return false
		}
		input := gjson.ParseBytes(request.Input)
		if input.Type != gjson.Null && input.Type != gjson.String && !input.IsArray() {
			return false
		}
		referenced := false
		input.ForEach(func(_, item gjson.Result) bool {
			encrypted := item.Get("encrypted_content")
			referenced = item.Get("type").String() == "item_reference" || encrypted.Type != gjson.Null && encrypted.String() != ""
			return !referenced
		})
		if referenced {
			return false
		}
	}
	for name := range relaycommon.GetEffectiveHeaderOverride(info) {
		if strings.EqualFold(strings.TrimSpace(name), "X-Codex-Turn-State") {
			return false
		}
	}
	if len(info.ParamOverride) == 0 {
		return true
	}
	// ponytail: body overrides are opaque here; inspect the final outbound JSON
	// before extending eligibility beyond header-only passthrough operations.
	if len(info.ParamOverride) != 1 {
		return false
	}
	operations, ok := info.ParamOverride["operations"].([]any)
	if !ok {
		return false
	}
	for _, raw := range operations {
		operation, ok := raw.(map[string]any)
		if !ok || operation["mode"] != "pass_headers" {
			return false
		}
	}
	return true
}

func responsesRetryEnvelope(value gjson.Result, response, terminal bool) bool {
	if !value.IsObject() {
		return false
	}
	safe := true
	value.ForEach(func(key, field gjson.Result) bool {
		switch key.String() {
		case "response":
			safe = !response && responsesRetryEnvelope(field, true, terminal)
		case "usage", "tool_usage":
			safe = responsesRetryUsageZero(field, 0)
		case "output":
			safe = field.Type == gjson.Null || field.IsArray() && len(field.Array()) == 0
		case "output_text":
			safe = response && (field.Type == gjson.Null || field.Type == gjson.String && field.String() == "")
		case "moderation":
			safe = response && field.Type == gjson.Null
		case "billing":
			safe = response && (field.Type == gjson.Null || field.IsObject())
			if safe && field.IsObject() {
				field.ForEach(func(k, v gjson.Result) bool {
					safe = k.String() == "payer" && (v.Type == gjson.Null || v.Type == gjson.String)
					return safe
				})
			}
		case "prompt_cache_diagnostics":
			safe = response && (field.Type == gjson.Null || field.IsObject())
			if safe && field.IsObject() {
				field.ForEach(func(k, v gjson.Result) bool {
					switch k.String() {
					case "type", "reason":
						safe = v.Type == gjson.Null || v.Type == gjson.String
					case "cache_missed_tokens", "comparison_reusable_tokens":
						safe = v.Type == gjson.Null || v.Type == gjson.Number && v.Num >= 0
					default:
						safe = false
					}
					return safe
				})
			}
		case "error":
			if field.Type != gjson.Null {
				safe = terminal && field.IsObject()
				if safe {
					field.ForEach(func(k, _ gjson.Result) bool {
						switch k.String() {
						case "code", "message", "type", "param":
						default:
							safe = false
						}
						return safe
					})
				}
			}
		case "previous_response_id", "conversation":
			safe = response && (field.Type == gjson.Null || field.Type == gjson.String && strings.TrimSpace(field.String()) == "")
		case "status":
			if terminal {
				safe = field.String() == "failed" || field.String() == "error"
			} else {
				safe = field.String() == "in_progress" || field.String() == "queued"
			}
		case "type", "sequence_number", "obfuscation":
		case "code", "message", "param":
			safe = terminal
		case "id", "object", "created_at", "completed_at", "background", "model", "access_programs", "environment_id",
			"instructions", "max_output_tokens", "max_tool_calls", "parallel_tool_calls",
			"reasoning", "store", "temperature", "tool_choice", "tools", "top_p", "top_logprobs", "truncation",
			"user", "metadata", "incomplete_details", "text", "service_tier", "safety_identifier", "prompt_cache_key",
			"prompt_cache_retention", "prompt_cache_options", "context_management":
			// Response configuration (including tool declarations and reasoning
			// effort) is echoed by the API; generated content belongs in output.
			safe = response
		default:
			safe = false
		}
		return safe
	})
	return safe
}

func responsesRetryUsageZero(value gjson.Result, depth int) bool {
	if depth > 32 {
		return false
	}
	if value.IsObject() || value.IsArray() {
		zero := true
		value.ForEach(func(_, child gjson.Result) bool { zero = responsesRetryUsageZero(child, depth+1); return zero })
		return zero
	}
	switch value.Type {
	case gjson.Null, gjson.False:
		return true
	case gjson.Number:
		return value.Num == 0
	case gjson.String:
		raw := strings.TrimSpace(value.String())
		if raw == "" {
			return true
		}
		number, err := strconv.ParseFloat(raw, 64)
		return err == nil && number == 0
	default:
		return false
	}
}
