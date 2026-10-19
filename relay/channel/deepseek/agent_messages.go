package deepseek

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
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

// NormalizeResponsesInput applies the whole DeepSeek /responses item contract to a
// raw input array: agent_message 与缺 call_id 的工具输出先改写成 user 轮，丢掉无法
// 回放的 reasoning item，再把工具轮规范化（reasoning/assistant 等提到 calls 前，
// developer/user 通知移到 outputs 后，丢掉孤立 output）。
//
// 规范化不能只挂在 DeepSeek adaptor 的 ConvertOpenAIResponsesRequest 上：只要某条
// 入口没走到它（渠道类型委托、直通路由、宿主工具转换各不相同），客户端插在
// call 与它自己的 output 之间的 developer message 就会原样发出去，上游整轮回
// "No tool output found for tool call ..."。因此所有会打到 DeepSeek /responses 上游的
// 入口都要先过这一层；本函数幂等，adaptor 再跑一次不会二次改动。
func NormalizeResponsesInput(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return input
	}
	return canonicalizeDeepSeekToolRuns(
		dropUnreplayableDeepSeekReasoning(
			makeDeepSeekCallIDsPortable(normalizeDeepSeekAgentMessages(input)),
		),
	)
}

const (
	deepSeekPortableCallIDPrefix  = "call_00_ET_"
	deepSeekPortableCallIDHashLen = 24
)

// native responses 上游签发的 call_id 形态：call_<两位>_<24 位字母数字>（实测长度 32）。
// chat 网关签发的是 call_00_ET_<24 位>，下划线不属于该字符集，不会误判。
var deepSeekNativeCallIDPattern = regexp.MustCompile(`^call_\d{2}_[A-Za-z0-9]{24}$`)

// makeDeepSeekCallIDsPortable 把上游签发形态的 call_id 确定性地映射到外部命名空间
// （call_00_ET_+hash），使任意账号、任意类渠道都能接续同一份历史。
//
// 背景：native responses 上游只认可自己账号命名空间签发的 call_id。历史里别的账号/网关
// 签发的同形 id 会被它当成"外来调用"，要求客户端回传 reasoning_text，否则整轮 400
// "The `reasoning_text` in the thinking mode must be passed back to the API."；这就是
// 2026-10-19 16:33 事故里 chat→native fallback 失败的真实触发点（形状归一化单独不够）。
// 上游对 call_00_ET_ 形态的外部 id 不做归属校验（生产 payload 实测：只改 id 即 200）。
//
// 说明：仓库 DeepSeek 规则禁止改写客户端 item；本改写是 owner 2026-10-19 明确批准的
// 例外，只动协议内部 call_id、不改任何正文内容，且是跨类 fallback 可达的必要条件。
// 映射对同一个 id 恒定，前缀缓存跨轮稳定；已经是外部形态的 id 不动，因此本函数幂等。
func makeDeepSeekCallIDsPortable(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return input
	}
	var items []json.RawMessage
	if common.Unmarshal(input, &items) != nil {
		return input
	}
	changed := false
	for i, item := range items {
		var fields map[string]json.RawMessage
		if common.Unmarshal(item, &fields) != nil {
			continue
		}
		rawCallID, ok := fields["call_id"]
		if !ok {
			continue
		}
		var callID string
		if common.Unmarshal(rawCallID, &callID) != nil {
			continue
		}
		portable := portableDeepSeekCallID(callID)
		if portable == callID {
			continue
		}
		encoded, err := common.Marshal(portable)
		if err != nil {
			continue
		}
		fields["call_id"] = encoded
		rebuilt, err := common.Marshal(fields)
		if err != nil {
			continue
		}
		items[i] = rebuilt
		changed = true
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

func portableDeepSeekCallID(callID string) string {
	if !deepSeekNativeCallIDPattern.MatchString(callID) {
		return callID
	}
	sum := sha256.Sum256([]byte("ai-cove:" + callID))
	return deepSeekPortableCallIDPrefix + hex.EncodeToString(sum[:])[:deepSeekPortableCallIDHashLen]
}

// chat completions → responses 转换产出的 reasoning item 只有 summary、content 为空、
// 也没有 encrypted_content；原生 DeepSeek /responses 上游要求 thinking 模式回传
// reasoning_text，这种 item 原样发出会被 400 "The `reasoning_text` in the thinking mode
// must be passed back to the API."（2026-10-19 16:33 生产事故：chat 类渠道故障后
// fallback 到原生类渠道被整轮拒绝）。上游接受没有 reasoning 的工具轮，而补/改
// reasoning_text 属于改写客户端内容，禁止；因此这类不可回放的 item 只能丢弃。
//
// 必须放在工具轮规整之前：丢掉 reasoning 后相邻的 call/output 需要按上游要求重新成组。
func dropUnreplayableDeepSeekReasoning(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return input
	}
	var items []json.RawMessage
	if common.Unmarshal(input, &items) != nil {
		return input
	}
	kept := make([]json.RawMessage, 0, len(items))
	changed := false
	for _, item := range items {
		if peekType(item) == "reasoning" && !deepSeekReasoningHasPassback(item) {
			changed = true
			continue
		}
		kept = append(kept, item)
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

// deepSeekReasoningHasPassback 判断 reasoning item 是否带有可回放材料：非空
// reasoning_text content 或 encrypted_content。空文本的 reasoning_text 仍保留——既有的
// Compact 续轮形状上游实测接受；解析失败时保守保留，不做丢弃。
//
// 不要复用 relay/common 的 DeepSeekToolReasoningDiagnostics.hasReasoningText：那边把
// summary 文本当作"有内容"用于报告客户端形状；这里判断的是上游能否校验回放，summary
// 不算可回放材料（chat 转换产物就是只有 summary 的 reasoning）。
func deepSeekReasoningHasPassback(item json.RawMessage) bool {
	var parsed struct {
		Content          json.RawMessage `json:"content"`
		EncryptedContent string          `json:"encrypted_content"`
	}
	if common.Unmarshal(item, &parsed) != nil {
		return true
	}
	if strings.TrimSpace(parsed.EncryptedContent) != "" {
		return true
	}
	if len(parsed.Content) == 0 || string(parsed.Content) == "null" {
		return false
	}
	var parts []deepSeekInputTextBlock
	if common.Unmarshal(parsed.Content, &parts) != nil {
		return true
	}
	for _, part := range parts {
		if part.Type == "reasoning_text" {
			return true
		}
	}
	return false
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
