package deepseek

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

func shouldExpandDeepSeekHistory(prevInput, delta json.RawMessage) bool {
	prevUser := firstUserContent(prevInput)
	if prevUser == "" {
		return true
	}
	for _, item := range parseDeepSeekItems(delta) {
		if userContent(item) == prevUser {
			return false
		}
	}
	return true
}

func mergeDeepSeekHistory(prevInput, prevOutput, delta json.RawMessage) json.RawMessage {
	prevItems := parseDeepSeekItems(prevInput)
	outItems := parseDeepSeekItems(prevOutput)
	deltaItems := parseDeepSeekItems(delta)
	echoed := make(map[string]struct{})
	for _, item := range outItems {
		switch peekType(item) {
		case "function_call", "custom_tool_call":
			var peek deepSeekToolPairItem
			if common.Unmarshal(item, &peek) == nil && peek.CallID != "" {
				echoed[peek.CallID] = struct{}{}
			}
		}
	}
	kept := make([]json.RawMessage, 0, len(prevItems)+len(outItems)+len(deltaItems))
	kept = append(kept, prevItems...)
	kept = append(kept, outItems...)
	for _, item := range deltaItems {
		switch peekType(item) {
		case "function_call", "custom_tool_call":
			var peek deepSeekToolPairItem
			if common.Unmarshal(item, &peek) == nil && peek.CallID != "" {
				if _, ok := echoed[peek.CallID]; ok {
					continue
				}
			}
		}
		kept = append(kept, item)
	}
	raw, err := common.Marshal(kept)
	if err != nil {
		return delta
	}
	return raw
}

func parseDeepSeekItems(input json.RawMessage) []json.RawMessage {
	if len(input) == 0 {
		return nil
	}
	var items []json.RawMessage
	if common.Unmarshal(input, &items) != nil {
		return nil
	}
	return items
}

func firstUserContent(input json.RawMessage) string {
	for _, item := range parseDeepSeekItems(input) {
		if text := userContent(item); text != "" {
			return text
		}
	}
	return ""
}

func userContent(item json.RawMessage) string {
	if peekType(item) != "message" {
		return ""
	}
	var msg struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if common.Unmarshal(item, &msg) != nil || msg.Role != "user" {
		return ""
	}
	content := strings.TrimSpace(string(msg.Content))
	if len(msg.Content) > 0 && msg.Content[0] == '"' {
		var s string
		if common.Unmarshal(msg.Content, &s) == nil {
			return s
		}
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if common.Unmarshal(msg.Content, &parts) == nil {
		var b strings.Builder
		for _, part := range parts {
			b.WriteString(part.Text)
		}
		return b.String()
	}
	return content
}
