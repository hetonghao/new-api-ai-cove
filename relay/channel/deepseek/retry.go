package deepseek

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

// reasoningRetryTrigger is the upstream wording for a thinking-mode turn whose
// reasoning_text never reached it.
const reasoningRetryTrigger = "reasoning_text"

// loadRetryReasoning is a seam for tests; production reads the DeepSeek
// reasoning cache.
var loadRetryReasoning = relaycommon.LoadDeepSeekReasoning

// buildReasoningRetryBody rebuilds a turn the upstream refused with the
// thinking-mode reasoning_text error, filling reasoning_text everywhere the
// upstream could look for it. It returns false when the rejection is about
// something else, or when there is nothing more to repair, so the caller can
// keep the original response untouched.
func buildReasoningRetryBody(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) ([]byte, bool) {
	if resp == nil || resp.StatusCode != http.StatusBadRequest {
		return nil, false
	}
	upstreamErr := relaycommon.ReadAndRestoreResponseBody(resp)
	if !strings.Contains(strings.ToLower(string(upstreamErr)), reasoningRetryTrigger) {
		return nil, false
	}
	request, ok := relaycommon.StashedDeepSeekResponsesRequest(c)
	if !ok {
		return nil, false
	}
	return reasoningRetryPayload(c, info, request, loadRetryReasoning(info, ""))
}

// reasoningRetryPayload rebuilds the rejected request around a fully repaired
// input and re-runs the same post-processing the original payload went through,
// so an operator's param override still applies to the retry.
func reasoningRetryPayload(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest, cached string) ([]byte, bool) {
	repaired, changed := forceRepairDeepSeekReasoning(request.Input, cached)
	if !changed {
		return nil, false
	}
	request.Input = repaired
	raw, err := common.Marshal(request)
	if err != nil {
		return nil, false
	}
	if info != nil {
		raw, err = relaycommon.RemoveDisabledFields(raw, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
		if err != nil {
			return nil, false
		}
		if len(info.ParamOverride) > 0 {
			raw, err = relaycommon.ApplyParamOverrideWithRelayInfo(raw, info)
			if err != nil {
				return nil, false
			}
		}
	}
	// The repaired payload is the turn that succeeds, so it has to be the one a
	// later continuation replays from.
	relaycommon.StashDeepSeekResponsesInput(c, repaired)
	if diagnostics, ok := relaycommon.DeepSeekToolReasoningDiagnosticsOfBody(raw); ok {
		logger.LogWarn(c, fmt.Sprintf("deepseek_reasoning_retry rebuilt sent[%s]", diagnostics))
	}
	return raw, true
}
