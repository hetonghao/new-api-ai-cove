package deepseek

import (
	"testing"

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
