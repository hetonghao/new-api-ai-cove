package deepseek

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func FillMissingOpenCodeReasoning(info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest) {
	if request == nil || !relaycommon.IsDeepSeekReasoningRelay(info, request.Model) {
		return
	}
	// OpenCode thinking rejects fc/fco groups without a preceding reasoning_text,
	// and rejects reasoning sitting between an unmatched call and its output.
	cached := relaycommon.LoadDeepSeekReasoningByResponseID(request.PreviousResponseID)
	request.Input = rewriteDeepSeekReasoningInput(request.Input, cached)
}

func injectCachedDeepSeekReasoning(info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest) {
	if request == nil || !relaycommon.IsDeepSeekReasoningRelay(info, request.Model) {
		return
	}
	cached := relaycommon.LoadDeepSeekReasoning(info, request.PreviousResponseID)
	request.Input = rewriteDeepSeekReasoningInput(request.Input, cached)
}

// rewriteDeepSeekReasoningInput keeps one invariant:
// every function_call group is immediately preceded by reasoning_text,
// and reasoning never sits between an unmatched function_call and its output.
func rewriteDeepSeekReasoningInput(input json.RawMessage, cached string) json.RawMessage {
	if len(input) == 0 {
		return input
	}
	var items []json.RawMessage
	if common.Unmarshal(input, &items) != nil {
		return input
	}
	changed := false
	insertText := cached
	lastSeenText := ""
	kept := make([]json.RawMessage, 0, len(items)+2)
	for i, item := range items {
		if peekType(item) != "reasoning" {
			kept = append(kept, item)
			continue
		}
		if reasoningInsideOpenToolTurn(items, i) {
			if text := reasoningItemText(item); text != "" {
				insertText = text
				lastSeenText = text
			}
			changed = true
			continue
		}
		if text := reasoningItemText(item); text != "" {
			kept = append(kept, item)
			lastSeenText = text
			continue
		}
		fillFrom := cached
		if fillFrom == "" {
			fillFrom = lastSeenText
		}
		if fillFrom != "" {
			if filled := fillReasoningItem(item, fillFrom); len(filled) > 0 {
				kept = append(kept, filled)
				changed = true
				continue
			}
		}
		kept = append(kept, item)
	}
	if insertText == "" {
		insertText = lastSeenText
	}
	if insertText != "" {
		if next, ok := ensureReasoningBeforeEveryToolTurn(kept, insertText); ok {
			kept = next
			changed = true
		}
		if next, ok := ensureReasoningBeforeOutputOnlyTurn(kept, insertText); ok {
			kept = next
			changed = true
		}
	}
	if !changed {
		return input
	}
	raw, err := common.Marshal(kept)
	if err != nil {
		return input
	}
	return raw
}

func ensureReasoningBeforeEveryToolTurn(items []json.RawMessage, text string) ([]json.RawMessage, bool) {
	filled := reasoningItemJSON(text)
	if len(filled) == 0 {
		return items, false
	}
	out := make([]json.RawMessage, 0, len(items)+2)
	changed := false
	i := 0
	for i < len(items) {
		if !isDeepSeekToolCall(items[i]) {
			out = append(out, items[i])
			i++
			continue
		}
		if len(out) == 0 || !reasoningItemHasText(out[len(out)-1]) {
			out = append(out, filled)
			changed = true
		}
		for i < len(items) && isDeepSeekToolCall(items[i]) {
			out = append(out, items[i])
			i++
		}
	}
	if !changed {
		return items, false
	}
	return out, true
}

func ensureReasoningBeforeOutputOnlyTurn(items []json.RawMessage, text string) ([]json.RawMessage, bool) {
	filled := reasoningItemJSON(text)
	if len(filled) == 0 {
		return items, false
	}
	hasCall := false
	firstOut := -1
	for i, item := range items {
		if isDeepSeekToolCall(item) {
			hasCall = true
			break
		}
		if firstOut < 0 && isDeepSeekToolOutput(item) {
			firstOut = i
		}
	}
	if hasCall || firstOut < 0 {
		return items, false
	}
	if firstOut > 0 && reasoningItemHasText(items[firstOut-1]) {
		return items, false
	}
	next := make([]json.RawMessage, 0, len(items)+1)
	next = append(next, items[:firstOut]...)
	next = append(next, filled)
	next = append(next, items[firstOut:]...)
	return next, true
}

func reasoningInsideOpenToolTurn(items []json.RawMessage, idx int) bool {
	open := 0
	for i := 0; i < idx; i++ {
		switch peekType(items[i]) {
		case "function_call", "custom_tool_call":
			open++
		case "function_call_output", "custom_tool_call_output":
			if open > 0 {
				open--
			}
		}
	}
	return open > 0
}

func isDeepSeekToolCall(item json.RawMessage) bool {
	switch peekType(item) {
	case "function_call", "custom_tool_call":
		return true
	default:
		return false
	}
}

func isDeepSeekToolOutput(item json.RawMessage) bool {
	switch peekType(item) {
	case "function_call_output", "custom_tool_call_output":
		return true
	default:
		return false
	}
}

func reasoningItemHasText(item json.RawMessage) bool {
	return reasoningItemText(item) != ""
}

func reasoningItemText(item json.RawMessage) string {
	var parsed map[string]any
	if common.Unmarshal(item, &parsed) != nil || peekType(item) != "reasoning" {
		return ""
	}
	content, _ := parsed["content"].([]any)
	for _, part := range content {
		fields, _ := part.(map[string]any)
		if strings.TrimSpace(asString(fields["type"])) != "reasoning_text" {
			continue
		}
		if text := strings.TrimSpace(asString(fields["text"])); text != "" {
			return text
		}
	}
	return ""
}

func fillReasoningItem(item json.RawMessage, text string) json.RawMessage {
	var parsed map[string]any
	if common.Unmarshal(item, &parsed) != nil {
		return reasoningItemJSON(text)
	}
	parsed["type"] = "reasoning"
	parsed["content"] = []map[string]string{{
		"type": "reasoning_text",
		"text": text,
	}}
	// ponytail: Codex stubs encrypted_content with a local id; DeepSeek wants reasoning_text, not that blob.
	delete(parsed, "encrypted_content")
	raw, err := common.Marshal(parsed)
	if err != nil {
		return reasoningItemJSON(text)
	}
	return raw
}

func reasoningItemJSON(text string) json.RawMessage {
	raw, err := common.Marshal(map[string]any{
		"type": "reasoning",
		"content": []map[string]string{{
			"type": "reasoning_text",
			"text": text,
		}},
	})
	if err != nil {
		return nil
	}
	return raw
}

func peekType(item json.RawMessage) string {
	var peek struct {
		Type string `json:"type"`
	}
	_ = common.Unmarshal(item, &peek)
	return peek.Type
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
