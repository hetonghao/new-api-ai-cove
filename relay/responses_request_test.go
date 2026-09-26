package relay

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/deepseek"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 2026-09-21 11:22 生产 400 现场（call_00_GLi6SIHQhqISBhtuwGpG3282）：Codex 把
// <image_resize_notice> 这条 developer message 发在 view_image 的 call 与它自己的
// output 之间，zen 上游整轮回 "No tool output found for tool call ..."。那次的 dump 里
// sent 与 client 形状完全一致，说明规范化根本没跑到——它当时只挂在 DeepSeek adaptor
// 的 ConvertOpenAIResponsesRequest 上。
//
// 这里用**非 DeepSeek adaptor** 的渠道（type 1 OpenAI）承载 deepseek 模型，锁住这一层：
// 只要请求会打到 DeepSeek /responses 上游，出站 payload 就必须已经规范化。
func TestPrepareResponsesRequestNormalizesDeepSeekToolsForAnyAdaptor(t *testing.T) {
	for _, modelName := range []string{"deepseek-v4.1-flash", "gpt-4.1"} {
		t.Run(modelName, func(t *testing.T) {
			clientInput := deepSeekInterleavedNoticeInput(t)
			req := dto.OpenAIResponsesRequest{Model: modelName, Input: clientInput}

			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(nil)
			c.Set(string(constant.ContextKeyChannelType), constant.ChannelTypeOpenAI)
			c.Set(string(constant.ContextKeyChannelId), 1)
			c.Set(string(constant.ContextKeyChannelBaseUrl), "https://upstream.invalid")
			c.Set(string(constant.ContextKeyChannelKey), "sk-test")
			c.Set(string(constant.ContextKeyOriginalModel), modelName)

			info := &relaycommon.RelayInfo{
				RelayMode: relayconstant.RelayModeResponses,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType:       constant.ChannelTypeOpenAI,
					ChannelId:         1,
					ApiType:           constant.APITypeOpenAI,
					ChannelBaseUrl:    "https://upstream.invalid",
					UpstreamModelName: modelName,
				},
			}
			info.OriginModelName = modelName
			info.Request = &req

			_, body, closer, _, apiErr := PrepareResponsesRequest(c, info, &req)
			require.Nil(t, apiErr)
			defer closer.Close()

			out, err := io.ReadAll(body)
			require.NoError(t, err)

			var envelope struct {
				Input json.RawMessage `json:"input"`
			}
			require.NoError(t, json.Unmarshal(out, &envelope))
			if modelName != "deepseek-v4.1-flash" {
				require.JSONEq(t, string(clientInput), string(envelope.Input), "其他模型的历史不做 DeepSeek 重排")
				return
			}
			require.Equal(t, []string{
				"message",
				"function_call",
				"function_call",
				"function_call_output",
				"function_call_output",
				"message",
			}, deepSeekInputTypes(envelope.Input))
			require.Equal(t, "developer", gjson.GetBytes(envelope.Input, "5.role").String())

			// 客户端那一份仍然带着插队形状：client[] 诊断要继续反映“客户端发了什么”，
			// 只有 sent[] 才应该是规范化后的形状。
			require.Equal(t, 1, relaycommon.DeepSeekToolReasoningDiagnosticsOfInput(clientInput).Interleaved)
			require.Equal(t, 0, relaycommon.DeepSeekToolReasoningDiagnosticsOfInput(envelope.Input).Interleaved)
		})
	}
}

func deepSeekInterleavedNoticeInput(t *testing.T) json.RawMessage {
	t.Helper()
	imageOutput := []map[string]any{{"type": "input_image", "image_url": "data:image/png;base64,iVBORw0KGgo=", "detail": "high"}}
	raw, err := json.Marshal([]map[string]any{
		{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "看这两张图"}}},
		{"type": "function_call", "call_id": "call_00_GLi6SIHQhqISBhtuwGpG3282", "name": "view_image", "arguments": `{"path":"/tmp/a.png"}`},
		{"type": "message", "role": "developer", "content": []map[string]any{{"type": "input_text", "text": "<image_resize_notice>\nImage 1 of 1 was resized.\n</image_resize_notice>"}}},
		{"type": "function_call", "call_id": "call_01_0SUCSgUYWTDdcW0GlNmX3093", "name": "view_image", "arguments": `{"path":"/tmp/b.png"}`},
		{"type": "function_call_output", "call_id": "call_00_GLi6SIHQhqISBhtuwGpG3282", "output": imageOutput},
		{"type": "function_call_output", "call_id": "call_01_0SUCSgUYWTDdcW0GlNmX3093", "output": imageOutput},
	})
	require.NoError(t, err)
	return raw
}

func deepSeekInputTypes(input json.RawMessage) []string {
	items := gjson.ParseBytes(input).Array()
	types := make([]string, 0, len(items))
	for _, item := range items {
		types = append(types, item.Get("type").String())
	}
	return types
}

// 2026-09-25 生产工具轮：call A, call B, output A, resize notice, output B。
// 断言 AdvancedCustom 最终出站的完整 input，避免 adaptor 再次规范化掩盖一次处理的缺陷。
func TestPrepareResponsesRequestDeepSeekNoticeBetweenOutputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := model_setting.GetGlobalSettings()
	oldPassThrough := settings.PassThroughRequestEnabled
	settings.PassThroughRequestEnabled = false
	t.Cleanup(func() { settings.PassThroughRequestEnabled = oldPassThrough })

	items := map[string]map[string]any{
		"A":  {"type": "function_call", "call_id": "call_00_ReproA0001", "name": "noop", "arguments": "{}"},
		"B":  {"type": "function_call", "call_id": "call_01_ReproB0002", "name": "noop", "arguments": "{}"},
		"C":  {"type": "function_call", "call_id": "call_02_ReproC0003", "name": "noop", "arguments": "{}"},
		"OA": {"type": "function_call_output", "call_id": "call_00_ReproA0001", "output": []map[string]any{{"type": "input_image", "image_url": "data:image/png;base64,iVBORw0KGgo="}}},
		"OB": {"type": "function_call_output", "call_id": "call_01_ReproB0002", "output": "B"},
		"OC": {"type": "function_call_output", "call_id": "call_02_ReproC0003", "output": "C"},
		"N":  {"type": "message", "role": "developer", "content": []map[string]any{{"type": "input_text", "text": "<image_resize_notice>Image resized.</image_resize_notice>"}}},
		"M":  {"type": "message", "role": "developer", "content": []map[string]any{{"type": "input_text", "text": "Second notice."}}},
		"S":  {"type": "message", "role": "system", "content": "System notice"},
		"U":  {"type": "message", "role": "user", "content": "Continue"},
		"R":  {"type": "reasoning", "summary": []any{}, "content": []map[string]any{{"type": "reasoning_text", "text": "Inspect results"}}, "encrypted_content": "opaque-test-content"},
		"X":  {"type": "custom_tool_call", "call_id": "call_00_ReproX0001", "name": "custom", "input": "x"},
		"Y":  {"type": "custom_tool_call", "call_id": "call_01_ReproY0002", "name": "custom", "input": "y"},
		"OX": {"type": "custom_tool_call_output", "call_id": "call_00_ReproX0001", "output": "x result"},
		"OY": {"type": "custom_tool_call_output", "call_id": "call_01_ReproY0002", "output": "y result"},
	}
	for _, tc := range []struct {
		name  string
		input []string
		want  []string
	}{
		{"before_outputs", []string{"A", "B", "N", "OA", "OB"}, []string{"A", "B", "OA", "OB", "N"}},
		{"between_outputs", []string{"A", "B", "OA", "N", "OB"}, []string{"A", "B", "OA", "OB", "N"}},
		{"after_outputs", []string{"A", "B", "OA", "OB", "N"}, []string{"A", "B", "OA", "OB", "N"}},
		{"three_calls", []string{"A", "B", "C", "OA", "N", "OB", "OC"}, []string{"A", "B", "C", "OA", "OB", "OC", "N"}},
		{"multiple_notices", []string{"A", "B", "C", "OA", "N", "OB", "M", "OC"}, []string{"A", "B", "C", "OA", "OB", "OC", "N", "M"}},
		{"overlapping_pairs", []string{"A", "B", "OA", "C", "OB", "N", "OC"}, []string{"A", "B", "C", "OA", "OB", "OC", "N"}},
		{"later_call_extends_run", []string{"A", "N", "B", "OA", "C", "OB", "OC"}, []string{"A", "B", "C", "OA", "OB", "OC", "N"}},
		{"completed_pair_prefix", []string{"C", "OC", "A", "B", "OA", "N", "OB"}, []string{"C", "A", "B", "OC", "OA", "OB", "N"}},
		{"previous_turn_prefix", []string{"C", "OC", "U", "R", "A", "B", "OA", "N", "OB"}, []string{"C", "OC", "U", "R", "A", "B", "OA", "OB", "N"}},
		{"custom_tools", []string{"X", "Y", "OX", "N", "OY"}, []string{"X", "Y", "OX", "OY", "N"}},
		{"user_notice", []string{"A", "U", "B", "OA", "OB"}, []string{"A", "B", "OA", "OB", "U"}},
		{"system_keeps_existing_hoist", []string{"A", "S", "B", "OA", "OB"}, []string{"S", "A", "B", "OA", "OB"}},
		{"reasoning_and_notices", []string{"A", "N", "R", "B", "OA", "M", "OB"}, []string{"R", "A", "B", "OA", "OB", "N", "M"}},
		{"following_turn", []string{"A", "B", "OA", "N", "OB", "U", "C", "OC"}, []string{"A", "B", "OA", "OB", "N", "U", "C", "OC"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inputItems, wantItems := make([]map[string]any, 0, len(tc.input)), make([]map[string]any, 0, len(tc.want))
			for _, key := range tc.input {
				inputItems = append(inputItems, items[key])
			}
			for _, key := range tc.want {
				wantItems = append(wantItems, items[key])
			}
			input, err := common.Marshal(inputItems)
			require.NoError(t, err)
			want, err := common.Marshal(wantItems)
			require.NoError(t, err)
			req := dto.OpenAIResponsesRequest{Model: "deepseek-v4.1-flash", Input: input, Tools: json.RawMessage(`[{"type":"function","name":"noop","parameters":{"type":"object","properties":{}}}]`)}
			c, _ := gin.CreateTestContext(nil)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Set(string(constant.ContextKeyChannelType), constant.ChannelTypeAdvancedCustom)
			c.Set(string(constant.ContextKeyChannelId), 56)
			c.Set(string(constant.ContextKeyChannelBaseUrl), "https://upstream.invalid")
			c.Set(string(constant.ContextKeyChannelKey), "test-only")
			c.Set(string(constant.ContextKeyOriginalModel), req.Model)
			c.Set(string(constant.ContextKeyChannelOtherSetting), dto.ChannelOtherSettings{AdvancedCustom: &dto.AdvancedCustomConfig{Routes: []dto.AdvancedCustomRoute{{IncomingPath: "/v1/responses", UpstreamPath: "/v1/responses", Converter: "none"}}}})
			info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses}
			info.OriginModelName, info.Request = req.Model, &req
			_, body, closer, _, apiErr := PrepareResponsesRequest(c, info, &req)
			require.Nil(t, apiErr)
			defer closer.Close()
			outbound, err := io.ReadAll(body)
			require.NoError(t, err)
			var envelope struct {
				Input json.RawMessage `json:"input"`
			}
			require.NoError(t, common.Unmarshal(outbound, &envelope))
			assert.JSONEq(t, string(want), string(envelope.Input), "preserve every item and its fields in the expected order")
			assert.Equal(t, string(input), string(req.Input), "do not mutate the client request")
			assert.Equal(t, string(envelope.Input), string(deepseek.NormalizeResponsesInput(envelope.Input)), "normalization must be idempotent")
			diagnostics := relaycommon.DeepSeekToolReasoningDiagnosticsOfInput(envelope.Input)
			assert.Zero(t, diagnostics.Interleaved)
			assert.Zero(t, diagnostics.UnpairedCalls)
			assert.Zero(t, diagnostics.OrphanOutputs)
			assert.Zero(t, diagnostics.ReasoningAfterMessage)
		})
	}
}
