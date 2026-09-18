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

func TestConvertOpenAIResponsesRequestContinuationDoesNotRegroupOrDrop(t *testing.T) {
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

// D-2 / D-7 这类 type 60 渠道走 newapi adaptor，但 DeepSeek 上游的 responses 规范化
// 必须照样生效：agent_message 会被 zen 上游整条丢掉，留下 assistant message 收尾即 400。
func TestConvertOpenAIResponsesRequestRewritesAgentMessageForDeepSeek(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: mustJSON(t, []map[string]any{
			{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "你还没给我任务"}}},
			{"type": "agent_message", "author": "/root", "recipient": "/root/x", "id": "amsg_1",
				"content": []map[string]any{{"type": "input_text", "text": "Message Type: NEW_TASK\nPayload:\n"}}},
		}),
	}

	got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
	require.NoError(t, err)
	converted, ok := got.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, []string{"message", "message"}, inputTypes(t, converted.Input))
	require.Equal(t, "user", gjson.GetBytes(converted.Input, "1.role").String())
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
