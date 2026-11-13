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

func TestOfficialOpenAIResponsesReplaysForeignReasoning(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	previousPassthrough := settings.PassThroughRequestEnabled
	settings.PassThroughRequestEnabled = false
	t.Cleanup(func() { settings.PassThroughRequestEnabled = previousPassthrough })
	service.InitHttpClient()
	client := service.GetHttpClient()
	previousTransport := client.Transport
	t.Cleanup(func() { client.Transport = previousTransport })
	const summary = `[{"type":"summary_text","text":"Keep the original reasoning summary."}]`
	const foreign = `{"type":"reasoning","id":"item_5ba7ac0d-28e6-4317-ba48-0a57d052b060_5","status":"completed","encrypted_content":"","summary":` + summary + `,"content":null,"vendor":9007199254740993}`
	const replayable = `{"type":"reasoning","encrypted_content":"","summary":` + summary + `,"content":null,"vendor":9007199254740993}`
	for _, tc := range []struct {
		name, endpoint, item, want                 string
		passthrough, globalPassthrough, referenced bool
	}{
		{name: "official", item: foreign, want: replayable},
		{name: "channel passthrough", item: foreign, want: replayable, passthrough: true},
		{name: "global passthrough", item: foreign, want: replayable, globalPassthrough: true},
		{name: "missing ciphertext", item: `{"type":"reasoning","id":"item_missing","status":"completed","summary":[]}`, want: `{"type":"reasoning","summary":[]}`},
		{name: "null ciphertext", item: `{"type":"reasoning","id":"item_null","status":"completed","encrypted_content":null,"summary":[]}`, want: `{"type":"reasoning","encrypted_content":null,"summary":[]}`},
		{name: "ciphertext preserved", item: `{"type":"reasoning","id":"item_signed","status":"completed","encrypted_content":"opaque","summary":[]}`},
		{name: "malformed ciphertext preserved", item: `{"type":"reasoning","id":"item_bad","status":"completed","encrypted_content":false,"summary":[]}`},
		{name: "official item reference preserved", item: `{"type":"reasoning","id":"rs_official","status":"completed","encrypted_content":"","summary":[]}`},
		{name: "explicit reference preserved", item: foreign, referenced: true},
		{name: "already replayable", item: replayable},
		{name: "CPA", endpoint: "http://cpa-pro-1:8319", item: foreign},
		{name: "other upstream", endpoint: "https://api.openai.com.example", item: foreign},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings.PassThroughRequestEnabled = tc.globalPassthrough
			endpoint := tc.endpoint
			if endpoint == "" {
				endpoint = "https://api.openai.com"
			}
			want := tc.want
			if want == "" {
				want = tc.item
			}
			input := `[{"role":"user","content":"Look up the result."},{"type":"function_call","call_id":"call_lookup","name":"lookup","arguments":"{}"},` + tc.item + `,{"type":"function_call_output","call_id":"call_lookup","output":"found"}]`
			if tc.referenced {
				input = strings.TrimSuffix(input, "]") + `,{"type":"item_reference","id":"item_5ba7ac0d-28e6-4317-ba48-0a57d052b060_5"}]`
			}
			raw := `{"model":"gpt-6.1-sol","store":false,"reasoning":{"effort":"high"},"input":` + input + `}`
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(raw))
			c.Request.Header.Set("Content-Type", "application/json")
			t.Cleanup(func() { common.CleanupBodyStorage(c) })
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, endpoint)
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-6.1-sol")
			common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: tc.passthrough})
			var request dto.OpenAIResponsesRequest
			require.NoError(t, common.Unmarshal([]byte(raw), &request))
			info := relaycommon.GenRelayInfoResponses(c, &request)
			info.RequestURLPath = "/v1/responses"
			adaptor, body, closer, _, apiErr := PrepareResponsesRequest(c, info, &request)
			require.Nil(t, apiErr)
			t.Cleanup(func() { require.NoError(t, closer.Close()) })
			var received []byte
			client.Transport = responsesIDTransport(func(upstream *http.Request) (*http.Response, error) {
				var err error
				received, err = io.ReadAll(upstream.Body)
				if err != nil {
					return nil, err
				}
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"output":[]}`))}, nil
			})
			result, err := adaptor.DoRequest(c, info, body)
			require.NoError(t, err)
			response := result.(*http.Response)
			t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
			assert.JSONEq(t, want, gjson.GetBytes(received, "input.2").Raw)
			assert.Equal(t, gjson.Get(tc.item, "vendor").Raw, gjson.GetBytes(received, "input.2.vendor").Raw)
			assert.Equal(t, "call_lookup", gjson.GetBytes(received, "input.1.call_id").String())
			assert.Equal(t, "call_lookup", gjson.GetBytes(received, "input.3.call_id").String())
			assert.Equal(t, "high", gjson.GetBytes(received, "reasoning.effort").String())
			assert.JSONEq(t, input, string(request.Input), "the next channel attempt must retain the original history")
			storage, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			original, err := storage.Bytes()
			require.NoError(t, err)
			assert.Equal(t, raw, string(original))
		})
	}
}
