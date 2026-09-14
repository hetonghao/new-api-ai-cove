package deepseek

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
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

func TestRewriteDeepSeekReasoningInputInjectsBeforeLatestSequentialToolCall(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "old"}}},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "old"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-2", "output": "new"},
	})

	got := rewriteDeepSeekReasoningInput(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call", "function_call_output", "reasoning", "function_call", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "old", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "need pwd", gjson.GetBytes(got, "3.content.0.text").String())
	require.Equal(t, "call-2", gjson.GetBytes(got, "4.call_id").String())
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

func TestRewriteDeepSeekReasoningInputFillsEveryEmptyReasoningFromCache(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "id": "rs_1"},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "old"},
		{"type": "reasoning", "id": "rs_2", "encrypted_content": "stub"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-2", "output": "new"},
	})

	got := rewriteDeepSeekReasoningInput(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call", "function_call_output", "reasoning", "function_call", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "rs_1", gjson.GetBytes(got, "0.id").String())
	require.Equal(t, "need pwd", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "rs_2", gjson.GetBytes(got, "3.id").String())
	require.Equal(t, "need pwd", gjson.GetBytes(got, "3.content.0.text").String())
	require.False(t, gjson.GetBytes(got, "3.encrypted_content").Exists())
}

func TestRewriteDeepSeekReasoningInputInjectsBeforeOutputOnlyTurn(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		{"type": "function_call_output", "call_id": "call-2", "output": "also"},
	})

	got := rewriteDeepSeekReasoningInput(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call_output", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "need pwd", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "1.call_id").String())
}

func TestRewriteDeepSeekReasoningInputMovesReasoningOutOfParallelToolTurn(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "reasoning", "id": "rs_mid"},
		{"type": "function_call_output", "call_id": "call-1", "output": "a"},
		{"type": "function_call_output", "call_id": "call-2", "output": "b"},
	})

	got := rewriteDeepSeekReasoningInput(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call", "function_call", "function_call_output", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "need pwd", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "1.call_id").String())
	require.Equal(t, "call-2", gjson.GetBytes(got, "2.call_id").String())
}

func TestRewriteDeepSeekReasoningInputMovesFilledReasoningOutOfToolTurn(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "turn thought"}}},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})

	got := rewriteDeepSeekReasoningInput(input, "cached")
	require.Equal(t, []string{"reasoning", "function_call", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "turn thought", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "1.call_id").String())
}

func TestRewriteDeepSeekReasoningInputFillsEmptyFromEarlierTurnWhenCacheMiss(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "first"}}},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "old"},
		{"type": "reasoning", "id": "rs_2"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-2", "output": "new"},
	})

	got := rewriteDeepSeekReasoningInput(input, "")
	require.Equal(t, []string{"reasoning", "function_call", "function_call_output", "reasoning", "function_call", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "first", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "first", gjson.GetBytes(got, "3.content.0.text").String())
}

func TestRewriteDeepSeekReasoningInputAddsReasoningBeforeEarlierToolTurn(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "old"},
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "later"}}},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-2", "output": "new"},
	})

	got := rewriteDeepSeekReasoningInput(input, "need pwd")
	require.Equal(t, []string{"reasoning", "function_call", "function_call_output", "reasoning", "function_call", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "need pwd", gjson.GetBytes(got, "0.content.0.text").String())
	require.Equal(t, "call-1", gjson.GetBytes(got, "1.call_id").String())
	require.Equal(t, "later", gjson.GetBytes(got, "3.content.0.text").String())
	require.Equal(t, "call-2", gjson.GetBytes(got, "4.call_id").String())
}

func TestRewriteDeepSeekReasoningInputAddsReasoningBeforeEveryMissingToolTurn(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "a"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-2", "output": "b"},
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "third"}}},
		{"type": "function_call", "call_id": "call-3", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-3", "output": "c"},
	})
	got := rewriteDeepSeekReasoningInput(input, "need pwd")
	require.Equal(t, []string{
		"reasoning", "function_call", "function_call_output",
		"reasoning", "function_call", "function_call_output",
		"reasoning", "function_call", "function_call_output",
	}, inputTypes(t, got))
	requireReasoningBeforeEveryCall(t, got)
}

func requireReasoningBeforeEveryCall(t *testing.T, raw []byte) {
	t.Helper()
	types := inputTypes(t, raw)
	open := 0
	for i, typ := range types {
		switch typ {
		case "function_call", "custom_tool_call":
			if open == 0 {
				require.Greater(t, i, 0, "function_call at %d missing preceding reasoning", i)
				require.Equal(t, "reasoning", types[i-1], "function_call at %d not preceded by reasoning", i)
				require.NotEmpty(t, gjson.GetBytes(raw, fmt.Sprintf("%d.content.0.text", i-1)).String())
			}
			open++
		case "function_call_output", "custom_tool_call_output":
			if open > 0 {
				open--
			}
		case "reasoning":
			require.Zerof(t, open, "reasoning at %d splits an open tool turn", i)
		}
	}
}

func TestFillMissingOpenCodeReasoningPutsReasoningBeforeEveryToolTurn(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: mustJSON(t, []map[string]any{
			{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "call-1", "output": "a"},
			{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "think"}}},
			{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "call-2", "output": "b"},
		}),
	}
	FillMissingOpenCodeReasoning(nil, &req)
	require.Equal(t, []string{
		"reasoning", "function_call", "function_call_output",
		"reasoning", "function_call", "function_call_output",
	}, inputTypes(t, req.Input))
	require.Equal(t, "think", gjson.GetBytes(req.Input, "0.content.0.text").String())
	require.NotContains(t, string(req.Input), "根据以上工具结果，回答用户最初的问题")
}

func TestFillMissingOpenCodeReasoningMovesReasoningOutOfOpenToolTurn(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: mustJSON(t, []map[string]any{
			{"type": "message", "role": "user", "content": "hi"},
			{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
			{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "think"}}},
			{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "call-1", "output": "a"},
			{"type": "function_call_output", "call_id": "call-2", "output": "b"},
		}),
	}
	FillMissingOpenCodeReasoning(nil, &req)
	requireReasoningBeforeEveryCall(t, req.Input)
	require.NotContains(t, string(req.Input), "根据以上工具结果，回答用户最初的问题")
}

func TestFillMissingOpenCodeReasoningNoCacheLeavesOutputOnly(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: mustJSON(t, []map[string]any{
			{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		}),
	}
	FillMissingOpenCodeReasoning(nil, &req)
	require.Equal(t, []string{"function_call_output"}, inputTypes(t, req.Input))
}
