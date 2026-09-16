package deepseek

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// The upstream needs reasoning_text above the tool run it is being asked to
// continue. Adding it must not rewrite anything the upstream already cached:
// one production conversation lost 92% of its prompt cache because every turn
// replaced the first empty reasoning item, 184 items into an 800k-token context.
func TestRepairDeepSeekTrailingToolRunInsertsAboveTrailingRunWithoutReasoning(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "id": "rs_1", "summary": []any{}, "content": nil},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})

	got := repairDeepSeekTrailingToolRun(input, "need pwd")
	require.Equal(t, []string{"reasoning", "reasoning", "function_call", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "rs_1", gjson.GetBytes(got, "0.id").String())
	require.Equal(t, "need pwd", gjson.GetBytes(got, "1.content.0.text").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "2.call_id").String())
}

func TestRepairDeepSeekTrailingToolRunLeavesCachedPrefixByteIdentical(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "id": "rs_1", "summary": []any{}, "content": nil},
		{"type": "function_call", "call_id": "call-old", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-old", "output": "old"},
		{"type": "message", "role": "user", "content": "继续"},
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "already"}}},
		{"type": "function_call", "call_id": "call-new", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-new", "output": "new"},
	})

	got := repairDeepSeekTrailingToolRun(input, "need pwd")
	require.Equal(t, string(input), string(got))
}

func TestRepairDeepSeekTrailingToolRunSkipsRunsBelowTheTail(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "id": "rs_1", "summary": []any{}, "content": nil},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		{"type": "message", "role": "user", "content": "a"},
		{"type": "message", "role": "assistant", "content": "b"},
		{"type": "message", "role": "user", "content": "c"},
	})

	got := repairDeepSeekTrailingToolRun(input, "need pwd")
	require.Equal(t, string(input), string(got))
}

func TestRepairDeepSeekTrailingToolRunKeepsEmptyReasoningWithoutCache(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "id": "rs_1", "summary": []any{}, "content": nil},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})

	got := repairDeepSeekTrailingToolRun(input, "")
	require.Equal(t, string(input), string(got))
}

func TestRepairDeepSeekTrailingToolRunRegroupsInterleavedToolPairs(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "get_a", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "A"},
		{"type": "function_call", "call_id": "call-2", "name": "get_b", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-2", "output": "B"},
	})

	got := repairDeepSeekTrailingToolRun(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call", "function_call", "function_call_output", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "need pwd", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "1.call_id").String())
	require.Equal(t, "call-2", gjson.GetBytes(got, "2.call_id").String())
}

func TestRepairDeepSeekTrailingToolRunRegroupsInterleavedAfterExistingReasoning(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "old"}}},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "old"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-2", "output": "new"},
	})

	got := repairDeepSeekTrailingToolRun(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call", "function_call", "function_call_output", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "old", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "1.call_id").String())
	require.Equal(t, "call-2", gjson.GetBytes(got, "2.call_id").String())
}

func TestRepairDeepSeekTrailingToolRunDoesNotSplitParallelToolCalls(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "a"},
		{"type": "function_call_output", "call_id": "call-2", "output": "b"},
	})

	got := repairDeepSeekTrailingToolRun(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call", "function_call", "function_call_output", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "need pwd", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "1.call_id").String())
	require.Equal(t, "call-2", gjson.GetBytes(got, "2.call_id").String())
}

func TestRepairDeepSeekTrailingToolRunKeepsRunWithUnpairedCallInOrder(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "old"}}},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "a"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
	})

	got := repairDeepSeekTrailingToolRun(input, "need pwd")
	require.Equal(t, string(input), string(got))
}

func TestRepairDeepSeekTrailingToolRunKeepsAdjacentReasoning(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "already"}}},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})

	got := repairDeepSeekTrailingToolRun(input, "cached")
	require.Equal(t, string(input), string(got))
}

func TestRepairDeepSeekTrailingToolRunInjectsBeforeCompactToolOutputs(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		{"type": "function_call_output", "call_id": "call-2", "output": "ok2"},
	})

	got := repairDeepSeekTrailingToolRun(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call_output", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "need pwd", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "1.call_id").String())
}

// Compact 续轮会以 [calls..., outputs..., reasoning(空)] 结束：缺的 reasoning_text
// 必须补在 call 之前，而末尾那条空 reasoning 一个字节都不能改。
func TestRepairDeepSeekTrailingToolRunKeepsCompactTrailingReasoningUntouched(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": "look at existing svg"},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": `{"cmd":"cat pelican-bike.svg"}`},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "exit=0"},
		{"type": "function_call_output", "call_id": "call-2", "output": "ok"},
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": ""}}},
	})

	got := repairDeepSeekTrailingToolRun(input, "look at existing svg")
	require.Equal(t, []string{
		"message",
		"reasoning",
		"function_call",
		"function_call",
		"function_call_output",
		"function_call_output",
		"reasoning",
	}, inputTypes(t, got))
	require.Equal(t, "look at existing svg", gjson.GetBytes(got, "1.content.0.text").String())
	require.Contains(t, gjson.GetBytes(got, "2.arguments").String(), "pelican-bike.svg")
	require.Equal(t, "", gjson.GetBytes(got, "6.content.0.text").String())
}

func TestRepairDeepSeekTrailingToolRunKeepsHoistedTextAboveCalls(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "message", "role": "assistant", "content": []map[string]string{{"type": "output_text", "text": "reading"}}},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "a"},
		{"type": "function_call_output", "call_id": "call-2", "output": "b"},
	})

	got := repairDeepSeekTrailingToolRun(canonicalizeDeepSeekToolRuns(input), "need pwd")
	require.Equal(t, []string{
		"message",
		"reasoning",
		"function_call",
		"function_call",
		"function_call_output",
		"function_call_output",
	}, inputTypes(t, got))
}

func TestForceRepairDeepSeekReasoningFillsEveryEmptyItemAndRun(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "id": "rs_1", "summary": []any{}, "content": nil},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		{"type": "message", "role": "user", "content": "继续"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-2", "output": "ok2"},
		{"type": "reasoning", "id": "rs_2", "summary": []any{}, "content": nil},
	})

	got, changed := forceRepairDeepSeekReasoning(input, "cached reasoning")
	require.True(t, changed)
	require.Equal(t, []string{
		"reasoning",
		"function_call",
		"function_call_output",
		"message",
		"reasoning",
		"function_call",
		"function_call_output",
		"reasoning",
	}, inputTypes(t, got))
	require.Equal(t, "rs_1", gjson.GetBytes(got, "0.id").String())
	require.Equal(t, "cached reasoning", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "cached reasoning", gjson.GetBytes(got, "4.content.0.text").String())
	require.Equal(t, "rs_2", gjson.GetBytes(got, "7.id").String())
	require.Equal(t, "cached reasoning", gjson.GetBytes(got, "7.content.0.text").String())
}

func TestReasoningRetryPayloadRebuildsRejectedTurn(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	request := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: mustJSON(t, []map[string]any{
			{"type": "message", "role": "user", "content": "继续"},
			{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		}),
	}

	raw, ok := reasoningRetryPayload(c, nil, request, "cached reasoning")
	require.True(t, ok)
	diagnostics := relaycommon.DeepSeekToolReasoningDiagnosticsOfInput([]byte(gjson.GetBytes(raw, "input").Raw))
	require.Equal(t, 1, diagnostics.Runs)
	require.False(t, diagnostics.NeedsReasoning())
	require.Contains(t, string(raw), "cached reasoning")
	require.Equal(t, "deepseek-v4.1-flash", gjson.GetBytes(raw, "model").String())

	// Nothing to repair means the caller keeps the original rejection.
	_, ok = reasoningRetryPayload(c, nil, dto.OpenAIResponsesRequest{Input: mustJSON(t, []map[string]any{
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "done"}}},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})}, "cached reasoning")
	require.False(t, ok)
}

func TestCanonicalizeDeepSeekToolRunsHoistsReasoningBetweenParallelCalls(t *testing.T) {
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

func TestCanonicalizeDeepSeekToolRunsDropsOrphanOutput(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		{"type": "function_call_output", "call_id": "call-orphan", "output": "stale result"},
		{"type": "function_call_output", "call_id": "call-2", "output": "kept"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
	})

	got := canonicalizeDeepSeekToolRuns(input)
	require.Equal(t, []string{"function_call", "function_call_output", "function_call_output", "function_call"}, inputTypes(t, got))
	require.NotContains(t, string(got), "call-orphan")
	require.Contains(t, string(got), "kept")
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
