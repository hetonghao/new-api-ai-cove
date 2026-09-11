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

func TestRewriteDeepSeekReasoningInputDropsEmptyReasoningWithoutCache(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "id": "rs_1"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})

	got := rewriteDeepSeekReasoningInput(input, "")
	require.Equal(t, []string{"function_call_output"}, inputTypes(t, got))
}

func TestRewriteDeepSeekReasoningInputInjectsBeforeLatestToolOutput(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "old"}}},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "old"},
		{"type": "function_call_output", "call_id": "call-2", "output": "new"},
	})

	got := rewriteDeepSeekReasoningInput(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call", "function_call_output", "reasoning", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "old", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "need pwd", gjson.GetBytes(got, "3.content.0.text").String())
}

func TestRewriteDeepSeekReasoningInputKeepsAdjacentReasoning(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "already"}}},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})

	got := rewriteDeepSeekReasoningInput(input, "cached")
	require.Equal(t, string(input), string(got))
}
