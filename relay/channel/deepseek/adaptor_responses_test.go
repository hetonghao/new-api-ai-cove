package deepseek

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConvertOpenAIResponsesRequestDropsUnpairedFunctionCall(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: mustJSON(t, []map[string]any{
			{"type": "message", "role": "user", "content": "hi"},
			{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		}),
	}

	got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
	require.NoError(t, err)
	converted, ok := got.(dto.OpenAIResponsesRequest)
	require.True(t, ok)

	types := inputTypes(t, converted.Input)
	require.Equal(t, []string{"message"}, types)
}

func TestConvertOpenAIResponsesRequestKeepsPairedFunctionCall(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: mustJSON(t, []map[string]any{
			{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		}),
	}

	got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
	require.NoError(t, err)
	converted, ok := got.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, []string{"function_call", "function_call_output"}, inputTypes(t, converted.Input))
	require.Equal(t, "call-1", gjson.GetBytes(converted.Input, "0.call_id").String())
	require.Equal(t, "call-1", gjson.GetBytes(converted.Input, "1.call_id").String())
}

func TestConvertOpenAIResponsesRequestKeepsUnpairedFunctionCallOutput(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: mustJSON(t, []map[string]any{
			{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		}),
	}

	got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
	require.NoError(t, err)
	converted, ok := got.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, []string{"function_call_output"}, inputTypes(t, converted.Input))
	require.Equal(t, "call-1", gjson.GetBytes(converted.Input, "0.call_id").String())
}

func TestConvertOpenAIResponsesRequestDropsUnpairedCustomToolCall(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: mustJSON(t, []map[string]any{
			{"type": "custom_tool_call", "call_id": "call-2", "name": "apply_patch"},
			{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		}),
	}

	got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
	require.NoError(t, err)
	converted, ok := got.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, []string{"function_call", "function_call_output"}, inputTypes(t, converted.Input))
}

func TestLoopingCodexContinuationKeepsSvgWriteThatUnpairedDropWouldStrip(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": "Generate an SVG image of a pelican riding a bicycle by the seaside."},
		{"type": "function_call", "call_id": "call-write", "name": "exec_command", "arguments": `{"cmd":"cat > outputs/pelican-bicycle-seaside.svg <<'SVG'\n<svg xmlns=\"http://www.w3.org/2000/svg\"/>\nSVG"}`},
		{"type": "function_call_output", "call_id": "call-write", "output": "exit=0"},
		{"type": "function_call", "call_id": "call-ls", "name": "exec_command", "arguments": `{"cmd":"ls outputs"}`},
		{"type": "function_call_output", "call_id": "call-ls", "output": "pelican-bicycle-seaside.svg"},
		{"type": "function_call", "call_id": "call-orphan", "name": "exec_command", "arguments": `{"cmd":"cat > outputs/pelican-bicycle-seaside.svg <<'SVG'\n<svg id=\"keep-me\"/>\nSVG"}`},
	})
	dropped := dropUnpairedDeepSeekToolCalls(input)
	require.NotContains(t, string(dropped), "keep-me")
	require.Contains(t, string(dropped), "call-write")

	req := dto.OpenAIResponsesRequest{
		Model:              "deepseek-v4.1-flash",
		PreviousResponseID: "resp_loop",
		Input:              input,
	}
	got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
	require.NoError(t, err)
	converted, ok := got.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, "resp_loop", converted.PreviousResponseID)
	require.Contains(t, string(converted.Input), "keep-me")
	require.Contains(t, string(converted.Input), "pelican-bicycle-seaside.svg")
	require.Equal(t, []string{
		"message",
		"function_call",
		"function_call_output",
		"function_call",
		"function_call_output",
		"function_call",
	}, inputTypes(t, converted.Input))
}

func TestConvertOpenAIResponsesRequestContinuationPreservesHistoryShape(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model:              "deepseek-v4.1-flash",
		PreviousResponseID: "resp_pelican",
		Input: mustJSON(t, []map[string]any{
			{"type": "message", "role": "user", "content": "Generate an SVG image of a pelican riding a bicycle by the seaside."},
			{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": `{"command":"cat > pelican-bike.svg <<'EOF'\n<svg/>\nEOF"}`},
			{"type": "function_call_output", "call_id": "call-1", "output": "exit=0"},
			{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "call-2", "output": "ok"},
			{"type": "function_call", "call_id": "call-3", "name": "exec_command", "arguments": "{}"},
		}),
	}

	got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
	require.NoError(t, err)
	converted, ok := got.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, "resp_pelican", converted.PreviousResponseID)
	require.Equal(t, []string{
		"message",
		"function_call",
		"function_call_output",
		"function_call",
		"function_call_output",
		"function_call",
	}, inputTypes(t, converted.Input))
	require.Contains(t, gjson.GetBytes(converted.Input, "1.arguments").String(), "pelican-bike.svg")
}

func TestConvertOpenAIResponsesRequestLeavesNonArrayInput(t *testing.T) {
	input := mustJSON(t, "hello")
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: input,
	}

	got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
	require.NoError(t, err)
	converted, ok := got.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, string(input), string(converted.Input))
}
func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return raw
}

func inputTypes(t *testing.T, input json.RawMessage) []string {
	t.Helper()
	arr := gjson.ParseBytes(input).Array()
	types := make([]string, 0, len(arr))
	for _, item := range arr {
		types = append(types, item.Get("type").String())
	}
	return types
}
