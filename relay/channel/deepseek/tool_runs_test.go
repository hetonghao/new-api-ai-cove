package deepseek

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 客户端把并行调用和结果交错着发回来（call/out/call/out）：上游只接受同一轮
// 调用在前、结果在后，这里只重排，不新增、不改写任何 item。
func TestCanonicalizeDeepSeekToolRunsRegroupsInterleavedToolPairs(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "get_a", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "A"},
		{"type": "function_call", "call_id": "call-2", "name": "get_b", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-2", "output": "B"},
	})

	got := canonicalizeDeepSeekToolRuns(input)
	require.Equal(t, []string{"function_call", "function_call", "function_call_output", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "call-1", gjson.GetBytes(got, "0.call_id").String())
	require.Equal(t, "call-2", gjson.GetBytes(got, "1.call_id").String())
	require.Equal(t, "A", gjson.GetBytes(got, "2.output").String())
	require.Equal(t, "B", gjson.GetBytes(got, "3.output").String())
}

func TestCanonicalizeDeepSeekToolRunsDoesNotSplitParallelToolCalls(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "a"},
		{"type": "function_call_output", "call_id": "call-2", "output": "b"},
	})

	require.Equal(t, string(input), string(canonicalizeDeepSeekToolRuns(input)))
}

// 调用没有配对结果时把 call 提到 outputs 上方会让它变成落单调用，上游直接拒，
// 所以这种轮次保持客户端原样的字节。
func TestCanonicalizeDeepSeekToolRunsKeepsRunWithUnpairedCallInOrder(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "a"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
	})

	require.Equal(t, string(input), string(canonicalizeDeepSeekToolRuns(input)))
}

// 已经合法的 payload 一个字节都不能动：上游按前缀缓存计费，改写前缀会把整段
// prompt cache 作废（生产上出现过 800k 上下文每轮只命中 62k）。
func TestCanonicalizeDeepSeekToolRunsLeavesValidPayloadByteIdentical(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": "看看磁盘"},
		{"type": "reasoning", "id": "rs_1", "summary": []any{}, "content": []map[string]any{{"type": "reasoning_text", "text": "先跑 df"}}},
		{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "开始扫描"}}},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "a"},
		{"type": "function_call_output", "call_id": "call-2", "output": "b"},
	})

	require.Equal(t, string(input), string(canonicalizeDeepSeekToolRuns(input)))
}

// 客户端没带 reasoning 的工具轮直接透传：上游对这种形状返回 200，而补一条
// reasoning 会造出 `assistant message → reasoning`，被判 400。
func TestCanonicalizeDeepSeekToolRunsKeepsToolRunWithoutReasoning(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "我来扫一遍"}}},
		{"type": "function_call", "call_id": "call-1", "name": "bash", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})

	got := canonicalizeDeepSeekToolRuns(input)
	require.Equal(t, string(input), string(got))
	require.NotContains(t, string(got), "reasoning")
}

// grok-4.6 开联网搜索时会把 web_search_call / reasoning 插进工具轮中间，
// 切到 deepseek 后既解析不了 web_search_call，也过不了工具轮配对校验。
func TestCanonicalizeDeepSeekToolRunsDropsForeignSearchCall(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "summary": []any{}, "content": nil, "encrypted_content": "tco_0"},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "web_search_call", "status": "completed", "action": map[string]any{"type": "search", "query": "site:github.com/openai/codex"}},
		{"type": "reasoning", "summary": []any{}, "content": nil, "encrypted_content": "tco_1"},
		{"type": "message", "role": "assistant", "content": []map[string]string{{"type": "output_text", "text": "查到了"}}},
		{"type": "function_call_output", "call_id": "call-1", "output": "a"},
	})

	got := canonicalizeDeepSeekToolRuns(input)
	require.Equal(t, []string{
		"reasoning",
		"reasoning",
		"message",
		"function_call",
		"function_call_output",
	}, inputTypes(t, got))
	require.NotContains(t, string(got), "web_search_call")
	require.Equal(t, "tco_0", gjson.GetBytes(got, "0.encrypted_content").String())
}

// 上游从不解析 previous_response_id 状态，孤立 output 会让整轮 400
// (No tool call found for tool output)，只能丢掉。
func TestCanonicalizeDeepSeekToolRunsDropsOrphanOutput(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		{"type": "function_call_output", "call_id": "call-orphan", "output": "stale result"},
		{"type": "function_call_output", "call_id": "call-2", "output": "kept"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
	})

	got := canonicalizeDeepSeekToolRuns(input)
	require.Equal(t, []string{"function_call", "function_call", "function_call_output", "function_call_output"}, inputTypes(t, got))
	require.NotContains(t, string(got), "call-orphan")
	require.Contains(t, string(got), "kept")
	require.Equal(t, "call-2", gjson.GetBytes(got, "1.call_id").String())
	require.Equal(t, "kept", gjson.GetBytes(got, "3.output").String())
}

// 夹在两个并行调用中间的 reasoning / assistant message 会让上游报
// "No tool output found for tool call ..."，必须提升到该轮调用之前。
func TestCanonicalizeDeepSeekToolRunsHoistsTextBetweenParallelCalls(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "searching"}}},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "a"},
		{"type": "function_call_output", "call_id": "call-2", "output": "b"},
	})

	got := canonicalizeDeepSeekToolRuns(input)
	require.Equal(t, []string{
		"reasoning",
		"function_call",
		"function_call",
		"function_call_output",
		"function_call_output",
	}, inputTypes(t, got))
	require.Equal(t, "searching", gjson.GetBytes(got, "0.content.0.text").String())
}

// 共享入口：agent_message 改写与工具轮规整一次做完，并且幂等——relay/controller 两层
// 与 adaptor 都会调它，任一层漏掉就会出现 2026-09-21 那种“sent 与 client 完全一致”的漏网。
func TestNormalizeResponsesInputComposesAndIsIdempotent(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "看两张图"}}},
		{"type": "function_call", "call_id": "call-1", "name": "view_image", "arguments": "{}"},
		{"type": "message", "role": "developer", "content": []map[string]any{{"type": "input_text", "text": "<image_resize_notice>1 of 1 resized</image_resize_notice>"}}},
		{"type": "function_call", "call_id": "call-2", "name": "view_image", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": []map[string]any{{"type": "input_image", "image_url": "data:image/png;base64,iVBORw0KGgo="}}},
		{"type": "function_call_output", "call_id": "call-2", "output": []map[string]any{{"type": "input_image", "image_url": "data:image/png;base64,iVBORw0KGgo="}}},
	})

	once := NormalizeResponsesInput(input)
	require.Equal(t, []string{
		"message",
		"function_call",
		"function_call",
		"function_call_output",
		"function_call_output",
		"message",
	}, inputTypes(t, once))
	require.Equal(t, "developer", gjson.GetBytes(once, "5.role").String())
	require.Equal(t, 0, relaycommon.DeepSeekToolReasoningDiagnosticsOfInput(once).Interleaved)

	require.Equal(t, string(once), string(NormalizeResponsesInput(once)))
}

// chat completions → responses 转换产出的 reasoning item：只有 summary，content 为空、
// 也没有 encrypted_content。原生 DeepSeek /responses 上游要求 thinking 模式回传
// reasoning_text，这种 item 原样发出就是 2026-10-19 16:33 生产 400
// （chat 类渠道 500 后 fallback 到原生类渠道）。上游接受“没有 reasoning 的工具轮”，
// 所以只能丢弃它；补 reasoning_text 属于改写 item，禁止。
func TestNormalizeResponsesInputDropsChatConvertedReasoning(t *testing.T) {
	chatReasoning := func(id, text string) map[string]any {
		return map[string]any{
			"type":    "reasoning",
			"id":      id,
			"status":  "completed",
			"role":    "",
			"content": nil,
			"summary": []map[string]any{{"type": "summary_text", "text": text}},
			"quality": "",
			"size":    "",
		}
	}
	input := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "继续排查"}}},
		chatReasoning("chatcmpl-a_reasoning_0", "先看错误日志"),
		{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "我来查一下"}}},
		{"type": "function_call", "call_id": "call_00_first", "name": "bash", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call_00_first", "output": "ok"},
		chatReasoning("chatcmpl-b_reasoning_0", "继续第二轮"),
		{"type": "function_call", "call_id": "call_01_second", "name": "bash", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call_01_second", "output": "done"},
	})

	got := NormalizeResponsesInput(input)
	require.Equal(t, []string{
		"message",
		"message",
		"function_call",
		"function_call",
		"function_call_output",
		"function_call_output",
	}, inputTypes(t, got))
	require.NotContains(t, string(got), "_reasoning_0")
	require.NotContains(t, string(got), "summary_text")
	require.Equal(t, string(got), string(NormalizeResponsesInput(got)))
}

// 可回放的 reasoning（原生渠道产物）以及既有 Compact 续轮 / 跨模型 encrypted-only
// 形状必须逐字节保留：改写或丢弃都会改变上游已经缓存的前缀，encrypted-only 的
// 回放语义也还没有上游证据支持改动。
func TestNormalizeResponsesInputKeepsReplayableReasoning(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": "继续"},
		{"type": "reasoning", "id": "rs_1", "summary": []any{}, "content": []map[string]any{{"type": "reasoning_text", "text": "先看 df"}}, "encrypted_content": "opaque"},
		{"type": "function_call", "call_id": "call-1", "name": "bash", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		{"type": "reasoning", "summary": []any{}, "content": []map[string]any{{"type": "reasoning_text", "text": ""}}},
		{"type": "reasoning", "summary": []any{}, "content": nil, "encrypted_content": "tco_0"},
	})

	require.Equal(t, string(input), string(NormalizeResponsesInput(input)))
}

// native responses 上游只认自己账号命名空间签发的 call_id；另一个账号/类渠道签发、
// 原样重放的 call_00_+24 位 id 会被要求回传 reasoning_text（2026-10-19 16:33 生产
// 事故的真实触发点，chat→native fallback 整轮 400）。改写为确定性的外部命名空间
// call_00_ET_+hash 后，上游按外部 id 接受，且同一 id 每轮映射一致、幂等。
func TestNormalizeResponsesInputRewritesForeignNativeCallIDs(t *testing.T) {
	const (
		nativeCall1  = "call_00_cwBm5ITHjfMIIw3Drstn9324"     // 32 位，上游签发形态
		nativeCall2  = "call_01_xlz65RztUGxjV9glpllb7847"     // 并行调用的第二位也是签发形态
		externalCall = "call_00_ET_persistedExternalId000001" // 更长且含 ET_，已经是外部形态
	)
	input := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": "继续"},
		{"type": "function_call", "call_id": nativeCall1, "name": "bash", "arguments": "{}"},
		{"type": "function_call", "call_id": nativeCall2, "name": "bash", "arguments": "{}"},
		{"type": "function_call", "call_id": externalCall, "name": "bash", "arguments": "{}"},
		{"type": "function_call_output", "call_id": nativeCall1, "output": "a"},
		{"type": "function_call_output", "call_id": nativeCall2, "output": "b"},
		{"type": "function_call_output", "call_id": externalCall, "output": "c"},
	})

	got := NormalizeResponsesInput(input)
	items := gjson.ParseBytes(got).Array()
	require.Len(t, items, 7)
	require.Equal(t, []string{
		"message",
		"function_call",
		"function_call",
		"function_call",
		"function_call_output",
		"function_call_output",
		"function_call_output",
	}, inputTypes(t, got))

	portable1, portable2 := items[1].Get("call_id").String(), items[2].Get("call_id").String()
	require.True(t, strings.HasPrefix(portable1, deepSeekPortableCallIDPrefix))
	require.True(t, strings.HasPrefix(portable2, deepSeekPortableCallIDPrefix))
	require.NotEqual(t, nativeCall1, portable1)
	require.NotEqual(t, nativeCall2, portable2)
	require.NotEqual(t, portable1, portable2)
	require.Equal(t, portable1, items[4].Get("call_id").String(), "call 与 output 必须映射到同一个 id")
	require.Equal(t, portable2, items[5].Get("call_id").String())
	require.Equal(t, externalCall, items[3].Get("call_id").String(), "外部形态 id 不改写")
	require.Equal(t, externalCall, items[6].Get("call_id").String())

	require.Equal(t, string(got), string(NormalizeResponsesInput(input)), "同一输入每轮映射必须一致")
	require.Equal(t, string(got), string(NormalizeResponsesInput(got)), "规范化必须幂等")
}

// fallback 归一化必须是无损边界：只允许丢弃目标上游无法回放的 reasoning item、把上游
// 签发形态的 call_id 映射到外部命名空间；用户/助手文本、工具名、arguments、工具输出等
// 正文逐字段保持不变，call/output 成对映射，客户端传入的原始字节不被修改。
func TestNormalizeResponsesInputPreservesConversationContent(t *testing.T) {
	const (
		nativeCall = "call_00_cwBm5ITHjfMIIw3Drstn9324"
		shortCall  = "call_9"
	)
	imageOutput := []map[string]any{{"type": "input_image", "image_url": "data:image/png;base64,iVBORw0KGgo=", "detail": "high"}}
	input := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "排查告警"}}},
		{"type": "reasoning", "id": "chatcmpl-x_reasoning_0", "status": "completed", "role": "", "content": nil, "summary": []map[string]any{{"type": "summary_text", "text": "先看日志"}}},
		{"type": "reasoning", "id": "rs_1", "summary": []any{}, "content": []map[string]any{{"type": "reasoning_text", "text": "先看 df"}}, "encrypted_content": "opaque"},
		{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "开始扫描"}}},
		{"type": "function_call", "call_id": nativeCall, "name": "bash", "arguments": `{"command":"pwd"}`},
		{"type": "function_call", "call_id": shortCall, "name": "noop", "arguments": `{"path":"/tmp"}`},
		{"type": "function_call_output", "call_id": nativeCall, "output": imageOutput},
		{"type": "function_call_output", "call_id": shortCall, "output": "ok"},
	})
	original := append([]byte(nil), input...)

	got := NormalizeResponsesInput(input)
	require.Equal(t, string(original), string(input), "不得修改调用方传入的原始字节")

	var before, after []map[string]any
	require.NoError(t, common.Unmarshal(original, &before))
	require.NoError(t, common.Unmarshal(got, &after))
	require.Len(t, after, len(before)-1, "只允许丢弃不可回放的 reasoning item")
	require.NotContains(t, string(got), "chatcmpl-x_reasoning_0")

	// 除 call_id 外，每个保留 item 必须与客户端内容逐字段一致（顺序不变）。
	contentWithoutCallID := func(item map[string]any) string {
		copied := make(map[string]any, len(item))
		for key, value := range item {
			if key == "call_id" {
				continue
			}
			copied[key] = value
		}
		raw, err := common.Marshal(copied)
		require.NoError(t, err)
		return string(raw)
	}
	require.JSONEq(t, contentWithoutCallID(before[0]), contentWithoutCallID(after[0]))
	require.JSONEq(t, contentWithoutCallID(before[2]), contentWithoutCallID(after[1]), "可回放 reasoning 必须原样保留")
	require.JSONEq(t, contentWithoutCallID(before[3]), contentWithoutCallID(after[2]))
	for i := range 4 {
		require.JSONEq(t, contentWithoutCallID(before[4+i]), contentWithoutCallID(after[3+i]),
			"工具调用/输出正文必须原样保留")
	}

	// 成对映射：call 与自己的 output 仍共享同一个 id；非签发形态的 id 不动。
	require.Equal(t, gjson.GetBytes(got, "3.call_id").String(), gjson.GetBytes(got, "5.call_id").String())
	require.Equal(t, gjson.GetBytes(got, "4.call_id").String(), gjson.GetBytes(got, "6.call_id").String())
	require.True(t, strings.HasPrefix(gjson.GetBytes(got, "3.call_id").String(), deepSeekPortableCallIDPrefix))
	require.Equal(t, shortCall, gjson.GetBytes(got, "4.call_id").String())
	require.Equal(t, string(got), string(NormalizeResponsesInput(got)), "规范化必须幂等")
}
