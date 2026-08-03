package openai

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
)

type ResponsesWebSocketUsageTracker struct {
	info         *relaycommon.RelayInfo
	responseText strings.Builder
	imageCounter relaycommon.ImageGenerationCallCounter
	seenOutputs  map[string]struct{}
	done         bool
	succeeded    bool
}

func NewResponsesWebSocketUsageTracker(info *relaycommon.RelayInfo) *ResponsesWebSocketUsageTracker {
	return &ResponsesWebSocketUsageTracker{info: info}
}

func (t *ResponsesWebSocketUsageTracker) Observe(payload []byte) (bool, *dto.Usage, error) {
	if t.done {
		return false, nil, nil
	}
	var event dto.ResponsesStreamResponse
	if err := common.Unmarshal(payload, &event); err != nil {
		return false, nil, err
	}

	switch event.Type {
	case "response.output_text.delta":
		t.responseText.WriteString(event.Delta)
	case dto.ResponsesOutputTypeItemDone:
		t.observeOutput(event.Item, event.OutputIndex)
	case "response.completed", "response.done":
		t.done = true
		t.succeeded = true
		t.observeCompletedResponse(event.Response)
		t.imageCounter.Commit(t.info)
		return true, t.usage(event.Response), nil
	case "response.failed", "response.incomplete", "response.cancelled", "response.canceled", "error":
		t.done = true
		t.imageCounter.Reset()
		t.imageCounter.Commit(t.info)
		return true, t.usage(event.Response), nil
	}
	return false, nil, nil
}

func (t *ResponsesWebSocketUsageTracker) Succeeded() bool {
	return t != nil && t.done && t.succeeded
}

func (t *ResponsesWebSocketUsageTracker) observeCompletedResponse(response *dto.OpenAIResponsesResponse) {
	if response == nil {
		return
	}
	for i := range response.Output {
		index := i
		t.observeOutput(&response.Output[i], &index)
	}
}

func (t *ResponsesWebSocketUsageTracker) observeOutput(output *dto.ResponsesOutput, outputIndex *int) {
	if output == nil || t.info == nil {
		return
	}
	aliases := make([]string, 0, 3)
	if output.ID != "" {
		aliases = append(aliases, "id:"+output.ID)
	}
	if output.CallId != "" {
		aliases = append(aliases, "call:"+output.CallId)
	}
	if outputIndex != nil && *outputIndex >= 0 {
		aliases = append(aliases, fmt.Sprintf("index:%d", *outputIndex))
	}
	if len(aliases) > 0 {
		if t.seenOutputs == nil {
			t.seenOutputs = make(map[string]struct{})
		}
		seen := false
		for _, alias := range aliases {
			if _, ok := t.seenOutputs[alias]; ok {
				seen = true
			}
			t.seenOutputs[alias] = struct{}{}
		}
		if seen {
			return
		}
	}
	switch output.Type {
	case dto.BuildInCallWebSearchCall:
		t.info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
	case dto.BuildInCallFileSearchCall:
		t.info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
	case dto.BuildInCallFunctionCall:
		t.info.CountBillableToolCall(dto.BuildInCallFunctionCall, output.Name)
	case dto.ResponsesOutputTypeImageGenerationCall:
		t.imageCounter.Observe(output, outputIndex)
	}
}

func (t *ResponsesWebSocketUsageTracker) usage(response *dto.OpenAIResponsesResponse) *dto.Usage {
	usage := &dto.Usage{}
	if response != nil && response.Usage != nil {
		usage.PromptTokens = response.Usage.InputTokens
		usage.CompletionTokens = response.Usage.OutputTokens
		usage.TotalTokens = response.Usage.TotalTokens
		if response.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = response.Usage.InputTokensDetails.CachedTokens
			usage.PromptTokensDetails.CacheWriteTokens = response.Usage.InputTokensDetails.CacheWriteTokens
		}
	}
	if usage.CompletionTokens == 0 && t.responseText.Len() > 0 && t.info != nil {
		modelName := t.info.GetUpstreamModelName()
		if modelName == "" {
			modelName = t.info.OriginModelName
		}
		usage.CompletionTokens = service.CountTextToken(t.responseText.String(), modelName)
	}
	if usage.PromptTokens == 0 && usage.CompletionTokens > 0 && t.info != nil {
		usage.PromptTokens = t.info.GetEstimatePromptTokens()
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return usage
}
