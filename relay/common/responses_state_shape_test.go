package common

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 线上真实文案：渠道 62（普通上游中转站）实测返回。
const (
	upstreamEmptyInputMessage       = `{"error":{"message":"One of \"input\" or \"previous_response_id\" or 'prompt' or 'conversation' must be provided.","type":"invalid_request_error"}}`
	upstreamMissingInputMessage     = `{"error":{"message":"input is required (request id: 202609161811159512742138268d9d6Ns0fGaO7)","type":"new_api_error","code":"invalid_request"}}`
	upstreamHttpContinuationMessage = `{"error":{"message":"previous_response_id is not available for this user; continuation via previous_response_id is only supported on Responses WebSocket v2","type":"invalid_request_error"}}`
)

func TestDetectResponsesStateShapeFromError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		body   string
		want   responsesStateShape
	}{
		// 这条文案同时包含 previous_response_id，因此它同时是判定顺序的回归用例。
		{"empty input message", http.StatusBadRequest, upstreamEmptyInputMessage, responsesStateShapeInput},
		{"missing input message", http.StatusBadRequest, upstreamMissingInputMessage, responsesStateShapeInput},
		{"http continuation message", http.StatusBadRequest, upstreamHttpContinuationMessage, responsesStateShapeContinuation},
		{"unrelated bad request", http.StatusBadRequest, `{"error":{"message":"invalid api key"}}`, responsesStateShapeNone},
		// 只提到 must be provided、但不涉及 input 的 400 不算输入形态。
		{"other must be provided message", http.StatusBadRequest, `{"error":{"message":"model must be provided once"}}`, responsesStateShapeNone},
		// 失效的 response id 是一次性可恢复错误，不能据此判定渠道不支持续传。
		{"stale response id message", http.StatusBadRequest, `{"error":{"message":"previous_response_not_found","code":"previous_response_not_found"}}`, responsesStateShapeNone},
		{"non 400 status is not learned", http.StatusTooManyRequests, upstreamEmptyInputMessage, responsesStateShapeNone},
		{"success body is not learned", http.StatusOK, upstreamEmptyInputMessage, responsesStateShapeNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, detectResponsesStateShapeFromError(tt.status, []byte(tt.body)))
		})
	}
}

func TestEmptyResponsesInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"missing input", ``, true},
		{"empty array", `[]`, true},
		{"padded empty array", "  [ ]  ", true},
		{"null", `null`, true},
		{"empty object", `{}`, true},
		{"empty string", `""`, true},
		{"blank string", `"   "`, true},
		{"one item", `[{"type":"message","role":"user"}]`, false},
		{"non empty string", `"hello"`, false},
		{"object with fields", `{"id":"conv_1"}`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, emptyResponsesInput(json.RawMessage(tt.input)))
		})
	}
}

func TestResponsesStateShapeRejection(t *testing.T) {
	t.Parallel()

	none := func(responsesStateShape) bool { return false }
	continuationOnly := func(shape responsesStateShape) bool { return shape == responsesStateShapeContinuation }
	inputOnly := func(shape responsesStateShape) bool { return shape == responsesStateShapeInput }

	tests := []struct {
		name               string
		unsupported        func(responsesStateShape) bool
		previousResponseID string
		input              string
		wantRejected       bool
	}{
		{"no learned verdict still calls upstream", none, "resp_1", `[{"type":"message"}]`, false},
		{"learned continuation rejects previous_response_id", continuationOnly, "resp_1", `[]`, true},
		{"learned continuation keeps a fresh request", continuationOnly, "", `[{"type":"message"}]`, false},
		{"learned empty input rejects empty input", inputOnly, "", `[]`, true},
		{"learned empty input keeps real input", inputOnly, "", `[{"type":"message"}]`, false},
		// 输入形态只说明“没有来源”会被拒；带 previous_response_id 的续传是否可用由续传形态单独判定。
		{"learned empty input does not reject a continuation", inputOnly, "resp_1", `[]`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			apiErr := responsesStateShapeRejection(tt.unsupported, tt.previousResponseID, json.RawMessage(tt.input))
			if !tt.wantRejected {
				assert.Nil(t, apiErr)
				return
			}
			require.NotNil(t, apiErr)
			assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			assert.Equal(t, types.ErrorCodeInvalidRequest, apiErr.GetErrorCode())
		})
	}
}

func TestResponsesStateShapeStoreExpiresVerdicts(t *testing.T) {
	t.Parallel()

	store := newResponsesStateShapeStore()
	now := time.Now()
	store.remember("channel=1:model=m:shape=http_continuation", 30*time.Minute, now)

	assert.True(t, store.remembered("channel=1:model=m:shape=http_continuation", now.Add(29*time.Minute)))
	assert.False(t, store.remembered("channel=1:model=m:shape=http_continuation", now.Add(31*time.Minute)))
}

func TestObserveResponsesUpstreamFailureLearnsShape(t *testing.T) {
	t.Parallel()

	const channelID = 987654
	const model = "responses-state-shape-test-model"

	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(upstreamHttpContinuationMessage)),
	}
	ObserveResponsesUpstreamFailure(nil, channelID, model, resp)

	assert.True(t, responsesStateShapeUnsupported(channelID, model, responsesStateShapeContinuation))
	assert.False(t, responsesStateShapeUnsupported(channelID, model, responsesStateShapeInput))

	// 响应体必须还原：同一个 resp 随后还要交给 RelayErrorHandler 读取。
	restored, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(restored), "previous_response_id")
}
