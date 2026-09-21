package controller

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// WS 这条路只用 sjson 给客户端 payload 打补丁（model / reasoning），其余字段照抄，
// 所以 adaptor 里那层 DeepSeek /responses 规范化不会作用到真正发出去的 payload 上。
// 这条用例锁住这层接线：插在 call 与它自己的 output 之间的 developer message 必须被
// 提到 call 之前，否则上游整轮回 "No tool output found for tool call ..."。
func TestResponsesWebSocketPayloadNormalizesDeepSeekInput(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"response.create","model":"deepseek-v4.1-flash","input":[` +
		`{"type":"message","role":"user","content":[{"type":"input_text","text":"看两张图"}]},` +
		`{"type":"function_call","call_id":"call-1","name":"view_image","arguments":"{}"},` +
		`{"type":"message","role":"developer","content":[{"type":"input_text","text":"<image_resize_notice>resized</image_resize_notice>"}]},` +
		`{"type":"function_call_output","call_id":"call-1","output":"ok"}]}`)
	var request dto.OpenAIResponsesRequest
	require.NoError(t, json.Unmarshal(payload, &request))

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
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

	outgoing, adaptor, apiErr := prepareResponsesWebSocketPayload(c, info, &request, payload)
	require.Nil(t, apiErr)
	require.NotNil(t, adaptor)

	items := gjson.GetBytes(outgoing, "input").Array()
	types := make([]string, 0, len(items))
	for _, item := range items {
		types = append(types, item.Get("type").String())
	}
	require.Equal(t, []string{"message", "message", "function_call", "function_call_output"}, types)
	require.Equal(t, "developer", gjson.GetBytes(outgoing, "input.1.role").String())
}
