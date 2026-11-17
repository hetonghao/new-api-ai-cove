package controller

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	hostdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelayKeepAliveDuringImagePreparation(t *testing.T) {
	settings := operation_setting.GetGeneralSetting()
	previousEnabled, previousInterval := settings.PingIntervalEnabled, settings.PingIntervalSeconds
	previousCount, previousMedia := constant.CountToken, constant.GetMediaToken
	fetchSettings := system_setting.GetFetchSetting()
	previousFetchSettings := *fetchSettings
	fetchSettings.AllowPrivateIp = true
	fetchSettings.AllowedPorts = []string{"1-65535"}
	settings.PingIntervalEnabled, settings.PingIntervalSeconds = true, 1
	constant.CountToken, constant.GetMediaToken = true, true
	t.Cleanup(func() {
		settings.PingIntervalEnabled, settings.PingIntervalSeconds = previousEnabled, previousInterval
		constant.CountToken, constant.GetMediaToken = previousCount, previousMedia
		*fetchSettings = previousFetchSettings
	})
	service.InitHttpClient()

	imageRequested := make(chan struct{})
	releaseImage := make(chan struct{})
	var releaseOnce sync.Once
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(imageRequested)
		<-releaseImage
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer imageServer.Close()
	engine := gin.New()
	engine.POST("/v1/messages", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyOriginalModel, "claude-sonnet-4-20250514")
		Relay(c, types.RelayFormatClaude)
	})
	gateway := httptest.NewServer(engine)
	defer gateway.Close()
	defer releaseOnce.Do(func() { close(releaseImage) })

	body := `{"model":"claude-sonnet-4-20250514","max_tokens":16,"stream":true,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"url","url":"` + imageServer.URL + `"}}]}]}`
	client := &http.Client{Timeout: 4 * time.Second}
	response, err := client.Post(gateway.URL+"/v1/messages", "application/json", strings.NewReader(body))
	require.NoError(t, err, "the gateway must send keepalive while its image download is blocked")
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "text/event-stream", response.Header.Get("Content-Type"))
	reader := bufio.NewReader(response.Body)
	for range 2 {
		line, readErr := reader.ReadString('\n')
		require.NoError(t, readErr)
		require.Equal(t, ": PING\n", line)
		line, readErr = reader.ReadString('\n')
		require.NoError(t, readErr)
		require.Equal(t, "\n", line)
	}
	select {
	case <-imageRequested:
	default:
		t.Fatal("the request never reached image preparation")
	}
	releaseOnce.Do(func() { close(releaseImage) })
	remaining, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Contains(t, string(remaining), "event: error\n")
	require.Contains(t, string(remaining), `"type":"error"`)
	require.Contains(t, string(remaining), "data: ")
}

func TestRelayKeepAliveAcrossUpstreamRetries(t *testing.T) {
	settings := operation_setting.GetGeneralSetting()
	previousEnabled, previousInterval := settings.PingIntervalEnabled, settings.PingIntervalSeconds
	settings.PingIntervalEnabled, settings.PingIntervalSeconds = true, 1
	t.Cleanup(func() {
		settings.PingIntervalEnabled, settings.PingIntervalSeconds = previousEnabled, previousInterval
	})
	service.InitHttpClient()
	var attempts atomic.Int32
	retryStarted, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		close(retryStarted)
		select {
		case <-release:
			_, _ = io.WriteString(w, "data: ok\n\n")
		case <-r.Context().Done():
		}
	}))
	defer upstream.Close()
	engine := gin.New()
	engine.POST("/v1/messages", func(c *gin.Context) {
		stop := helper.StartStreamKeepAlive(c, 10*time.Millisecond)
		defer stop()
		if !assert.NoError(t, http.NewResponseController(c.Writer).SetWriteDeadline(time.Now().Add(5*time.Second))) {
			return
		}
		info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{}}
		for range 2 {
			request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstream.URL, strings.NewReader("{}"))
			if !assert.NoError(t, err) {
				return
			}
			response, err := channel.DoRequest(c, request, info)
			if !assert.NoError(t, err) {
				return
			}
			if response.StatusCode == http.StatusTooManyRequests {
				response.Body.Close()
				continue
			}
			_, _ = io.Copy(c.Writer, response.Body)
			response.Body.Close()
		}
	})
	gateway := httptest.NewServer(engine)
	defer gateway.Close()
	defer releaseOnce.Do(func() { close(release) })
	client := &http.Client{Timeout: 4 * time.Second}
	response, err := client.Post(gateway.URL+"/v1/messages", "application/json", strings.NewReader("{}"))
	require.NoError(t, err)
	defer response.Body.Close()
	select {
	case <-retryStarted:
	case <-time.After(time.Second):
		t.Fatal("the upstream retry did not start")
	}
	reader := bufio.NewReader(response.Body)
	for range 2 {
		line, readErr := reader.ReadString('\n')
		require.NoError(t, readErr)
		require.Equal(t, ": PING\n", line)
		line, readErr = reader.ReadString('\n')
		require.NoError(t, readErr)
		require.Equal(t, "\n", line)
	}
	releaseOnce.Do(func() { close(release) })
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Contains(t, string(body), "data: ok\n\n")
	require.Equal(t, int32(2), attempts.Load())
}

func TestRelayKeepAlivePreservesValidationErrors(t *testing.T) {
	settings := operation_setting.GetGeneralSetting()
	previousEnabled := settings.PingIntervalEnabled
	settings.PingIntervalEnabled = true
	t.Cleanup(func() { settings.PingIntervalEnabled = previousEnabled })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-20250514","stream":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	Relay(c, types.RelayFormatClaude)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
	require.NotContains(t, recorder.Body.String(), ": PING")
}

func TestStreamKeepAlivePreservesFastHTTPFailures(t *testing.T) {
	settings := operation_setting.GetGeneralSetting()
	previousEnabled, previousInterval := settings.PingIntervalEnabled, settings.PingIntervalSeconds
	settings.PingIntervalEnabled, settings.PingIntervalSeconds = true, 60
	t.Cleanup(func() {
		settings.PingIntervalEnabled, settings.PingIntervalSeconds = previousEnabled, previousInterval
	})
	service.InitHttpClient()
	for _, status := range []int{http.StatusUnauthorized, http.StatusPaymentRequired, http.StatusForbidden, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, `{"error":"request rejected"}`)
			}))
			defer upstream.Close()
			engine := gin.New()
			engine.POST("/v1/messages", func(c *gin.Context) {
				stop := helper.StartStreamKeepAlive(c, time.Minute)
				defer stop()
				request, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstream.URL, strings.NewReader("{}"))
				if !assert.NoError(t, err) {
					return
				}
				info := &relaycommon.RelayInfo{IsStream: true, ChannelMeta: &relaycommon.ChannelMeta{}}
				response, err := channel.DoRequest(c, request, info)
				if !assert.NoError(t, err) {
					return
				}
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				if assert.NoError(t, err) {
					c.Header("Content-Type", "application/json")
					c.Data(response.StatusCode, "application/json", body)
				}
			})
			gateway := httptest.NewServer(engine)
			defer gateway.Close()
			client := &http.Client{Timeout: 4 * time.Second}
			response, err := client.Post(gateway.URL+"/v1/messages", "application/json", strings.NewReader("{}"))
			require.NoError(t, err)
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, status, response.StatusCode)
			require.Equal(t, "application/json", response.Header.Get("Content-Type"))
			require.JSONEq(t, `{"error":"request rejected"}`, string(body))
		})
	}
}

func TestRelayKeepAliveHonorsStreamSettings(t *testing.T) {
	settings := operation_setting.GetGeneralSetting()
	previousEnabled := settings.PingIntervalEnabled
	previousCount, previousMedia, previousNonStreamMedia := constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream
	constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream = true, true, true
	t.Cleanup(func() {
		settings.PingIntervalEnabled = previousEnabled
		constant.CountToken, constant.GetMediaToken, constant.GetMediaTokenNotStream = previousCount, previousMedia, previousNonStreamMedia
	})
	for _, test := range []struct {
		name    string
		enabled bool
		stream  bool
	}{
		{name: "ping disabled", enabled: false, stream: true},
		{name: "non-streaming", enabled: true, stream: false},
		{name: "fast streaming failure", enabled: true, stream: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings.PingIntervalEnabled = test.enabled
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			body := `{"model":"claude-sonnet-4-20250514","max_tokens":16,"stream":` + strconv.FormatBool(test.stream) + `,"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"!"}}]}]}`
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "claude-sonnet-4-20250514")
			Relay(c, types.RelayFormatClaude)
			require.Equal(t, http.StatusInternalServerError, recorder.Code)
			require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
			require.NotContains(t, recorder.Body.String(), ": PING")
			var response map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			require.Equal(t, "error", response["type"])
		})
	}
}

func TestRelayStreamErrorAfterKeepAlive(t *testing.T) {
	for _, format := range []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI} {
		t.Run(string(format), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/stream", nil)
			helper.SetEventStreamHeaders(c)
			require.NoError(t, helper.PingData(c))
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://provider.example")
			common.SetContextKey(c, constant.ContextKeyUpstreamRequestOrigin, "https://provider.example")
			apiError := types.NewErrorWithStatusCode(errors.New("upstream rejected input at https://provider.example/v1/messages"), types.ErrorCodeBadResponseStatusCode, http.StatusBadRequest)
			writeRelayStreamError(c, format, apiError)
			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
			require.Contains(t, recorder.Body.String(), "upstream rejected input")
			require.NotContains(t, recorder.Body.String(), "provider.example")
			status, ok := common.GetContextKeyType[*relaycommon.StreamStatus](c, constant.ContextKeyResponseStreamStatus)
			require.True(t, ok)
			require.True(t, status.ResponseFailed(), "stream errors must not consume the successful-request rate limit")
			require.Equal(t, http.StatusBadRequest, status.OutcomeSnapshot().ErrorStatus)
			if format == types.RelayFormatClaude {
				require.Contains(t, recorder.Body.String(), "event: error\n")
			} else {
				require.Contains(t, recorder.Body.String(), "data: [DONE]")
			}
		})
	}
}

func TestStreamKeepAliveCancellation(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	requestContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Request = httptest.NewRequest(http.MethodPost, "/stream", nil).WithContext(requestContext)
	stop := helper.StartStreamKeepAlive(c, time.Hour)
	cancel()
	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("keepalive did not stop after client cancellation")
	}
	_, err := c.Writer.WriteString("data: done\n\n")
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(recorder.Body.String(), "data: done\n\n"))
}

func TestRelayResponsesRetriesRateLimitBeforeOutput(t *testing.T) {
	const modelName = "gpt-6.1-sol"
	const created = `{"type":"response.created","response":{"id":"resp_primary","status":"in_progress","output":[],"usage":null,"reasoning":{"effort":"high"},"tools":[{"type":"function","name":"lookup"}]}}`
	const limited = `{"type":"response.failed","response":{"id":"resp_primary","status":"failed","output":[],"usage":null,"error":{"code":"rate_limit_exceeded","type":"rate_limit_error","message":"TPM limit reached"}}}`
	startup := "data: " + created + "\n\n" + "data: " + `{"type":"response.in_progress","response":{"id":"resp_primary","status":"in_progress","output":[],"usage":null}}` + "\n\n"
	for _, format := range []types.RelayFormat{types.RelayFormatOpenAIResponses, types.RelayFormatOpenAI} {
		for _, tc := range []struct {
			name, prefix, failure, wantText, paramOverride                                       string
			fallback, noBudget, pin, strict, continuation, stateHeader, hold, cancel, nativeOnly bool
			retry500, writeFailure, cancelOnClose, cancelBeforeSend, itemReference, requestState bool
			settled, chatFallback, azureFallback                                                 bool
		}{
			{name: "nested limit", prefix: startup, failure: limited, fallback: true, settled: true},
			{name: "standard response metadata", prefix: strings.Replace(startup, `"output":[]`, `"completed_at":null,"output_text":"","tool_usage":{"web_search":{"num_requests":0}},"billing":{"payer":"developer"},"moderation":null,"prompt_cache_diagnostics":{"cache_missed_tokens":7,"reason":"model_changed","type":"cache_miss","comparison_reusable_tokens":0},"output":[]`, 1), failure: strings.Replace(limited, `"output":[]`, `"completed_at":null,"output_text":"","tool_usage":{"web_search":{"num_requests":0}},"billing":{"payer":"developer"},"moderation":null,"prompt_cache_diagnostics":{"cache_missed_tokens":7,"reason":"model_changed","type":"cache_miss","comparison_reusable_tokens":0},"output":[]`, 1), fallback: true, settled: true},
			{name: "nonzero tool usage", prefix: startup, failure: strings.Replace(limited, `"output":[]`, `"tool_usage":{"future_tool":{"calls":1}},"output":[]`, 1), settled: true},
			{name: "flat limit", prefix: startup, failure: `{"type":"error","code":"rate_limit_exceeded","message":"TPM limit reached"}`, fallback: true, settled: true},
			{name: "nested error envelope", prefix: startup, failure: `{"type":"error","error":{"code":"rate_limit_exceeded","type":"rate_limit_error","message":"TPM limit reached"}}`, fallback: true, settled: true},
			{name: "explicit zero usage", prefix: startup, failure: strings.Replace(limited, `"usage":null`, `"usage":{"input_tokens":0,"output_tokens_details":{"reasoning_tokens":"0"},"future":{"cost":0}}`, 1), fallback: true, settled: true},
			{name: "text already emitted", prefix: startup + "data: " + `{"type":"response.output_text.delta","delta":"already-visible"}` + "\n\n", failure: limited, wantText: "already-visible", settled: true},
			{name: "reasoning already emitted", prefix: startup + "data: " + `{"type":"response.reasoning_summary_text.delta","delta":"already-thinking"}` + "\n\n", failure: limited, settled: true},
			{name: "tool already emitted", prefix: startup + "data: " + `{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":""}}` + "\n\n", failure: limited, wantText: "lookup", settled: true},
			{name: "terminal then tool conversion error", prefix: startup + "data: " + `{"type":"error","code":"insufficient_quota","message":"stop"}` + "\n\n" + "data: " + `{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":""}}` + "\n\n" + "data: " + `{"type":"response.output_text.annotation.added","annotation":[]}` + "\n\n", failure: limited, wantText: "lookup", retry500: true},
			{name: "reported input usage", prefix: startup, failure: strings.Replace(limited, `"usage":null`, `"usage":{"input_tokens":7,"output_tokens":0,"total_tokens":7}`, 1), settled: true},
			{name: "reasoning-only usage", prefix: startup, failure: strings.Replace(limited, `"usage":null`, `"usage":{"output_tokens_details":{"reasoning_tokens":1}}`, 1), settled: true},
			{name: "unknown usage extension", prefix: startup, failure: strings.Replace(limited, `"usage":null`, `"usage":{"future_charge":{"tokens":1}}`, 1), settled: true},
			{name: "top-level usage", prefix: startup, failure: strings.Replace(limited, `"type":"response.failed"`, `"type":"response.failed","usage":{"input_tokens":7}`, 1), settled: true},
			{name: "unknown startup field", prefix: strings.Replace(startup, `"output":[]`, `"vendor_output":"opaque","output":[]`, 1), failure: limited, settled: true},
			{name: "oversized startup", prefix: strings.Replace(startup, `"output":[]`, `"instructions":"`+strings.Repeat("x", 128<<10)+`end-instructions","output":[]`, 1), failure: limited, wantText: "end-instructions", nativeOnly: true, settled: true},
			{name: "bare EOF preserves prefix", prefix: startup, wantText: "resp_primary", nativeOnly: true},
			{name: "DONE preserves prefix", prefix: startup + "data: [DONE]\n\n", wantText: "resp_primary", nativeOnly: true},
			{name: "EOF prefix write fails", prefix: startup, nativeOnly: true, writeFailure: true},
			{name: "quota error is not transient", prefix: startup, failure: strings.ReplaceAll(limited, "rate_limit_exceeded", "insufficient_quota")},
			{name: "no retry budget", prefix: startup, failure: limited, noBudget: true},
			{name: "fixed channel", prefix: startup, failure: limited, pin: true},
			{name: "strict affinity", prefix: startup, failure: limited, strict: true},
			{name: "server continuation", prefix: startup, failure: limited, continuation: true, nativeOnly: true},
			{name: "body override continuation", prefix: startup, failure: limited, paramOverride: `{"previous_response_id":"resp_server"}`, nativeOnly: true},
			{name: "operation override continuation", prefix: startup, failure: limited, paramOverride: `{"operations":[{"mode":"set","path":"conversation","value":"conv_server"}]}`, nativeOnly: true},
			{name: "echoed continuation", prefix: strings.Replace(startup, `"output":[]`, `"previous_response_id":"resp_server","output":[]`, 1), failure: limited, nativeOnly: true},
			{name: "item reference input", prefix: startup, failure: limited, itemReference: true, nativeOnly: true},
			{name: "header passthrough remains replayable", prefix: startup, failure: limited, paramOverride: `{"operations":[{"mode":"pass_headers","keep_origin":true,"value":["User-Agent"]}]}`, fallback: true, settled: true},
			{name: "request turn state", prefix: startup, failure: limited, paramOverride: `{"operations":[{"mode":"pass_headers","value":["X-Codex-Turn-State"]}]}`, requestState: true, nativeOnly: true},
			{name: "provider turn state", prefix: startup, failure: limited, stateHeader: true, nativeOnly: true},
			{name: "idle timeout", prefix: startup, hold: true},
			{name: "client cancellation", prefix: startup, hold: true, cancel: true},
			{name: "cancel during stream cleanup", prefix: startup, failure: limited, cancelOnClose: true},
			{name: "cancel before fallback network send", prefix: startup, failure: limited, cancelBeforeSend: true},
			{name: "native chat fallback", prefix: startup, failure: limited, chatFallback: true, fallback: true, settled: true},
			{name: "cancel before native chat fallback", prefix: startup, failure: limited, chatFallback: true, cancelBeforeSend: true},
			{name: "azure fallback", prefix: startup, failure: limited, azureFallback: true, fallback: true, settled: true},
			{name: "cancel before azure fallback", prefix: startup, failure: limited, azureFallback: true, cancelBeforeSend: true},
		} {
			if tc.nativeOnly && format != types.RelayFormatOpenAIResponses || tc.chatFallback && format != types.RelayFormatOpenAI {
				continue
			}
			t.Run(string(format)+"/"+tc.name, func(t *testing.T) {
				db := setupResponsesWebSocketHandlerTest(t)
				global := model_setting.GetGlobalSettings()
				previousGlobal := *global
				previousRetryCodes := operation_setting.AutomaticRetryStatusCodesToString()
				general, affinity := operation_setting.GetGeneralSetting(), operation_setting.GetChannelAffinitySetting()
				previousGeneral, previousAffinity := *general, *affinity
				previousCount, previousQuotaUnit, previousTimeout := constant.CountToken, common.QuotaPerUnit, constant.StreamingTimeout
				global.PassThroughRequestEnabled = false
				global.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{Enabled: true, ChannelIDs: []int{2401, 2402}, ModelPatterns: []string{"^" + modelName + "$"}}
				if tc.chatFallback {
					global.ChatCompletionsToResponsesPolicy.ChannelIDs = []int{2401}
				}
				retryCodes := "429"
				if tc.settled && !tc.fallback || tc.retry500 {
					retryCodes = "429,500"
				}
				require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString(retryCodes))
				general.PingIntervalEnabled, affinity.Enabled, constant.CountToken = false, false, false
				common.QuotaPerUnit = 500000
				if tc.noBudget {
					common.RetryTimes = 0
				}
				if tc.hold {
					constant.StreamingTimeout = 1
				}
				if tc.strict {
					*affinity = operation_setting.ChannelAffinitySetting{Enabled: true, DefaultTTLSeconds: 60, MaxEntries: 100, Rules: []operation_setting.ChannelAffinityRule{{Name: t.Name(), ModelRegex: []string{".*"}, SessionMode: "strict", KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Session"}}}}}
				}
				t.Cleanup(func() {
					*global, *general, *affinity = previousGlobal, previousGeneral, previousAffinity
					constant.CountToken, common.QuotaPerUnit, constant.StreamingTimeout = previousCount, previousQuotaUnit, previousTimeout
					require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString(previousRetryCodes))
				})
				withTieredBillingConfig(t, map[string]string{modelName: "tiered_expr"}, map[string]string{modelName: `tier("base", fixed(0.01))`})
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
				require.NoError(t, db.AutoMigrate(&model.Token{}))
				require.NoError(t, db.Create(&model.Token{Id: 43, UserId: 42, Key: "sse-retry-token", Status: common.TokenStatusEnabled, UnlimitedQuota: true}).Error)
				service.InitHttpClient()
				requestContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				var primaryCalls, fallbackCalls atomic.Int32
				requests := make(chan string, 2)
				primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					primaryCalls.Add(1)
					body, err := io.ReadAll(r.Body)
					if !assert.NoError(t, err) {
						return
					}
					requests <- string(body)
					assert.Equal(t, "/v1/responses", r.URL.Path)
					w.Header().Set("Content-Type", "text/event-stream")
					if tc.stateHeader {
						w.Header().Set("X-Codex-Turn-State", "provider-owned-state")
					}
					_, _ = io.WriteString(w, tc.prefix)
					if tc.hold {
						w.(http.Flusher).Flush()
						if tc.cancel {
							cancel()
						}
						<-r.Context().Done()
						return
					}
					if tc.failure != "" {
						_, _ = io.WriteString(w, "data: "+tc.failure+"\n\n")
					}
					if tc.cancelOnClose {
						w.(http.Flusher).Flush()
						<-r.Context().Done()
					}
				}))
				t.Cleanup(primary.Close)
				fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fallbackCalls.Add(1)
					body, err := io.ReadAll(r.Body)
					if !assert.NoError(t, err) {
						return
					}
					requests <- string(body)
					expectedPath := "/v1/responses"
					if tc.azureFallback {
						expectedPath = "/openai/v1/responses"
					}
					if tc.chatFallback {
						expectedPath = "/v1/chat/completions"
					}
					assert.Equal(t, expectedPath, r.URL.Path)
					w.Header().Set("Content-Type", "text/event-stream")
					if tc.chatFallback {
						_, _ = io.WriteString(w, "data: "+`{"id":"chat_fallback","object":"chat.completion.chunk","model":"gpt-6.1-sol","choices":[{"index":0,"delta":{"role":"assistant","content":"fallback-ok"},"finish_reason":null}]}`+"\n\n"+
							"data: "+`{"id":"chat_fallback","object":"chat.completion.chunk","model":"gpt-6.1-sol","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":5,"total_tokens":12}}`+"\n\ndata: [DONE]\n\n")
						return
					}
					_, _ = io.WriteString(w, "data: "+`{"type":"response.created","response":{"id":"resp_fallback","status":"in_progress","output":[],"usage":null}}`+"\n\n"+
						"data: "+`{"type":"response.output_text.delta","delta":"fallback-ok"}`+"\n\n"+
						"data: "+`{"type":"response.completed","response":{"id":"resp_fallback","status":"completed","output":[],"usage":{"input_tokens":7,"output_tokens":5,"total_tokens":12}}}`+"\n\n")
				}))
				t.Cleanup(fallback.Close)
				t.Cleanup(cancel)
				if tc.cancelOnClose || tc.cancelBeforeSend {
					client := service.GetHttpClient()
					previous := client.Transport
					base := previous
					if base == nil {
						base = http.DefaultTransport
					}
					origin := primary.URL
					if tc.cancelBeforeSend {
						origin = fallback.URL
					}
					client.Transport = &responsesRetryCancelTransport{RoundTripper: base, origin: origin, cancel: cancel, beforeSend: tc.cancelBeforeSend}
					t.Cleanup(func() { client.Transport = previous })
				}
				insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 2401, baseURL: primary.URL, priority: 100, models: []string{modelName}})
				insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 2402, baseURL: fallback.URL, priority: 0, models: []string{modelName}})
				if tc.azureFallback {
					require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2402).Update("type", constant.ChannelTypeAzure).Error)
				}
				if tc.paramOverride != "" {
					require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2401).Update("param_override", tc.paramOverride).Error)
				}
				model.InitChannelCache()
				path, body := "/v1/responses", `{"model":"`+modelName+`","input":"hello","reasoning":{"effort":"high"},"tools":[{"type":"function","name":"lookup","parameters":{"type":"object","properties":{}}}],"stream":true}`
				if format == types.RelayFormatOpenAI {
					path, body = "/v1/chat/completions", `{"model":"`+modelName+`","messages":[{"role":"user","content":"hello"}],"reasoning_effort":"high","tools":[{"type":"function","function":{"name":"lookup","parameters":{"type":"object","properties":{}}}}],"stream":true}`
				}
				if tc.continuation {
					body = strings.TrimSuffix(body, "}") + `,"previous_response_id":"resp_previous"}`
				}
				if tc.itemReference {
					body = strings.Replace(body, `"input":"hello"`, `"input":[{"type":"item_reference","id":"msg_server"}]`, 1)
				}
				engine := gin.New()
				engine.POST(path, func(c *gin.Context) {
					common.SetContextKey(c, constant.ContextKeyUserId, 42)
					common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
					common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
					common.SetContextKey(c, constant.ContextKeyUserQuota, 1_000_000)
					common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
					common.SetContextKey(c, constant.ContextKeyTokenId, 43)
					common.SetContextKey(c, constant.ContextKeyTokenKey, "sse-retry-token")
					common.SetContextKey(c, constant.ContextKeyTokenGroup, "default")
					common.SetContextKey(c, constant.ContextKeyTokenUnlimited, true)
					c.Set("username", "responses-ws-test")
					c.Set("token_name", "sse-retry-token")
					if tc.pin {
						service.GetChannelConstraints(c).AddPin(hostdto.ChannelPin{ChannelId: 2401, Source: hostdto.PinSourceToken, Rank: hostdto.PinRankToken, RetryMode: hostdto.PinRetrySingleAttempt})
					}
				}, middleware.Distribute(), func(c *gin.Context) { Relay(c, format) })
				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(requestContext)
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("X-Session", t.Name())
				request.Header.Set("User-Agent", "responses-retry-test")
				if tc.requestState {
					request.Header.Set("X-Codex-Turn-State", "client-state")
				}
				if tc.writeFailure {
					engine.ServeHTTP(&responsesRetryFailingWriter{ResponseRecorder: recorder}, request)
				} else {
					engine.ServeHTTP(recorder, request)
				}
				require.Equal(t, int32(1), primaryCalls.Load(), recorder.Body.String())
				wantFallback := int32(0)
				if tc.fallback {
					wantFallback = 1
				}
				require.Equal(t, wantFallback, fallbackCalls.Load(), recorder.Body.String())
				if tc.fallback {
					assert.Equal(t, http.StatusOK, recorder.Code)
					assert.Contains(t, recorder.Body.String(), "fallback-ok")
					assert.NotContains(t, recorder.Body.String(), "resp_primary")
					assert.NotContains(t, recorder.Body.String(), "rate_limit_exceeded")
					require.Len(t, requests, 2)
					primaryRequest, fallbackRequest := <-requests, <-requests
					if tc.chatFallback {
						var primaryPayload, fallbackPayload map[string]any
						require.NoError(t, common.UnmarshalJsonStr(primaryRequest, &primaryPayload))
						require.NoError(t, common.UnmarshalJsonStr(fallbackRequest, &fallbackPayload))
						assert.Equal(t, primaryPayload["model"], fallbackPayload["model"])
						assert.Contains(t, primaryPayload, "input")
						assert.Contains(t, fallbackPayload, "messages")
					} else {
						assert.JSONEq(t, primaryRequest, fallbackRequest, "fallback must replay the same model and request")
					}
				}
				if tc.cancelOnClose {
					assert.Equal(t, "0", recorder.Header().Get("X-AI-Cove-Retry-Count"))
				}
				if tc.wantText != "" {
					assert.Contains(t, recorder.Body.String(), tc.wantText)
				}
				var logs []model.Log
				require.NoError(t, db.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
				assert.LessOrEqual(t, len(logs), 1, "only one attempt may be settled")
				if tc.writeFailure {
					require.Len(t, logs, 1)
					var details struct {
						StreamStatus struct {
							Status     string `json:"status"`
							EndReason  string `json:"end_reason"`
							ErrorCount int    `json:"error_count"`
						} `json:"stream_status"`
					}
					require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &details))
					assert.Equal(t, "error", details.StreamStatus.Status)
					assert.Equal(t, "eof", details.StreamStatus.EndReason)
					assert.Positive(t, details.StreamStatus.ErrorCount)
				}
				charged := 0
				for _, log := range logs {
					charged += log.Quota
				}
				assert.LessOrEqual(t, charged, 5000)
				if tc.settled {
					require.Len(t, logs, 1)
					assert.Equal(t, 5000, charged)
					wantChannel := 2401
					if tc.fallback {
						wantChannel = 2402
					}
					assert.Equal(t, wantChannel, logs[0].ChannelId)
				}
				if tc.noBudget || tc.pin || tc.strict || tc.cancelOnClose || tc.cancelBeforeSend {
					assert.Empty(t, logs)
					assert.Zero(t, charged)
				}
				if tc.hold && !tc.cancel {
					require.Len(t, logs, 1)
					assert.Contains(t, logs[0].Other, `"end_reason":"timeout"`)
				}
				var user model.User
				require.Eventually(t, func() bool {
					return db.First(&user, 42).Error == nil && user.UsedQuota == charged && user.Quota == 1_000_000-charged
				}, 5*time.Second, time.Millisecond, "asynchronous refunds must restore the exact balance")
			})
		}
	}
}

type responsesRetryFailingWriter struct{ *httptest.ResponseRecorder }

func (w *responsesRetryFailingWriter) Write([]byte) (int, error) {
	return 0, errors.New("downstream write failed")
}
func (w *responsesRetryFailingWriter) WriteString(string) (int, error) {
	return 0, errors.New("downstream write failed")
}

type responsesRetryCancelTransport struct {
	http.RoundTripper
	origin     string
	cancel     context.CancelFunc
	beforeSend bool
}

func (t *responsesRetryCancelTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	matched := request.URL.Scheme+"://"+request.URL.Host == t.origin
	if matched && t.beforeSend {
		t.cancel()
	}
	response, err := t.RoundTripper.RoundTrip(request)
	if err == nil && matched && !t.beforeSend {
		response.Body = &responsesRetryCancelBody{ReadCloser: response.Body, cancel: t.cancel}
	}
	return response, err
}

type responsesRetryCancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *responsesRetryCancelBody) Close() error {
	b.cancel()
	return b.ReadCloser.Close()
}
