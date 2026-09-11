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

func rewriteDeepSeekReasoningInput(input json.RawMessage, cached string) json.RawMessage {
	if len(input) == 0 {
		return input
	}
	var items []json.RawMessage
	if common.Unmarshal(input, &items) != nil {
		return input
	}
	changed := false
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
		if cached == "" {
			changed = true
			continue
		}
		if filled := reasoningItemJSON(cached); len(filled) > 0 {
			kept = append(kept, filled)
			changed = true
			cached = ""
			continue
		}
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

func lastToolTurnMissingReasoning(items []json.RawMessage) (int, bool) {
	outputIdx := -1
	for i := len(items) - 1; i >= 0; i-- {
		switch peekType(items[i]) {
		case "function_call_output", "custom_tool_call_output":
			outputIdx = i
		}
		if outputIdx >= 0 {
			break
		}
	}
	insertAt := -1
	if outputIdx >= 0 {
		callID := peekCallID(items[outputIdx])
		if callID != "" {
			for i := outputIdx - 1; i >= 0; i-- {
				if (peekType(items[i]) == "function_call" || peekType(items[i]) == "custom_tool_call") &&
					peekCallID(items[i]) == callID {
					insertAt = i
					break
				}
			}
		}
		if insertAt < 0 {
			insertAt = outputIdx
		}
	} else {
		for i := len(items) - 1; i >= 0; i-- {
			switch peekType(items[i]) {
			case "function_call", "custom_tool_call":
				insertAt = i
			}
			if insertAt >= 0 {
				break
			}
		}
	}
	if insertAt < 0 {
		return 0, false
	}
	if insertAt > 0 && reasoningItemHasText(items[insertAt-1]) {
		return 0, false
	}
	return insertAt, true
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

func peekCallID(item json.RawMessage) string {
	var peek struct {
		CallID string `json:"call_id"`
	}
	_ = common.Unmarshal(item, &peek)
	return peek.CallID
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}
