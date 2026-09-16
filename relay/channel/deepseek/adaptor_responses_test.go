package deepseek

import (
	"encoding/json"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
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

// 上游不解析 previous_response_id 状态，孤立 output 只会让整轮 400
// (No tool call found for tool output)，所以和落单 call 一样丢掉。
func TestConvertOpenAIResponsesRequestDropsUnpairedFunctionCallOutput(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: mustJSON(t, []map[string]any{
			{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
			{"type": "message", "role": "user", "content": "continue"},
		}),
	}

	got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
	require.NoError(t, err)
	converted, ok := got.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, []string{"message"}, inputTypes(t, converted.Input))
	require.NotContains(t, string(converted.Input), "call-1")
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

func TestConvertOpenAIResponsesRequestHoistsTextBetweenParallelToolCalls(t *testing.T) {
	shape := []map[string]any{
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "need skill"}}},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": `{"cmd":"cat SKILL.md"}`},
		{"type": "message", "role": "assistant", "content": []map[string]string{{"type": "output_text", "text": "规则已读完。"}}},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command", "arguments": `{"cmd":"ps aux"}`},
		{"type": "function_call_output", "call_id": "call-1", "output": "skill body"},
		{"type": "function_call_output", "call_id": "call-2", "output": "ps out"},
	}

	for _, previousResponseID := range []string{"", "resp_switch"} {
		req := dto.OpenAIResponsesRequest{
			Model:              "deepseek-v4.1-flash",
			PreviousResponseID: previousResponseID,
			Input:              mustJSON(t, shape),
		}

		got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
		require.NoError(t, err)
		converted, ok := got.(dto.OpenAIResponsesRequest)
		require.True(t, ok)
		require.Equal(t, []string{
			"reasoning",
			"message",
			"function_call",
			"function_call",
			"function_call_output",
			"function_call_output",
		}, inputTypes(t, converted.Input))
		require.Equal(t, "call-1", gjson.GetBytes(converted.Input, "2.call_id").String())
		require.Equal(t, "call-1", gjson.GetBytes(converted.Input, "4.call_id").String())
		require.Equal(t, "call-2", gjson.GetBytes(converted.Input, "5.call_id").String())
	}
}

func TestConvertOpenAIResponsesRequestKeepsTextAfterToolOutputs(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: mustJSON(t, []map[string]any{
			{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
			{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
			{"type": "message", "role": "assistant", "content": []map[string]string{{"type": "output_text", "text": "done"}}},
		}),
	}

	got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
	require.NoError(t, err)
	converted, ok := got.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.Equal(t, []string{"function_call", "function_call_output", "message"}, inputTypes(t, converted.Input))
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

// DeepSeek 链路门禁：下面每一种形状都是生产上真出过 400 的 payload。转换只允许
// 重排、提升、丢弃，不允许插入或改写 item。上游把 `assistant message → reasoning`
// 这种相邻顺序判成 400（The `reasoning_text` in the thinking mode must be passed
// back to the API.，2026-09-16 用生产 dump 直连上游逐条复现），而工具轮缺 reasoning
// 或缺 assistant message 都是 200；插入 item 还会改写上游已经缓存的前缀。
func TestConvertOpenAIResponsesRequestKeepsDeepSeekUpstreamContract(t *testing.T) {
	cases := []struct {
		name   string
		shape  []map[string]any
		expect []string
		// stable 表示这段 payload 已经合法，转换必须逐字节保持不变。
		stable bool
	}{
		{
			name: "工具输出报错：并行轮交错 call/out",
			shape: []map[string]any{
				{"type": "message", "role": "user", "content": "跑一下"},
				{"type": "function_call", "call_id": "call-1", "name": "bash", "arguments": "{}"},
				{"type": "function_call_output", "call_id": "call-1", "output": "a"},
				{"type": "function_call", "call_id": "call-2", "name": "bash", "arguments": "{}"},
				{"type": "function_call_output", "call_id": "call-2", "output": "b"},
			},
			expect: []string{"message", "function_call", "function_call", "function_call_output", "function_call_output"},
		},
		{
			name: "reasoning_text 400：assistant message 后面直接跟工具轮，客户端没带 reasoning",
			shape: []map[string]any{
				{"type": "message", "role": "user", "content": "看看磁盘"},
				{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "我来扫一遍"}}},
				{"type": "function_call", "call_id": "call-1", "name": "bash", "arguments": "{}"},
				{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
			},
			expect: []string{"message", "message", "function_call", "function_call_output"},
			stable: true,
		},
		{
			name: "续轮：客户端自己带了 reasoning，message 在 reasoning 之后",
			shape: []map[string]any{
				{"type": "message", "role": "user", "content": "继续"},
				{"type": "reasoning", "id": "rs_1", "summary": []any{}, "content": []map[string]any{{"type": "reasoning_text", "text": "先看 df"}}},
				{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "开始扫描"}}},
				{"type": "function_call", "call_id": "call-1", "name": "bash", "arguments": "{}"},
				{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
			},
			expect: []string{"message", "reasoning", "message", "function_call", "function_call_output"},
			stable: true,
		},
		{
			name: "跨模型历史：xAI web_search_call 和孤立 output",
			shape: []map[string]any{
				{"type": "message", "role": "user", "content": "查一下"},
				{"type": "function_call", "call_id": "call-1", "name": "bash", "arguments": "{}"},
				{"type": "web_search_call", "status": "completed"},
				{"type": "function_call_output", "call_id": "call-1", "output": "a"},
				{"type": "function_call_output", "call_id": "call-orphan", "output": "stale"},
			},
			expect: []string{"message", "function_call", "function_call_output"},
		},
		{
			name: "工具轮中间夹 assistant message",
			shape: []map[string]any{
				{"type": "function_call", "call_id": "call-1", "name": "bash", "arguments": "{}"},
				{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "读到了"}}},
				{"type": "function_call", "call_id": "call-2", "name": "bash", "arguments": "{}"},
				{"type": "function_call_output", "call_id": "call-1", "output": "a"},
				{"type": "function_call_output", "call_id": "call-2", "output": "b"},
			},
			expect: []string{"message", "function_call", "function_call", "function_call_output", "function_call_output"},
		},
		{
			name: "Compact 续轮：calls、outputs、末尾空 reasoning",
			shape: []map[string]any{
				{"type": "message", "role": "user", "content": "看看现有 svg"},
				{"type": "function_call", "call_id": "call-1", "name": "bash", "arguments": "{}"},
				{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
				{"type": "reasoning", "summary": []any{}, "content": []map[string]any{{"type": "reasoning_text", "text": ""}}},
			},
			expect: []string{"message", "function_call", "function_call_output", "reasoning"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := dto.OpenAIResponsesRequest{
				Model: "deepseek-v4.1-flash",
				Input: mustJSON(t, tc.shape),
			}

			got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
			require.NoError(t, err)
			converted, ok := got.(dto.OpenAIResponsesRequest)
			require.True(t, ok)
			require.Equal(t, tc.expect, inputTypes(t, converted.Input))
			if tc.stable {
				require.Equal(t, string(mustJSON(t, tc.shape)), string(converted.Input))
			}

			// 上游契约：不允许 assistant message → reasoning、落单调用、
			// 孤立 output，或工具轮里夹别的 item。
			diagnostics := relaycommon.DeepSeekToolReasoningDiagnosticsOfInput(converted.Input)
			require.Zero(t, diagnostics.ReasoningAfterMessage, diagnostics.String())
			require.Zero(t, diagnostics.UnpairedCalls, diagnostics.String())
			require.Zero(t, diagnostics.OrphanOutputs, diagnostics.String())
			require.Zero(t, diagnostics.Interleaved, diagnostics.String())
		})
	}
}
