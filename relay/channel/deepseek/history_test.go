package deepseek

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestShouldExpandDeepSeekHistorySkipsFullReplayOfSameUserPrompt(t *testing.T) {
	prev := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": "Generate an SVG image of a pelican riding a bicycle by the seaside."},
	})
	replay := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": "Generate an SVG image of a pelican riding a bicycle by the seaside."},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})
	delta := mustJSON(t, []map[string]any{
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})
	followup := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": "Did you already write pelican-bike.svg?"},
	})
	require.False(t, shouldExpandDeepSeekHistory(prev, replay))
	require.True(t, shouldExpandDeepSeekHistory(prev, delta))
	require.True(t, shouldExpandDeepSeekHistory(prev, followup))
}

func TestMergeDeepSeekHistoryKeepsSvgWriteForIncrementalContinuation(t *testing.T) {
	prevInput := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": "Generate an SVG image of a pelican riding a bicycle by the seaside."},
	})
	prevOutput := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-write", "name": "exec_command", "arguments": `{"command":"cat > pelican-bike.svg <<'SVG'\n<svg id=\"keep-me\"/>\nSVG"}`},
	})
	delta := mustJSON(t, []map[string]any{
		{"type": "function_call_output", "call_id": "call-write", "output": "exit=0"},
	})

	got := mergeDeepSeekHistory(prevInput, prevOutput, delta)
	require.Equal(t, []string{"message", "function_call", "function_call_output"}, inputTypes(t, got))
	require.Contains(t, gjson.GetBytes(got, "1.arguments").String(), "keep-me")
	require.Equal(t, "call-write", gjson.GetBytes(got, "2.call_id").String())
}

func TestMergeDeepSeekHistoryDropsEchoedFunctionCall(t *testing.T) {
	prevInput := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": "write svg"},
	})
	prevOutput := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-write", "name": "exec_command", "arguments": `{"command":"cat > pelican-bike.svg"}`},
	})
	delta := mustJSON(t, []map[string]any{
		{"type": "function_call", "call_id": "call-write", "name": "exec_command", "arguments": `{"command":"cat > pelican-bike.svg"}`},
		{"type": "function_call_output", "call_id": "call-write", "output": "exit=0"},
	})

	got := mergeDeepSeekHistory(prevInput, prevOutput, delta)
	require.Equal(t, []string{"message", "function_call", "function_call_output"}, inputTypes(t, got))
	require.Equal(t, "call-write", gjson.GetBytes(got, "1.call_id").String())
	require.Equal(t, "call-write", gjson.GetBytes(got, "2.call_id").String())
}
