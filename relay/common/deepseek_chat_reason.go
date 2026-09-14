package common

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type OpenCodeSessionModel int

const (
	OpenCodeSessionUnknown OpenCodeSessionModel = iota
	OpenCodeSessionDeepSeek
	OpenCodeSessionOther
)

func PrepareChatCompletionsBody(info *RelayInfo, body []byte) []byte {
	if len(body) == 0 || !IsDeepSeekReasoningRelay(info, infoOriginModel(info)) {
		return body
	}
	bindOpenCodeSession(info, body)
	cached, last := LoadOpenCodeChatThinking(info, body)
	out := ApplyChatOpenCodeThinking(body, cached, last)
	WriteOpenCodeSessionModel(info)
	return out
}

func ApplyChatOpenCodeThinking(body []byte, cached string, last OpenCodeSessionModel) []byte {
	body = NormalizeChatCompletionsReasoning(body)
	body = copyEarlierAssistantReasoningToToolTurns(body)
	if chatToolAssistantMissingReasoning(body) && last == OpenCodeSessionDeepSeek {
		body = fillToolAssistantReasoning(body, cached)
	}
	if last == OpenCodeSessionOther && chatHasToolCalls(body) && !chatHasReasoningContent(body) {
		return flattenChatToolTurns(body)
	}
	return body
}

func copyEarlierAssistantReasoningToToolTurns(body []byte) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}
	lastReasoning := ""
	result := body
	for i, msg := range messages.Array() {
		if msg.Get("role").String() != "assistant" {
			continue
		}
		if text := strings.TrimSpace(msg.Get("reasoning_content").String()); text != "" {
			lastReasoning = text
			continue
		}
		if lastReasoning == "" || !chatMsgHasToolCalls(msg) {
			continue
		}
		next, err := sjson.SetBytes(result, fmt.Sprintf("messages.%d.reasoning_content", i), lastReasoning)
		if err != nil {
			return result
		}
		result = next
	}
	return result
}

func fillToolAssistantReasoning(body []byte, cached string) []byte {
	cached = strings.TrimSpace(cached)
	if cached == "" {
		return body
	}
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}
	result := body
	arr := messages.Array()
	for i, msg := range arr {
		if msg.Get("role").String() != "assistant" || !chatMsgHasToolCalls(msg) {
			continue
		}
		if strings.TrimSpace(msg.Get("reasoning_content").String()) != "" {
			continue
		}
		next, err := sjson.SetBytes(result, fmt.Sprintf("messages.%d.reasoning_content", i), cached)
		if err != nil {
			return result
		}
		result = next
	}
	return result
}

func flattenChatToolTurns(body []byte) []byte {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}
	arr := messages.Array()
	out := make([]any, 0, len(arr))
	for i := 0; i < len(arr); {
		msg := arr[i]
		if msg.Get("role").String() != "assistant" || !chatMsgHasToolCalls(msg) {
			if msg.Get("role").String() == "tool" {
				i++
				continue
			}
			var raw any
			if err := json.Unmarshal([]byte(msg.Raw), &raw); err != nil {
				return body
			}
			out = append(out, raw)
			i++
			continue
		}
		var b strings.Builder
		if text := chatMsgContent(msg); text != "" {
			b.WriteString(text)
		}
		for _, tc := range msg.Get("tool_calls").Array() {
			name := tc.Get("function.name").String()
			if name == "" {
				name = tc.Get("name").String()
			}
			args := tc.Get("function.arguments").String()
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString("调用 ")
			b.WriteString(name)
			if args != "" {
				b.WriteByte('(')
				b.WriteString(args)
				b.WriteByte(')')
			}
		}
		i++
		for i < len(arr) && arr[i].Get("role").String() == "tool" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString("结果：")
			b.WriteString(arr[i].Get("content").String())
			i++
		}
		out = append(out, map[string]any{
			"role":    "assistant",
			"content": strings.TrimSpace(b.String()),
		})
	}
	next, err := sjson.SetBytes(body, "messages", out)
	if err != nil {
		return body
	}
	return next
}

func chatHasToolCalls(body []byte) bool {
	for _, msg := range gjson.GetBytes(body, "messages").Array() {
		if chatMsgHasToolCalls(msg) {
			return true
		}
	}
	return false
}

func chatHasReasoningContent(body []byte) bool {
	for _, msg := range gjson.GetBytes(body, "messages").Array() {
		if strings.TrimSpace(msg.Get("reasoning_content").String()) != "" {
			return true
		}
	}
	return false
}

func chatToolAssistantMissingReasoning(body []byte) bool {
	for _, msg := range gjson.GetBytes(body, "messages").Array() {
		if msg.Get("role").String() == "assistant" && chatMsgHasToolCalls(msg) && strings.TrimSpace(msg.Get("reasoning_content").String()) == "" {
			return true
		}
	}
	return false
}

func chatMsgHasToolCalls(msg gjson.Result) bool {
	calls := msg.Get("tool_calls")
	return calls.Exists() && calls.IsArray() && len(calls.Array()) > 0
}

func chatMsgContent(msg gjson.Result) string {
	c := msg.Get("content")
	if c.Type == gjson.String {
		return strings.TrimSpace(c.String())
	}
	return strings.TrimSpace(c.Raw)
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
