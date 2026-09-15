package common

import (
	"strconv"
	"strings"
	"time"

	napicommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

const deepSeekReasoningTTL = 30 * time.Minute

func IsDeepSeekReasoningRelay(info *RelayInfo, model string) bool {
	if info != nil && info.ChannelType == constant.ChannelTypeDeepSeek {
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
}

func LoadDeepSeekReasoning(info *RelayInfo, previousResponseID string) string {
	if info == nil || !napicommon.RedisEnabled {
		return ""
	}
	if previousResponseID = strings.TrimSpace(previousResponseID); previousResponseID != "" {
		if text, err := napicommon.RedisGet(deepSeekReasonIDKey(previousResponseID)); err == nil && text != "" {
			return text
		}
	}
	text, err := napicommon.RedisGet(deepSeekLatestReasonKey(info))
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
