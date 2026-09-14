package common

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	napicommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/tidwall/gjson"
)

const deepSeekReasoningTTL = 30 * time.Minute

func IsDeepSeekReasoningRelay(info *RelayInfo, model string) bool {
	if info != nil && info.ChannelMeta != nil && info.ChannelType == constant.ChannelTypeDeepSeek {
		return true
	}
	if info != nil && strings.HasPrefix(info.OriginModelName, "deepseek") {
		return true
	}
	return strings.HasPrefix(model, "deepseek")
}

func AppendDeepSeekReasoning(buf *strings.Builder, ev dto.ResponsesStreamResponse) {
	if buf == nil {
		return
	}
	switch ev.Type {
	case "response.reasoning_text.delta":
		buf.WriteString(ev.Delta)
	case "response.reasoning_text.done":
		if ev.Text != nil && strings.TrimSpace(*ev.Text) != "" {
			buf.Reset()
			buf.WriteString(*ev.Text)
		}
	case dto.ResponsesOutputTypeItemDone:
		if ev.Item == nil {
			return
		}
		if text := reasoningTextFromOutput(ev.Item); text != "" {
			buf.Reset()
			buf.WriteString(text)
		}
	case "response.completed", "response.done":
		if ev.Response == nil {
			return
		}
		var last string
		for i := range ev.Response.Output {
			if text := reasoningTextFromOutput(&ev.Response.Output[i]); text != "" {
				last = text
			}
		}
		if last != "" {
			buf.Reset()
			buf.WriteString(last)
		}
	}
}

func SaveDeepSeekReasoning(info *RelayInfo, responseID, text string) {
	text = strings.TrimSpace(text)
	if info == nil || text == "" || !napicommon.RedisEnabled {
		return
	}
	_ = napicommon.RedisSet(deepSeekLatestReasonKey(info), text, deepSeekReasoningTTL)
	if responseID = strings.TrimSpace(responseID); responseID != "" {
		_ = napicommon.RedisSet(deepSeekReasonIDKey(responseID), text, deepSeekReasoningTTL)
	}
	key := info.OpenCodeSession
	if key == "" {
		key = OpenCodeSessionKey(info, nil)
	}
	if key != "" {
		_ = napicommon.RedisSet(openCodeSessionReasonKey(key), text, deepSeekReasoningTTL)
	}
}

func LoadDeepSeekReasoning(info *RelayInfo, previousResponseID string) string {
	if info == nil || !napicommon.RedisEnabled {
		return ""
	}
	if text := LoadDeepSeekReasoningByResponseID(previousResponseID); text != "" {
		return text
	}
	text, err := napicommon.RedisGet(deepSeekLatestReasonKey(info))
	if err != nil {
		return ""
	}
	return text
}

func LoadDeepSeekReasoningByResponseID(responseID string) string {
	if !napicommon.RedisEnabled || napicommon.RDB == nil {
		return ""
	}
	responseID = strings.TrimSpace(responseID)
	if responseID == "" {
		return ""
	}
	text, err := napicommon.RedisGet(deepSeekReasonIDKey(responseID))
	if err != nil {
		return ""
	}
	return text
}

func CompletedDeepSeekResponseID(ev dto.ResponsesStreamResponse) string {
	if ev.Response == nil {
		return ""
	}
	return strings.TrimSpace(ev.Response.ID)
}

func reasoningTextFromOutput(item *dto.ResponsesOutput) string {
	if item == nil || item.Type != "reasoning" {
		return ""
	}
	for _, part := range item.Content {
		if part.Type == "reasoning_text" && strings.TrimSpace(part.Text) != "" {
			return part.Text
		}
	}
	for _, part := range item.Summary {
		if strings.TrimSpace(part.Text) != "" {
			return part.Text
		}
	}
	return ""
}

func deepSeekLatestReasonKey(info *RelayInfo) string {
	return "ds_reason:" + strconv.Itoa(info.UserId) + ":" + strconv.Itoa(info.TokenId) + ":" + info.OriginModelName
}

func deepSeekReasonIDKey(responseID string) string {
	return "ds_reason:id:" + responseID
}

func RememberOpenCodeSessionModel(info *RelayInfo, body []byte) {
	bindOpenCodeSession(info, body)
	if info != nil && !IsDeepSeekReasoningRelay(info, infoOriginModel(info)) {
		WriteOpenCodeSessionModel(info)
	}
}

func bindOpenCodeSession(info *RelayInfo, body []byte) {
	key := OpenCodeSessionKey(info, body)
	if info != nil && key != "" {
		info.OpenCodeSession = key
	}
}

func WriteOpenCodeSessionModel(info *RelayInfo) {
	if info == nil || info.OpenCodeSession == "" || !napicommon.RedisEnabled || !tracksOpenCodeSessionModel(info) {
		return
	}
	model := strings.TrimSpace(infoOriginModel(info))
	if model == "" {
		return
	}
	_ = napicommon.RedisSet(openCodeSessionModelKey(info.OpenCodeSession), model, deepSeekReasoningTTL)
}

func LoadOpenCodeChatThinking(info *RelayInfo, body []byte) (string, OpenCodeSessionModel) {
	last := LoadOpenCodeSessionModel(info, body)
	if last != OpenCodeSessionDeepSeek {
		return "", last
	}
	return LoadOpenCodeSessionReasoning(info, body), last
}

func LoadOpenCodeSessionModel(info *RelayInfo, body []byte) OpenCodeSessionModel {
	key := OpenCodeSessionKey(info, body)
	if key == "" || !napicommon.RedisEnabled || napicommon.RDB == nil {
		return OpenCodeSessionUnknown
	}
	model, err := napicommon.RedisGet(openCodeSessionModelKey(key))
	if err != nil || strings.TrimSpace(model) == "" {
		return OpenCodeSessionUnknown
	}
	if strings.HasPrefix(model, "deepseek") {
		return OpenCodeSessionDeepSeek
	}
	return OpenCodeSessionOther
}

func LoadOpenCodeSessionReasoning(info *RelayInfo, body []byte) string {
	key := OpenCodeSessionKey(info, body)
	if key == "" || !napicommon.RedisEnabled || napicommon.RDB == nil {
		return ""
	}
	text, err := napicommon.RedisGet(openCodeSessionReasonKey(key))
	if err != nil {
		return ""
	}
	return text
}

func OpenCodeSessionKey(info *RelayInfo, body []byte) string {
	if len(body) > 0 {
		if key := validOpenCodeSessionKey(gjson.GetBytes(body, "prompt_cache_key").String()); key != "" {
			return key
		}
	}
	if info != nil {
		if key := validOpenCodeSessionKey(promptCacheKeyFromRelay(info)); key != "" {
			return key
		}
		for _, name := range []string{"session-id", "session_id", "x-opencode-session"} {
			if key := validOpenCodeSessionKey(relayHeader(info, name)); key != "" {
				return key
			}
		}
	}
	return ""
}

func promptCacheKeyFromRelay(info *RelayInfo) string {
	if info == nil || info.Request == nil {
		return ""
	}
	switch req := info.Request.(type) {
	case *dto.OpenAIResponsesRequest:
		return rawJSONString(req.PromptCacheKey)
	case *dto.GeneralOpenAIRequest:
		return req.PromptCacheKey
	default:
		return ""
	}
}

func rawJSONString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.Trim(string(raw), `"`)
}

func validOpenCodeSessionKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" || strings.HasPrefix(key, "AI-Cove") {
		return ""
	}
	return key
}

func relayHeader(info *RelayInfo, name string) string {
	if info == nil || info.RequestHeaders == nil {
		return ""
	}
	if v := strings.TrimSpace(info.RequestHeaders[name]); v != "" {
		return v
	}
	for k, v := range info.RequestHeaders {
		if strings.EqualFold(k, name) {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func openCodeSessionModelKey(session string) string {
	return "ds_sess:" + session + ":model"
}

func openCodeSessionReasonKey(session string) string {
	return "ds_sess:" + session + ":reason"
}

func tracksOpenCodeSessionModel(info *RelayInfo) bool {
	if info == nil {
		return false
	}
	switch info.RelayMode {
	case relayconstant.RelayModeChatCompletions, relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		return true
	default:
		path := info.RequestURLPath
		return strings.Contains(path, "/chat/completions") || strings.Contains(path, "/responses")
	}
}
