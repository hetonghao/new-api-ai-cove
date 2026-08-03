package openai

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

func TestResponsesWebSocketUsageTrackerUsesCompletedUsageOnce(t *testing.T) {
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.4",
		ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{
			BuiltInTools: map[string]*relaycommon.BuildInToolInfo{},
		},
	}
	tracker := NewResponsesWebSocketUsageTracker(info)

	terminal, usage, err := tracker.Observe([]byte(`{"type":"response.output_item.done","item":{"type":"web_search_call"}}`))
	require.NoError(t, err)
	require.False(t, terminal)
	require.Nil(t, usage)

	terminal, usage, err = tracker.Observe([]byte(`{"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18,"input_tokens_details":{"cached_tokens":3}}}}`))
	require.NoError(t, err)
	require.True(t, terminal)
	require.Equal(t, &dto.Usage{
		PromptTokens:     11,
		CompletionTokens: 7,
		TotalTokens:      18,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 3,
		},
	}, usage)
	require.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview].CallCount)

	terminal, usage, err = tracker.Observe([]byte(`{"type":"response.completed","response":{"usage":{"input_tokens":99,"output_tokens":99,"total_tokens":198}}}`))
	require.NoError(t, err)
	require.False(t, terminal)
	require.Nil(t, usage)
}

func TestResponsesWebSocketUsageTrackerDeduplicatesCompletedOutputAfterItemDone(t *testing.T) {
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.4",
		ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{
			BuiltInTools: map[string]*relaycommon.BuildInToolInfo{},
		},
	}
	tracker := NewResponsesWebSocketUsageTracker(info)

	terminal, _, err := tracker.Observe([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"type":"web_search_call","id":"ws_1"}}`))
	require.NoError(t, err)
	require.False(t, terminal)

	terminal, _, err = tracker.Observe([]byte(`{"type":"response.completed","response":{"status":"completed","output":[{"type":"web_search_call","id":"ws_1"}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`))
	require.NoError(t, err)
	require.True(t, terminal)
	require.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview].CallCount)
}

func TestResponsesWebSocketUsageTrackerFallsBackToObservedText(t *testing.T) {
	service.InitTokenEncoders()
	info := &relaycommon.RelayInfo{OriginModelName: "gpt-5.4"}
	info.SetEstimatePromptTokens(5)
	tracker := NewResponsesWebSocketUsageTracker(info)

	_, _, err := tracker.Observe([]byte(`{"type":"response.output_text.delta","delta":"hello world"}`))
	require.NoError(t, err)
	terminal, usage, err := tracker.Observe([]byte(`{"type":"response.failed","response":{"status":"failed"}}`))
	require.NoError(t, err)
	require.True(t, terminal)
	require.NotNil(t, usage)
	require.Equal(t, 5, usage.PromptTokens)
	require.Greater(t, usage.CompletionTokens, 0)
	require.Equal(t, usage.PromptTokens+usage.CompletionTokens, usage.TotalTokens)
}
