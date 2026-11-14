package openai

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestOpenDesignToolStreamPreservesCallsAndCachedUsage(t *testing.T) {
	const endpoint = "https://amr-link.open-design.ai/v1/chat/completions"
	const firstID = "call_00_qwtu6xr4al2l3cug37hufjcm"
	const secondID = "call_01_toylys6yfcl7od9ovay4xyb0"
	const thirdID = "call_02_0123456789abcdefghijklmn"
	const firstStart = `data: {"model":"deepseek-v4.1-flash","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_00_qwtu6xr4al2l3cug37hufjcm","type":"function","function":{"name":"probe_a","arguments":""}}]}}]}`
	const firstArgs = `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"value\":\"A\"}"}}]}}]}`
	const secondStart = `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_01_toylys6yfcl7od9ovay4xyb0","type":"function","function":{"name":"probe_b","arguments":""}}]}}]}`
	const secondArgs = `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"value\":\""}}]}}]}`
	const secondTail = `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"B\"}"}}]}}]}`
	const finish = `data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`
	const usageFrame = `data: {"choices":[],"usage":{"prompt_tokens":376,"completion_tokens":67,"total_tokens":443,"prompt_tokens_details":{"cached_tokens":256}}}`
	normalIndex := func(frame string) string {
		return strings.ReplaceAll(frame, `"tool_calls":[{"index":0`, `"tool_calls":[{"index":1`)
	}
	thirdStart := strings.ReplaceAll(strings.ReplaceAll(secondStart, secondID, thirdID), "probe_b", "probe_c")
	frames := []string{firstStart, firstArgs, secondStart, secondArgs, secondTail}
	tests := []struct {
		name           string
		endpoint       string
		model          string
		frames         []string
		fail           bool
		bareEOF        bool
		missingRequest bool
		sameName       bool
		threeCalls     bool
		custom         bool
	}{
		{name: "repairs sequential zero indices", frames: frames},
		{name: "restores a custom tool without changing its input", frames: []string{firstStart, strings.ReplaceAll(firstArgs, "value", "input"), secondStart, secondArgs, secondTail}, custom: true},
		{name: "accepts explicit TLS default port", endpoint: "https://AMR-LINK.OPEN-DESIGN.AI:443/v1/chat/completions", frames: frames},
		{name: "keeps two calls to the same function separate", frames: []string{firstStart, firstArgs, strings.ReplaceAll(secondStart, "probe_b", "probe_a"), secondArgs, secondTail}, sameName: true},
		{name: "repairs three sequential calls", frames: append(append([]string{}, frames...), thirdStart, firstArgs), threeCalls: true},
		{name: "repairs a sequential nonzero call after index reuse", frames: append(append([]string{}, frames...), normalIndex(thirdStart), normalIndex(firstArgs)), threeCalls: true},
		{name: "repairs a sequential call with its original ordinal after index reuse", frames: append(append([]string{}, frames...), strings.ReplaceAll(thirdStart, `"tool_calls":[{"index":0`, `"tool_calls":[{"index":2`), strings.ReplaceAll(firstArgs, `"tool_calls":[{"index":0`, `"tool_calls":[{"index":2`)), threeCalls: true},
		{name: "preserves valid parallel indices", frames: []string{firstStart, firstArgs, normalIndex(secondStart), normalIndex(secondArgs), normalIndex(secondTail)}},
		{name: "preserves valid interleaved arguments", frames: []string{firstStart, normalIndex(secondStart), firstArgs, normalIndex(secondArgs), normalIndex(secondTail)}},
		{name: "does not repair OpenCode", endpoint: "https://opencode.ai/inference/go/v1/chat/completions", frames: frames, fail: true},
		{name: "does not repair another host on the same path", endpoint: "https://opencode.ai/v1/chat/completions", frames: frames, fail: true},
		{name: "does not repair a lookalike host", endpoint: "https://amr-link.open-design.ai.example.com/v1/chat/completions", frames: frames, fail: true},
		{name: "does not repair insecure transport", endpoint: "http://amr-link.open-design.ai/v1/chat/completions", frames: frames, fail: true},
		{name: "does not repair a different port", endpoint: "https://amr-link.open-design.ai:444/v1/chat/completions", frames: frames, fail: true},
		{name: "does not repair a different endpoint", endpoint: "https://amr-link.open-design.ai/v1/responses", frames: frames, fail: true},
		{name: "does not repair another model", model: "deepseek-v4-pro", frames: frames, fail: true},
		{name: "does not repair a model name lookalike", model: "deepseek-v4.1-flash-other", frames: frames, fail: true},
		{name: "requires the actual upstream request", frames: frames, missingRequest: true, fail: true},
		{name: "rejects a new call before previous arguments complete", frames: []string{firstStart, secondArgs, secondStart, firstArgs}, fail: true},
		{name: "rejects a skipped call ordinal", frames: []string{firstStart, firstArgs, strings.ReplaceAll(secondStart, secondID, thirdID)}, fail: true},
		{name: "rejects an unrecognized call id", frames: []string{firstStart, firstArgs, strings.ReplaceAll(secondStart, secondID, "call_arbitrary")}, fail: true},
		{name: "rejects a new call without a function name", frames: []string{firstStart, firstArgs, strings.ReplaceAll(secondStart, "probe_b", "")}, fail: true},
		{name: "rejects null previous arguments", frames: []string{firstStart, strings.ReplaceAll(firstArgs, `{\"value\":\"A\"}`, "null"), secondStart}, fail: true},
		{name: "rejects a missing upstream index", frames: []string{firstStart, firstArgs, strings.ReplaceAll(secondStart, `"tool_calls":[{"index":0,`, `"tool_calls":[{`)}, fail: true},
		{name: "rejects an earlier id after repair", frames: append(append([]string{}, frames...), firstStart), fail: true},
		{name: "rejects nonzero fragments after repair", frames: []string{firstStart, firstArgs, secondStart, normalIndex(secondArgs)}, fail: true},
		{name: "rejects an older index fragment after a new sequential call", frames: append(append([]string{}, frames...), normalIndex(thirdStart), firstArgs), fail: true},
		{name: "rejects a nonzero new call before previous arguments complete", frames: []string{firstStart, firstArgs, secondStart, secondArgs, normalIndex(thirdStart), normalIndex(firstArgs)}, fail: true},
		{name: "rejects a nonzero call returning to an earlier id", frames: append(append([]string{}, frames...), normalIndex(firstStart), normalIndex(firstArgs)), fail: true},
		{name: "rejects a missing index after repair", frames: append(append([]string{}, frames...), strings.ReplaceAll(thirdStart, `"tool_calls":[{"index":0,`, `"tool_calls":[{`)), fail: true},
		{name: "rejects a negative index after repair", frames: append(append([]string{}, frames...), strings.ReplaceAll(thirdStart, `"tool_calls":[{"index":0`, `"tool_calls":[{"index":-1`), strings.ReplaceAll(firstArgs, `"tool_calls":[{"index":0`, `"tool_calls":[{"index":-1`)), fail: true},
		{name: "rejects an index beyond the new call ordinal", frames: append(append([]string{}, frames...), strings.ReplaceAll(thirdStart, `"tool_calls":[{"index":0`, `"tool_calls":[{"index":3`), strings.ReplaceAll(firstArgs, `"tool_calls":[{"index":0`, `"tool_calls":[{"index":3`)), fail: true},
		{name: "rejects multiple choices after repair", frames: append(append([]string{}, frames...), `data: {"choices":[{"index":0,"delta":{}},{"index":1,"delta":{}}]}`), fail: true},
		{name: "rejects tools after finish", frames: append(append([]string{}, frames...), finish, secondStart), fail: true},
		{name: "rejects starting repair after finish", frames: []string{firstStart, firstArgs, finish, secondStart, secondArgs, secondTail}, fail: true},
		{name: "rejects a complete new call after finish", frames: []string{firstStart, firstArgs, finish, strings.ReplaceAll(secondStart, `"arguments":""`, `"arguments":"{\"value\":\"B\"}"`)}, fail: true},
		{name: "rejects a name change after repair", frames: []string{firstStart, firstArgs, secondStart, strings.ReplaceAll(firstStart, firstID, secondID)}, fail: true},
		{name: "rejects incomplete repaired arguments at finish", frames: []string{firstStart, firstArgs, secondStart, secondArgs}, fail: true},
		{name: "rejects incomplete repaired arguments at EOF", frames: []string{firstStart, firstArgs, secondStart, secondArgs}, bareEOF: true, fail: true},
		{name: "rejects a repaired stream without finish reason", frames: frames, bareEOF: true, fail: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstreamURL := tt.endpoint
			if upstreamURL == "" {
				upstreamURL = endpoint
			}
			model := tt.model
			if model == "" {
				model = "deepseek-v4.1-flash"
			}
			bodyFrames := append([]string{}, tt.frames...)
			if !tt.bareEOF {
				bodyFrames = append(bodyFrames, finish, usageFrame, "data: [DONE]")
			}
			body := strings.Join(append(bodyFrames, ""), "\n")
			c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info.RelayFormat = types.RelayFormatOpenAIResponses
			info.OriginModelName = "deepseek-v4-flash"
			info.UpstreamModelName = model
			info.ChannelBaseUrl = "https://amr-link.open-design.ai"
			if tt.custom {
				info.SetResponsesToolState(&convmeta.ResponsesToolState{CustomToolNames: map[string]struct{}{"probe_a": {}}})
			}
			if !tt.missingRequest {
				resp.Request = httptest.NewRequest(http.MethodPost, upstreamURL, nil)
			}
			usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
			var completed *dto.OpenAIResponsesResponse
			failed := false
			var added []string
			for line := range strings.SplitSeq(recorder.Body.String(), "\n") {
				data, ok := strings.CutPrefix(line, "data:")
				if !ok {
					continue
				}
				var event dto.ResponsesStreamResponse
				require.NoError(t, common.UnmarshalJsonStr(strings.TrimSpace(data), &event))
				switch event.Type {
				case "response.completed":
					completed = event.Response
				case "response.failed":
					failed = true
				case "response.output_item.added":
					require.NotNil(t, event.Item)
					if event.Item.Type == "function_call" || event.Item.Type == "custom_tool_call" {
						added = append(added, event.Item.CallId)
					}
				}
			}
			require.NotNil(t, info.StreamStatus)
			if tt.fail {
				require.True(t, failed, "ambiguous input must produce response.failed")
				require.Nil(t, completed)
				require.True(t, info.StreamStatus.ResponseFailed())
				return
			}
			require.Nil(t, apiErr)
			require.False(t, failed)
			require.False(t, info.StreamStatus.HasErrors())
			require.NotNil(t, completed)
			require.Equal(t, "completed", common.JsonRawMessageToString(completed.Status))
			expectedIDs := []string{firstID, secondID}
			expectedNames := []string{"probe_a", "probe_b"}
			expectedArgs := []string{`{"value":"A"}`, `{"value":"B"}`}
			if tt.sameName {
				expectedNames[1] = "probe_a"
			}
			if tt.threeCalls {
				expectedIDs = append(expectedIDs, thirdID)
				expectedNames = append(expectedNames, "probe_c")
				expectedArgs = append(expectedArgs, `{"value":"A"}`)
			}
			require.Equal(t, expectedIDs, added)
			require.Len(t, completed.Output, len(expectedIDs))
			for i, item := range completed.Output {
				require.Equal(t, expectedIDs[i], item.CallId)
				require.Equal(t, expectedNames[i], item.Name)
				if tt.custom && i == 0 {
					require.Equal(t, "custom_tool_call", item.Type)
					require.Equal(t, "A", common.JsonRawMessageToString(item.Input))
				} else {
					require.Equal(t, "function_call", item.Type)
					require.Equal(t, expectedArgs[i], item.ArgumentsString())
				}
			}
			require.NotNil(t, usage)
			require.Equal(t, 376, usage.PromptTokens)
			require.Equal(t, 67, usage.CompletionTokens)
			canonical, ok := usage.BillingUsage.CanonicalUsage()
			require.True(t, ok)
			require.Equal(t, 256, canonical.PromptTokensDetails.CachedTokens)
			require.NotNil(t, completed.Usage)
			require.Equal(t, 256, completed.Usage.InputTokensDetails.CachedTokens)
			require.False(t, common.GetContextKeyBool(c, constant.ContextKeyLocalCountTokens))
			if tt.name == "repairs three sequential calls" {
				diagnostics := info.ConversionDiagnostics()
				require.Len(t, diagnostics, 1)
				require.Equal(t, "open_design_tool_index_repaired", diagnostics[0].Code)
			}
		})
	}
}
