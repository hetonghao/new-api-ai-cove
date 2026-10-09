package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
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
		InputTokens:        11,
		OutputTokens:       7,
		InputTokensDetails: &dto.InputTokenDetails{CachedTokens: 3},
		PromptTokens:       11,
		CompletionTokens:   7,
		TotalTokens:        18,
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

func TestResponsesWebSocketUsageTrackerRetainsImagesAfterFailure(t *testing.T) {
	for _, terminalType := range []string{"response.failed", "response.incomplete", "response.cancelled"} {
		t.Run(terminalType, func(t *testing.T) {
			info := &relaycommon.RelayInfo{OriginModelName: "gpt-5.4"}
			tracker := NewResponsesWebSocketUsageTracker(info)
			_, _, err := tracker.Observe([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"type":"image_generation_call","id":"img_1","status":"completed","result":"image-data"}}`))
			require.NoError(t, err)
			terminal, _, err := tracker.Observe([]byte(`{"type":"` + terminalType + `","response":{"output":[{"type":"image_generation_call","id":"img_1","status":"completed","result":"image-data"}]}}`))
			require.NoError(t, err)
			require.True(t, terminal)
			require.False(t, tracker.Succeeded())
			require.Equal(t, 1, info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolImageGeneration].CallCount)
		})
	}
}

func TestResponsesWebSocketUsageTrackerUsesVendorSearchCounts(t *testing.T) {
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.1",
		RelayMode:       relayconstant.RelayModeResponses,
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeAzure, UpstreamModelName: "gpt-5.1"},
	}
	adaptor := &Adaptor{}
	adaptor.Init(info)
	tracker := NewResponsesWebSocketUsageTracker(info)
	_, _, err := tracker.Observe([]byte(`{"type":"response.output_item.done","item":{"type":"web_search_call","id":"search_1"}}`))
	require.NoError(t, err)
	terminal, _, err := tracker.Observe([]byte(`{"type":"response.completed","response":{"status":"completed","tool_usage":{"web_search":{"num_requests":5}}}}`))
	require.NoError(t, err)
	require.True(t, terminal)
	require.Contains(t, info.ResponsesUsageInfo.BuiltInTools, "bing_web_search")
	require.Equal(t, 5, info.ResponsesUsageInfo.BuiltInTools["bing_web_search"].CallCount)
	for name, tool := range info.ResponsesUsageInfo.BuiltInTools {
		if name != "bing_web_search" {
			require.Zero(t, tool.CallCount, name)
		}
	}
}
