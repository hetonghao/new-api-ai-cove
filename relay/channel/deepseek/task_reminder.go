package deepseek

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

const openCodeTaskPrefix = "当前用户任务："

func RememberOpenCodeUserTask(info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest) {
	if request == nil || !relaycommon.IsDeepSeekReasoningRelay(info, request.Model) {
		return
	}
	key := openCodeSessionKey(info, request)
	if text := lastResponsesUserText(request.Input); text != "" && key != "" {
		relaycommon.SaveOpenCodeUserTask(key, text)
	}
	if lastResponsesRole(request.Input) == "user" {
		return
	}
	if key == "" {
		return
	}
	task := relaycommon.LoadOpenCodeUserTask(key)
	if task == "" {
		return
	}
	request.Input = appendOpenCodeTaskReminder(request.Input, task)
}

func openCodeSessionKey(info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest) string {
	if request != nil {
		if s := strings.Trim(strings.TrimSpace(string(request.PromptCacheKey)), `"`); s != "" && s != "null" {
			return s
		}
	}
	if info == nil {
		return ""
	}
	for _, name := range []string{"session-id", "session_id", "x-opencode-session"} {
		if v := strings.TrimSpace(info.RequestHeaders[name]); v != "" {
			return v
		}
	}
	return ""
}

func lastResponsesUserText(input json.RawMessage) string {
	items := peekInputItems(input)
	for i := len(items) - 1; i >= 0; i-- {
		if peekType(items[i]) != "message" {
			continue
		}
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if common.Unmarshal(items[i], &msg) != nil || msg.Role != "user" {
			continue
		}
		return truncateRunes(responsesContentText(msg.Content), 8000)
	}
	return ""
}

func lastResponsesRole(input json.RawMessage) string {
	items := peekInputItems(input)
	for i := len(items) - 1; i >= 0; i-- {
		if peekType(items[i]) != "message" {
			continue
		}
		var msg struct {
			Role string `json:"role"`
		}
		if common.Unmarshal(items[i], &msg) != nil {
			return ""
		}
		return msg.Role
	}
	return ""
}

func responsesContentText(content json.RawMessage) string {
	content = bytesTrimSpace(content)
	if len(content) == 0 {
		return ""
	}
	if content[0] == '"' {
		var s string
		if common.Unmarshal(content, &s) == nil {
			return strings.TrimSpace(s)
		}
	}
	var parts []map[string]any
	if common.Unmarshal(content, &parts) != nil {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		if strings.TrimSpace(asString(part["type"])) != "input_text" && strings.TrimSpace(asString(part["type"])) != "text" {
			continue
		}
		if t := strings.TrimSpace(asString(part["text"])); t != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(t)
		}
	}
	return b.String()
}

func bytesTrimSpace(raw json.RawMessage) json.RawMessage {
	return json.RawMessage(strings.TrimSpace(string(raw)))
}

func peekInputItems(input json.RawMessage) []json.RawMessage {
	if len(input) == 0 {
		return nil
	}
	var items []json.RawMessage
	if common.Unmarshal(input, &items) != nil {
		return nil
	}
	return items
}

func appendOpenCodeTaskReminder(input json.RawMessage, task string) json.RawMessage {
	task = strings.TrimSpace(task)
	if task == "" {
		return input
	}
	items := peekInputItems(input)
	if lastResponsesRole(input) == "developer" {
		if strings.Contains(string(items[len(items)-1]), openCodeTaskPrefix) {
			return input
		}
	}
	hint, err := common.Marshal(map[string]any{
		"type": "message",
		"role": "developer",
		"content": []map[string]string{{
			"type": "input_text",
			"text": openCodeTaskPrefix + "\n" + task,
		}},
	})
	if err != nil {
		return input
	}
	items = append(items, hint)
	raw, err := common.Marshal(items)
	if err != nil {
		return input
	}
	return raw
}

func truncateRunes(s string, max int) string {
	if max <= 0 || s == "" {
		return s
	}
	n := 0
	for i := range s {
		if n == max {
			return s[:i]
		}
		n++
	}
	return s
}
