package common

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func PrepareChatCompletionsBody(info *RelayInfo, body []byte) []byte {
	if len(body) == 0 || !IsDeepSeekReasoningRelay(info, infoOriginModel(info)) {
		return body
	}
	return NormalizeChatCompletionsReasoning(body)
}

func NormalizeChatCompletionsReasoning(body []byte) []byte {
	body = copyAssistantReasoning(body, "messages")
	choices := gjson.GetBytes(body, "choices")
	if !choices.IsArray() {
		return body
	}
	result := body
	for i, choice := range choices.Array() {
		msg := choice.Get("message")
		if strings.TrimSpace(msg.Get("reasoning_content").String()) != "" {
			continue
		}
		alt := chatReasoningText(msg)
		if alt == "" {
			continue
		}
		next, err := sjson.SetBytes(result, fmt.Sprintf("choices.%d.message.reasoning_content", i), alt)
		if err != nil {
			return result
		}
		result = next
	}
	return result
}

func copyAssistantReasoning(body []byte, arrayPath string) []byte {
	messages := gjson.GetBytes(body, arrayPath)
	if !messages.IsArray() {
		return body
	}
	result := body
	for i, msg := range messages.Array() {
		if msg.Get("role").String() != "assistant" {
			continue
		}
		if strings.TrimSpace(msg.Get("reasoning_content").String()) != "" {
			continue
		}
		alt := chatReasoningText(msg)
		if alt == "" {
			continue
		}
		next, err := sjson.SetBytes(result, fmt.Sprintf("%s.%d.reasoning_content", arrayPath, i), alt)
		if err != nil {
			return result
		}
		result = next
	}
	return result
}

func injectChatCachedReasoning(body []byte, cached string) []byte {
	cached = strings.TrimSpace(cached)
	if cached == "" {
		return body
	}
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}
	last := -1
	arr := messages.Array()
	for i, msg := range arr {
		if msg.Get("role").String() == "assistant" {
			last = i
		}
	}
	if last < 0 {
		return body
	}
	if strings.TrimSpace(arr[last].Get("reasoning_content").String()) != "" {
		return body
	}
	next, err := sjson.SetBytes(body, fmt.Sprintf("messages.%d.reasoning_content", last), cached)
	if err != nil {
		return body
	}
	return next
}

func NormalizeChatStreamDelta(info *RelayInfo, data string) string {
	if data == "" || !IsDeepSeekReasoningRelay(info, infoOriginModel(info)) {
		return data
	}
	if strings.TrimSpace(gjson.Get(data, "choices.0.delta.reasoning_content").String()) != "" {
		return data
	}
	alt := gjson.Get(data, "choices.0.delta.reasoning")
	if alt.Type != gjson.String || strings.TrimSpace(alt.String()) == "" {
		return data
	}
	next, err := sjson.Set(data, "choices.0.delta.reasoning_content", alt.String())
	if err != nil {
		return data
	}
	return next
}

func AppendDeepSeekChatReasoning(buf *strings.Builder, data string) {
	if buf == nil || data == "" {
		return
	}
	for _, choice := range gjson.Get(data, "choices").Array() {
		text := strings.TrimSpace(choice.Get("delta.reasoning_content").String())
		if text == "" {
			text = strings.TrimSpace(choice.Get("delta.reasoning").String())
		}
		if text == "" {
			text = strings.TrimSpace(choice.Get("message.reasoning_content").String())
		}
		if text == "" {
			text = strings.TrimSpace(choice.Get("message.reasoning").String())
		}
		if text != "" {
			buf.WriteString(text)
		}
	}
}

func infoOriginModel(info *RelayInfo) string {
	if info == nil {
		return ""
	}
	return info.OriginModelName
}

func chatReasoningText(msg gjson.Result) string {
	if text := strings.TrimSpace(msg.Get("reasoning").String()); text != "" && msg.Get("reasoning").Type == gjson.String {
		return text
	}
	return ""
}
