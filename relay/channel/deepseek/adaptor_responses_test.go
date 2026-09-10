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
