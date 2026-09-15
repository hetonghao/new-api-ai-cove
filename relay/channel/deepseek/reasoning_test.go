package deepseek

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRewriteDeepSeekReasoningInputFillsEmptyReasoningFromCache(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "id": "rs_1"},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})

	got := rewriteDeepSeekReasoningInput(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "reasoning_text", gjson.GetBytes(got, "0.content.0.type").String())
	require.Equal(t, "need pwd", gjson.GetBytes(got, "0.content.0.text").String())
}

func TestRewriteDeepSeekReasoningInputKeepsEmptyReasoningWithoutCache(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "id": "rs_1"},
		{"type": "reasoning", "id": "rs_2"},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})

	got := rewriteDeepSeekReasoningInput(input, "")
	require.Equal(t, []string{"reasoning", "reasoning", "function_call", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "rs_1", gjson.GetBytes(got, "0.id").String())
	require.Equal(t, "rs_2", gjson.GetBytes(got, "1.id").String())
}

func TestRewriteDeepSeekReasoningInputRegroupsInterleavedToolPairs(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "get_a", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "A"},
		{"type": "function_call", "call_id": "call-2", "name": "get_b", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-2", "output": "B"},
	})

	got := rewriteDeepSeekReasoningInput(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call", "function_call", "function_call_output", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "need pwd", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "1.call_id").String())
	require.Equal(t, "call-2", gjson.GetBytes(got, "2.call_id").String())
}

func TestRewriteDeepSeekReasoningInputRegroupsInterleavedAfterExistingReasoning(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "old"}}},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "old"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-2", "output": "new"},
	})

	got := rewriteDeepSeekReasoningInput(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call", "function_call", "function_call_output", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "old", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "1.call_id").String())
	require.Equal(t, "call-2", gjson.GetBytes(got, "2.call_id").String())
}

func TestRewriteDeepSeekReasoningInputDoesNotSplitParallelToolCalls(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "a"},
		{"type": "function_call_output", "call_id": "call-2", "output": "b"},
	})

	got := rewriteDeepSeekReasoningInput(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call", "function_call", "function_call_output", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "need pwd", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "1.call_id").String())
	require.Equal(t, "call-2", gjson.GetBytes(got, "2.call_id").String())
}

func TestRewriteDeepSeekReasoningInputKeepsAdjacentReasoning(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "already"}}},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})

	got := rewriteDeepSeekReasoningInput(input, "cached")
	require.Equal(t, string(input), string(got))
}

func TestRewriteDeepSeekReasoningInputInjectsBeforeCompactToolOutputs(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		{"type": "function_call_output", "call_id": "call-2", "output": "ok2"},
	})

	got := rewriteDeepSeekReasoningInput(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call_output", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "need pwd", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "1.call_id").String())
}

func TestFillEmptyDeepSeekReasoningInPlaceDoesNotRegroupOrInsert(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": `{"command":"cat > pelican-bike.svg <<'EOF'\n<svg/>\nEOF"}`},
		{"type": "function_call_output", "call_id": "call-1", "output": "exit=0"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-2", "output": "ok"},
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": ""}}},
	})

	got := fillEmptyDeepSeekReasoningInPlace(input, "look at existing svg")
	require.Equal(t, []string{"function_call", "function_call_output", "function_call", "function_call_output", "reasoning"}, inputTypes(t, got))
	require.Equal(t, "call-1", gjson.GetBytes(got, "0.call_id").String())
	require.Contains(t, gjson.GetBytes(got, "0.arguments").String(), "pelican-bike.svg")
	require.Equal(t, "look at existing svg", gjson.GetBytes(got, "4.content.0.text").String())
}
