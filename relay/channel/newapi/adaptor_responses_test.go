package newapi

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGetRequestURLJoinsResponsesPathWithoutBaseV1(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta:    &relaycommon.ChannelMeta{ChannelBaseUrl: "https://opencode.ai/zen/go", ChannelType: constant.ChannelTypeNewAPI},
		RequestURLPath: "/v1/responses",
	}
	got, err := (&Adaptor{}).GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://opencode.ai/zen/go/v1/responses", got)
}

func TestConvertOpenAIResponsesRequestRegroupsInterleavedDeepSeekTools(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: mustJSON(t, []map[string]any{
			{"type": "function_call", "call_id": "call-1", "name": "get_a", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "call-1", "output": "A"},
			{"type": "function_call", "call_id": "call-2", "name": "get_b", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "call-2", "output": "B"},
		}),
	}

	got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
	require.NoError(t, err)
	converted, ok := got.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, []string{"function_call", "function_call", "function_call_output", "function_call_output"}, inputTypes(t, converted.Input))
	require.Equal(t, "call-1", gjson.GetBytes(converted.Input, "0.call_id").String())
	require.Equal(t, "call-2", gjson.GetBytes(converted.Input, "1.call_id").String())
}

func TestConvertOpenAIResponsesRequestRegroupsCodexPelicanContinuation(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: mustJSON(t, []map[string]any{
			{"type": "message", "role": "user", "content": "Generate an SVG image of a pelican riding a bicycle by the seaside."},
			{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": `{"command":"cat > pelican-bike.svg <<'EOF'\n<svg/>\nEOF"}`},
			{"type": "function_call_output", "call_id": "call-1", "output": "exit=0"},
			{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": `{"command":"grep -c '<svg' pelican-bike.svg"}`},
			{"type": "function_call_output", "call_id": "call-2", "output": "1"},
			{"type": "function_call", "call_id": "call-3", "name": "exec_command", "arguments": `{"command":"ls -l pelican-bike.svg"}`},
			{"type": "function_call_output", "call_id": "call-3", "output": "VERIFY: <svg tag present"},
		}),
	}

	got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
	require.NoError(t, err)
	converted, ok := got.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, []string{
		"message",
		"function_call",
		"function_call",
		"function_call",
		"function_call_output",
		"function_call_output",
		"function_call_output",
	}, inputTypes(t, converted.Input))
	require.Equal(t, "call-1", gjson.GetBytes(converted.Input, "1.call_id").String())
	require.Equal(t, "call-2", gjson.GetBytes(converted.Input, "2.call_id").String())
	require.Equal(t, "call-3", gjson.GetBytes(converted.Input, "3.call_id").String())
}

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
