package deepseek

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

// zen 上游（Console Go → DeepSeek）不认识 `agent_message`：既不报错也不当作用户轮，
// 整条丢掉。Codex 子代理的 NEW_TASK / MESSAGE 正文就在这个 item 里，丢掉以后：
//   - 请求里最后一条"上游看得见"的消息变成没有 reasoning 的 assistant message，
//     触发 "The `reasoning_text` in the thinking mode must be passed back to the API."；
//   - 子代理看不到任务，只能回一句"还没给我任务"。
//
// normalizeDeepSeekAgentMessages 把这类 item 改写成 user 轮，一次解决两件事。
// encrypted_content 字段实测装的是任务明文（2026-09-18 生产 dump 逐字节还原 + 真上游
// 直连复现），因此按 input_text 传；author / recipient / id 上游不认，丢掉。没有可读
// 文本的 agent_message 原样留下，不凭空造内容。
//
// 同类第二件：侧边会话往主会话投递消息时，Codex 会记一条没有 call_id 的
// function_call_output（主会话里没有配对的 send_message_to_thread 调用）。上游对这种
// 缺字段是硬失败而不是静默忽略（422 + input: missing field call_id），
// 且这条 item 进了历史就每轮重放，整条线程从此打不通。正文照旧改写成 user 轮。
func normalizeDeepSeekAgentMessages(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return input
	}
	var items []json.RawMessage
	if common.Unmarshal(input, &items) != nil {
		return input
	}
	changed := false
	out := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		if converted, ok := deepSeekAgentMessageAsUser(item); ok {
			changed = true
			out = append(out, converted)
			continue
		}
		if isDeepSeekToolOutput(item) && toolCallID(item) == "" {
			changed = true
			// 缺 call_id 的工具输出上游直接 422，改写不出正文时只能丢。
			if converted, ok := deepSeekCallLessToolOutputAsUser(item); ok {
				out = append(out, converted)
			}
			continue
		}
		out = append(out, item)
	}
	if !changed {
		return input
	}
	raw, err := common.Marshal(out)
	if err != nil {
		return input
	}
	return raw
}

type deepSeekInputTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type deepSeekUserMessage struct {
	Type    string                   `json:"type"`
	Role    string                   `json:"role"`
	Content []deepSeekInputTextBlock `json:"content"`
}

func deepSeekAgentMessageAsUser(item json.RawMessage) (json.RawMessage, bool) {
	if peekType(item) != "agent_message" {
		return nil, false
	}
	var parsed struct {
		Content []struct {
			Type             string `json:"type"`
			Text             string `json:"text"`
			EncryptedContent string `json:"encrypted_content"`
		} `json:"content"`
	}
	if common.Unmarshal(item, &parsed) != nil {
		return nil, false
	}
	blocks := make([]deepSeekInputTextBlock, 0, len(parsed.Content))
	for _, block := range parsed.Content {
		text := ""
		switch block.Type {
		case "input_text", "output_text":
			text = block.Text
		case "encrypted_content":
			// 真变成密文时字段里会出现替换字符，此时宁可不传，也不往 prompt 里灌噪声。
			if strings.ContainsRune(block.EncryptedContent, utf8.RuneError) {
				continue
			}
			text = block.EncryptedContent
		default:
			continue
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		blocks = append(blocks, deepSeekInputTextBlock{Type: "input_text", Text: text})
	}
	if len(blocks) == 0 {
		return nil, false
	}
	raw, err := common.Marshal(deepSeekUserMessage{Type: "message", Role: "user", Content: blocks})
	if err != nil {
		return nil, false
	}
	return raw, true
}

// 缺 call_id 的工具输出（2026-09-18 23:01 生产 422 现场，渠道 Console Go）：正文是别的
// 会话投递进来的内容，改写成 user 轮保住，不再让上游按缺字段拒收整条请求。
func deepSeekCallLessToolOutputAsUser(item json.RawMessage) (json.RawMessage, bool) {
	var parsed struct {
		Output string `json:"output"`
	}
	// ponytail: 只认字符串正文；Responses 规格允许的 content parts 数组形态当没有正文丢掉，
	// 真在流量里见到再补解析。
	if common.Unmarshal(item, &parsed) != nil || strings.TrimSpace(parsed.Output) == "" {
		return nil, false
	}
	raw, err := common.Marshal(deepSeekUserMessage{Type: "message", Role: "user",
		Content: []deepSeekInputTextBlock{{Type: "input_text", Text: parsed.Output}}})
	if err != nil {
		return nil, false
	}
	return raw, true
}
