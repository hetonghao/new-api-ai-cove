package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOfficialOpenAIChatFunctionToolsDisableReasoning(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	previous := *settings
	t.Cleanup(func() { *settings = previous })
	settings.ChatCompletionsToResponsesPolicy.Enabled = false
	service.InitHttpClient()
	client := service.GetHttpClient()
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })

	const functionTools = `[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}]`
	for _, tc := range []struct {
		name, endpoint, model, effort, tools, functions string
		override                                        map[string]any
		passthrough, globalPassthrough, stream, force   bool
	}{
		{name: "explicit reasoning", force: true},
		{name: "default reasoning", effort: "absent", force: true},
		{name: "already disabled", effort: "none", force: true},
		{name: "streaming", stream: true, force: true},
		{name: "luna snapshot", model: "gpt-6-luna-2026-11-01", force: true},
		{name: "sol", model: "gpt-6-sol", force: true},
		{name: "sol snapshot", model: "gpt-6-sol-2026-11-01", force: true},
		{name: "previous generation sol", model: "gpt-5.6-sol"},
		{name: "previous generation terra", model: "gpt-5.6-terra"},
		{name: "channel passthrough", passthrough: true, force: true},
		{name: "global passthrough", globalPassthrough: true, force: true},
		{name: "explicit official port", endpoint: "https://API.OPENAI.COM:443", force: true},
		{name: "reasoning parameter override", override: map[string]any{"reasoning_effort": "max"}, force: true},
		{name: "model parameter override", model: "gpt-4o", override: map[string]any{"model": "gpt-6-luna"}, force: true},
		{name: "tools parameter override", tools: "[]", override: map[string]any{"tools": gjson.Parse(functionTools).Value()}, force: true},
		{name: "legacy functions", tools: "[]", functions: `[{"name":"lookup","parameters":{"type":"object"}}]`, force: true},
		{name: "no function tools", tools: "[]"},
		{name: "other tool type", tools: `[{"type":"custom","custom":{"name":"lookup","format":{"type":"text"}}}]`},
		{name: "other model", model: "gpt-4o"},
		{name: "astra requires Responses", model: "gpt-6-astra"},
		{name: "new sol requires Responses", model: "gpt-6.1-sol"},
		{name: "terra requires Responses", model: "gpt-6.1-terra"},
		{name: "unknown model variant", model: "gpt-6-luna-pro"},
		{name: "invalid model snapshot", model: "gpt-6-luna-2026-99-99"},
		{name: "model override out of scope", override: map[string]any{"model": "gpt-4o"}},
		{name: "tools override out of scope", override: map[string]any{"tools": []any{}}},
		{name: "CPA", endpoint: "http://cpa-pro-1:8319"},
		{name: "other OpenAI compatible", endpoint: "https://api.openai.com.example"},
		{name: "other port", endpoint: "https://api.openai.com:8443"},
		{name: "insecure official address", endpoint: "http://api.openai.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings.PassThroughRequestEnabled = tc.globalPassthrough
			model, endpoint, effort, tools := tc.model, tc.endpoint, tc.effort, tc.tools
			if model == "" {
				model = "gpt-6-luna"
			}
			if endpoint == "" {
				endpoint = "https://api.openai.com"
			}
			if effort == "" {
				effort = "high"
			}
			if tools == "" {
				tools = functionTools
			}
			input := map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": "lookup"}}, "tools": gjson.Parse(tools).Value(), "stream": tc.stream, "vendor_extension": uint64(9007199254740993)}
			if effort != "absent" {
				input["reasoning_effort"] = effort
			}
			if tc.functions != "" {
				input["functions"] = gjson.Parse(tc.functions).Value()
			}
			raw, err := common.Marshal(input)
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(raw)))
			c.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { common.CleanupBodyStorage(c) })
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, endpoint)
			common.SetContextKey(c, constant.ContextKeyOriginalModel, model)
			common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: tc.passthrough})
			common.SetContextKey(c, constant.ContextKeyChannelParamOverride, tc.override)
			var request dto.GeneralOpenAIRequest
			require.NoError(t, common.Unmarshal(raw, &request))
			info := relaycommon.GenRelayInfoOpenAI(c, &request)
			var received, replay []byte
			client.Transport = responsesIDTransport(func(req *http.Request) (*http.Response, error) {
				var err error
				received, err = io.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				assert.EqualValues(t, len(received), req.ContentLength)
				if req.GetBody != nil {
					body, err := req.GetBody()
					if err != nil {
						return nil, err
					}
					defer body.Close()
					replay, err = io.ReadAll(body)
					if err != nil {
						return nil, err
					}
				}
				return &http.Response{StatusCode: http.StatusBadGateway, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"error":{"message":"upstream fixture","type":"upstream_error"}}`))}, nil
			})

			apiErr := TextHelper(c, info)

			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
			require.NotEmpty(t, received)
			assert.Equal(t, received, replay, "transport retries must replay the normalized body")
			wantEffort := effort
			if override, ok := tc.override["reasoning_effort"].(string); ok {
				wantEffort = override
			}
			if tc.force {
				wantEffort = "none"
				assert.Equal(t, "none", info.GetReasoningEffort())
			}
			assert.Equal(t, wantEffort, gjson.GetBytes(received, "reasoning_effort").String())
			assert.JSONEq(t, gjson.GetBytes(raw, "messages").Raw, gjson.GetBytes(received, "messages").Raw)
			assert.Equal(t, gjson.GetBytes(raw, "reasoning_effort").String(), request.ReasoningEffort)
			if tc.passthrough || tc.globalPassthrough {
				assert.Equal(t, "9007199254740993", gjson.GetBytes(received, "vendor_extension").Raw)
			}
			storage, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			original, err := storage.Bytes()
			require.NoError(t, err)
			assert.Equal(t, raw, original, "a later channel attempt must retain the original request")
		})
	}
}

func TestOfficialOpenAIResponsesFunctionToolsKeepReasoning(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	previous := settings.PassThroughRequestEnabled
	t.Cleanup(func() { settings.PassThroughRequestEnabled = previous })
	settings.PassThroughRequestEnabled = false
	service.InitHttpClient()
	client := service.GetHttpClient()
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	const raw = `{"model":"gpt-6-luna","input":"lookup","reasoning":{"effort":"high"},"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}]}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://api.openai.com")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-6-luna")
	var request dto.OpenAIResponsesRequest
	require.NoError(t, common.Unmarshal([]byte(raw), &request))
	info := relaycommon.GenRelayInfoResponses(c, &request)
	adaptor, body, closer, _, apiErr := PrepareResponsesRequest(c, info, &request)
	require.Nil(t, apiErr)
	defer closer.Close()
	var received []byte
	client.Transport = responsesIDTransport(func(req *http.Request) (*http.Response, error) {
		var err error
		received, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
	})

	response, err := adaptor.DoRequest(c, info, body)

	require.NoError(t, err)
	httpResponse, ok := response.(*http.Response)
	require.True(t, ok)
	require.NoError(t, httpResponse.Body.Close())
	assert.Equal(t, "high", gjson.GetBytes(received, "reasoning.effort").String())
	assert.False(t, gjson.GetBytes(received, "reasoning_effort").Exists())
	assert.Equal(t, "high", info.GetReasoningEffort())
}
