package relay

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
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
	clientInput := deepSeekInterleavedNoticeInput(t)
	req := dto.OpenAIResponsesRequest{Model: "deepseek-v4.1-flash", Input: clientInput}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set(string(constant.ContextKeyChannelType), constant.ChannelTypeOpenAI)
	c.Set(string(constant.ContextKeyChannelId), 1)
	c.Set(string(constant.ContextKeyChannelBaseUrl), "https://upstream.invalid")
	c.Set(string(constant.ContextKeyChannelKey), "sk-test")
	c.Set(string(constant.ContextKeyOriginalModel), "deepseek-v4.1-flash")

	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelId:         1,
			ApiType:           constant.APITypeOpenAI,
			ChannelBaseUrl:    "https://upstream.invalid",
			UpstreamModelName: "deepseek-v4.1-flash",
		},
	}
	info.OriginModelName = "deepseek-v4.1-flash"
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
	require.Equal(t, []string{
		"message",
		"message",
		"function_call",
		"function_call",
		"function_call_output",
		"function_call_output",
	}, deepSeekInputTypes(envelope.Input))
	require.Equal(t, "developer", gjson.GetBytes(envelope.Input, "1.role").String())

	// 客户端那一份仍然带着插队形状：client[] 诊断要继续反映“客户端发了什么”，
	// 只有 sent[] 才应该是规范化后的形状。
	require.Equal(t, 1, relaycommon.DeepSeekToolReasoningDiagnosticsOfInput(clientInput).Interleaved)
	require.Equal(t, 0, relaycommon.DeepSeekToolReasoningDiagnosticsOfInput(envelope.Input).Interleaved)
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
