package deepseek

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func injectCachedDeepSeekReasoning(info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest) {
	if request == nil || !relaycommon.IsDeepSeekReasoningRelay(info, request.Model) {
		return
	}
	cached := relaycommon.LoadDeepSeekReasoning(info, request.PreviousResponseID)
	request.Input = rewriteDeepSeekReasoningInput(request.Input, cached)
}

func fillEmptyDeepSeekReasoningInPlace(input json.RawMessage, cached string) json.RawMessage {
	if len(input) == 0 || strings.TrimSpace(cached) == "" {
		return input
	}
	var items []json.RawMessage
	if common.Unmarshal(input, &items) != nil {
		return input
	}
	changed := false
	for i, item := range items {
		if peekType(item) != "reasoning" || reasoningItemHasText(item) {
			continue
		}
		if filled := reasoningItemJSON(cached); len(filled) > 0 {
			items[i] = filled
			changed = true
			break
		}
	}
	if !changed {
		return input
	}
	raw, err := common.Marshal(items)
	if err != nil {
		return input
	}
	return raw
}

func rewriteDeepSeekReasoningInput(input json.RawMessage, cached string) json.RawMessage {
	if len(input) == 0 {
		return input
	}
	var items []json.RawMessage
	if common.Unmarshal(input, &items) != nil {
		return input
	}
	changed := false
	hadCache := cached != ""
	kept := make([]json.RawMessage, 0, len(items)+1)
	for _, item := range items {
		if peekType(item) != "reasoning" {
			kept = append(kept, item)
			continue
		}
		if reasoningItemHasText(item) {
			kept = append(kept, item)
			continue
		}
		if cached != "" {
			if filled := reasoningItemJSON(cached); len(filled) > 0 {
				kept = append(kept, filled)
				changed = true
				cached = ""
				continue
			}
		}
		if hadCache {
			changed = true
			continue
		}
		kept = append(kept, item)
	}
	if regrouped, ok := regroupDeepSeekToolRuns(kept); ok {
		kept = regrouped
		changed = true
	}
	if cached != "" {
		if insertAt, ok := lastToolTurnMissingReasoning(kept); ok {
			if filled := reasoningItemJSON(cached); len(filled) > 0 {
				next := make([]json.RawMessage, 0, len(kept)+1)
				next = append(next, kept[:insertAt]...)
				next = append(next, filled)
				next = append(next, kept[insertAt:]...)
				kept = next
				changed = true
			}
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

func regroupDeepSeekToolRuns(items []json.RawMessage) ([]json.RawMessage, bool) {
	out := make([]json.RawMessage, 0, len(items))
	changed := false
	i := 0
	for i < len(items) {
		if !isDeepSeekToolCall(items[i]) && !isDeepSeekToolOutput(items[i]) {
			out = append(out, items[i])
			i++
			continue
		}
		j := i
		for j < len(items) && (isDeepSeekToolCall(items[j]) || isDeepSeekToolOutput(items[j])) {
			j++
		}
		run := items[i:j]
		calls := make([]json.RawMessage, 0, len(run))
		outs := make([]json.RawMessage, 0, len(run))
		seenOut := false
		grouped := true
		for _, item := range run {
			if isDeepSeekToolOutput(item) {
				seenOut = true
				outs = append(outs, item)
				continue
			}
			if seenOut {
				grouped = false
			}
			calls = append(calls, item)
		}
		if grouped {
			out = append(out, run...)
		} else {
			out = append(out, calls...)
			out = append(out, outs...)
			changed = true
		}
		i = j
	}
	if !changed {
		return items, false
	}
	return out, true
}

func lastToolTurnMissingReasoning(items []json.RawMessage) (int, bool) {
	lastOut := -1
	for i := len(items) - 1; i >= 0; i-- {
		if isDeepSeekToolOutput(items[i]) {
			lastOut = i
			break
		}
	}
	if lastOut < 0 {
		return 0, false
	}
	startOut := lastOut
	for startOut > 0 && isDeepSeekToolOutput(items[startOut-1]) {
		startOut--
	}
	insertAt := startOut
	if startOut > 0 && isDeepSeekToolCall(items[startOut-1]) {
		insertAt = startOut - 1
		for insertAt > 0 && isDeepSeekToolCall(items[insertAt-1]) {
			insertAt--
		}
	}
	if insertAt > 0 && reasoningItemHasText(items[insertAt-1]) {
		return 0, false
	}
	return insertAt, true
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
	var parsed map[string]any
	if common.Unmarshal(item, &parsed) != nil || peekType(item) != "reasoning" {
		return false
	}
	content, _ := parsed["content"].([]any)
	for _, part := range content {
		fields, _ := part.(map[string]any)
		if strings.TrimSpace(asString(fields["type"])) != "reasoning_text" {
			continue
		}
		if strings.TrimSpace(asString(fields["text"])) != "" {
			return true
		}
	}
	return false
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
