package newapi

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestConvertOpenAIResponsesRequestPreparesDeepSeekLikeNativeAdaptor(t *testing.T) {
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
	require.Equal(t, []string{"message"}, inputTypes(t, converted.Input))
}

func TestConvertOpenAIResponsesRequestSkipsNonDeepSeek(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model: "gpt-4.1",
		Input: mustJSON(t, []map[string]any{
			{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		}),
	}

	got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
	require.NoError(t, err)
	converted, ok := got.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, []string{"function_call"}, inputTypes(t, converted.Input))
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
