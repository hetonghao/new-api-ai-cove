package common

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	napicommon "github.com/QuantumNous/new-api/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDeepSeekToolReasoningDiagnosticsFlagToolRunWithoutReasoning(t *testing.T) {
	body := deepSeekResponsesBody(t, []map[string]any{
		{"type": "message", "role": "user", "content": "hi"},
		{"type": "reasoning", "content": []map[string]any{{"type": "reasoning_text", "text": "need pwd"}}},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		{"type": "reasoning", "content": []map[string]any{{"type": "reasoning_text", "text": ""}}},
		{"type": "function_call", "call_id": "call-2", "name": "apply_patch", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-2", "output": "ok"},
	})

	diagnostics, ok := DeepSeekToolReasoningDiagnosticsOfBody(body)
	require.True(t, ok)
	require.Equal(t, 7, diagnostics.Items)
	require.Equal(t, 2, diagnostics.Runs)
	require.Equal(t, 2, diagnostics.Calls)
	require.Equal(t, 2, diagnostics.Outputs)
	require.Equal(t, 2, diagnostics.Reasoning)
	require.Equal(t, 1, diagnostics.EmptyReasoning)
	require.Equal(t, 1, diagnostics.MissingReasoningTotal)
	require.Equal(t, []string{"call-2@5:apply_patch"}, diagnostics.MissingReasoning)
	require.True(t, diagnostics.NeedsReasoning())
	require.Contains(t, diagnostics.String(), "missing_reasoning=1[call-2@5:apply_patch]")
	require.Contains(t, diagnostics.String(), fmt.Sprintf("bytes=%d", len(body)))
}

func TestDeepSeekToolReasoningDiagnosticsFlagPairingDefects(t *testing.T) {
	input := deepSeekRawInput(t, []map[string]any{
		{"type": "function_call_output", "call_id": "foreign-call", "output": "x"},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command"},
		{"type": "message", "role": "assistant", "content": "narrating between call and output"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		{"type": "function_call", "call_id": "call-2", "name": "exec_command"},
	})

	diagnostics := DeepSeekToolReasoningDiagnosticsOfInput(input)
	require.Equal(t, 1, diagnostics.OrphanOutputs)
	require.Equal(t, 1, diagnostics.UnpairedCalls)
	require.Equal(t, 1, diagnostics.Interleaved)
	require.Equal(t, 2, diagnostics.Runs)
	require.Equal(t, 2, diagnostics.MissingReasoningTotal)
	require.Contains(t, diagnostics.String(), "unpaired_calls=1 orphan_outputs=1 interleaved=1")
}

func TestDeepSeekFailureCaptureAppliesOnlyToDeepSeekResponsesRelays(t *testing.T) {
	c := newDeepSeekTestContext(t)

	require.Nil(t, NewDeepSeekFailureCapture(c, nil, nil))
	require.Nil(t, NewDeepSeekFailureCapture(c, &RelayInfo{
		RelayMode:       relayconstant.RelayModeResponses,
		OriginModelName: "gpt-5.5",
	}, nil))
	require.Nil(t, NewDeepSeekFailureCapture(c, &RelayInfo{
		RelayMode:       relayconstant.RelayModeChatCompletions,
		OriginModelName: "deepseek-v4.1-flash",
	}, nil))
	require.NotNil(t, NewDeepSeekFailureCapture(c, &RelayInfo{
		RelayMode:       relayconstant.RelayModeResponses,
		OriginModelName: "deepseek-v4.1-flash",
	}, nil))
	require.NotNil(t, NewDeepSeekFailureCapture(c, &RelayInfo{
		RelayMode:       relayconstant.RelayModeResponsesCompact,
		OriginModelName: "deepseek-v4.1-flash",
	}, nil))

	var missing *DeepSeekFailureCapture
	missing.ObserveOutbound([]byte(`{"input":[]}`))
	missing.ReportFailure(&http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader("boom"))})
}

func TestDeepSeekFailureCaptureDumpsPayloadAndKeepsErrorBody(t *testing.T) {
	resetDeepSeekFailureDumpState()
	logs := captureDeepSeekErrorLogs(t)
	c := newDeepSeekTestContext(t)
	info := &RelayInfo{
		ChannelMeta:     &ChannelMeta{ChannelId: 7, UpstreamModelName: "deepseek-chat"},
		RelayMode:       relayconstant.RelayModeResponses,
		OriginModelName: "deepseek-v4.1-flash",
	}
	clientInput := deepSeekRawInput(t, []map[string]any{
		{"type": "message", "role": "user", "content": "hi"},
		{"type": "function_call", "call_id": "call-2", "name": "apply_patch"},
		{"type": "function_call_output", "call_id": "call-2", "output": "ok"},
	})
	body := deepSeekResponsesBody(t, []map[string]any{
		{"type": "message", "role": "user", "content": "hi"},
		{"type": "function_call", "call_id": "call-2", "name": "apply_patch"},
		{"type": "function_call_output", "call_id": "call-2", "output": "ok"},
	})
	capture := NewDeepSeekFailureCapture(c, info, clientInput)
	capture.ObserveOutbound(body)

	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"No tool call found for tool output call-2"}}`)),
	}
	capture.ReportFailure(resp)

	remaining, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(remaining), "No tool call found for tool output call-2")

	out := logs.String()
	require.Contains(t, out, "deepseek_tool_reasoning precondition")
	require.Contains(t, out, "deepseek_upstream_failure status=400 channel=7")
	require.Contains(t, out, `model="deepseek-v4.1-flash" upstream_model="deepseek-chat"`)
	require.Contains(t, out, "client[items=3")
	require.Contains(t, out, "sent[items=3")
	require.Contains(t, out, "No tool call found for tool output call-2")
	require.Contains(t, out, "deepseek_upstream_failure payload fingerprint=")
	require.Contains(t, out, `"call_id":"call-2"`)
	require.Contains(t, out, "deepseek_upstream_failure payload_end")
}

func TestDeepSeekFailureCaptureDeduplicatesRepeatedPayload(t *testing.T) {
	resetDeepSeekFailureDumpState()
	logs := captureDeepSeekErrorLogs(t)
	c := newDeepSeekTestContext(t)
	info := &RelayInfo{
		ChannelMeta:     &ChannelMeta{ChannelId: 7},
		RelayMode:       relayconstant.RelayModeResponses,
		OriginModelName: "deepseek-v4.1-flash",
	}
	input := deepSeekRawInput(t, []map[string]any{
		{"type": "function_call", "call_id": "call-9", "name": "apply_patch"},
		{"type": "function_call_output", "call_id": "call-9", "output": "ok"},
	})
	body := deepSeekResponsesBody(t, []map[string]any{
		{"type": "function_call", "call_id": "call-9", "name": "apply_patch"},
		{"type": "function_call_output", "call_id": "call-9", "output": "ok"},
	})

	for range 2 {
		capture := NewDeepSeekFailureCapture(c, info, input)
		capture.ObserveOutbound(body)
		capture.ReportFailure(&http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"invalid request"}}`)),
		})
	}

	out := logs.String()
	require.Equal(t, 1, strings.Count(out, "deepseek_upstream_failure payload fingerprint="))
	require.Equal(t, 2, strings.Count(out, "deepseek_upstream_failure status=400"))
	require.Contains(t, out, "payload=suppressed reason=duplicate_payload")
}

func captureDeepSeekErrorLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	previous := gin.DefaultErrorWriter
	buffer := &bytes.Buffer{}
	gin.DefaultErrorWriter = buffer
	t.Cleanup(func() {
		gin.DefaultErrorWriter = previous
	})
	return buffer
}

func newDeepSeekTestContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c
}

func deepSeekResponsesBody(t *testing.T, items []map[string]any) []byte {
	t.Helper()
	body, err := napicommon.Marshal(map[string]any{
		"model": "deepseek-v4.1-flash",
		"input": items,
	})
	require.NoError(t, err)
	return body
}

func deepSeekRawInput(t *testing.T, items []map[string]any) json.RawMessage {
	t.Helper()
	raw, err := napicommon.Marshal(items)
	require.NoError(t, err)
	return raw
}
