package deepseek

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := claude.Adaptor{}
	convertedRequest, err := adaptor.ConvertClaudeRequest(c, info, req)
	if err != nil {
		return nil, err
	}
	claudeRequest, ok := convertedRequest.(*dto.ClaudeRequest)
	if !ok {
		return convertedRequest, nil
	}
	if err := applyDeepSeekV4ClaudeThinkingSuffix(info, claudeRequest); err != nil {
		return nil, err
	}
	return claudeRequest, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	fimBaseUrl := info.ChannelBaseUrl
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		return fmt.Sprintf("%s/anthropic/v1/messages", info.ChannelBaseUrl), nil
	default:
		if !strings.HasSuffix(info.ChannelBaseUrl, "/beta") {
			fimBaseUrl += "/beta"
		}
		switch info.RelayMode {
		case constant.RelayModeCompletions:
			return fmt.Sprintf("%s/completions", fimBaseUrl), nil
		case constant.RelayModeResponses:
			return fmt.Sprintf("%s/responses", info.ChannelBaseUrl), nil
		default:
			return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
		}
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if err := applyDeepSeekV4OpenAIThinkingSuffix(info, request); err != nil {
		return nil, err
	}

	return request, nil
}

func applyDeepSeekV4OpenAIThinkingSuffix(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) error {
	modelName := request.Model
	if info != nil && info.ChannelMeta != nil && info.UpstreamModelName != "" {
		modelName = info.UpstreamModelName
	}
	if model_setting.ShouldPreserveThinkingSuffix(modelName) || info != nil && model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) {
		return nil
	}
	baseModel, thinkingType, effort, ok := reasoning.ParseDeepSeekV4ThinkingSuffix(modelName)
	if !ok {
		return nil
	}
	thinking, err := common.Marshal(map[string]string{
		"type": thinkingType,
	})
	if err != nil {
		return fmt.Errorf("error marshalling thinking: %w", err)
	}
	request.Model = baseModel
	request.THINKING = thinking
	request.ReasoningEffort = effort
	if info != nil {
		if info.ChannelMeta != nil {
			info.UpstreamModelName = baseModel
		}
		info.SetReasoningEffort(effort)
	}
	return nil
}

func applyDeepSeekV4ClaudeThinkingSuffix(info *relaycommon.RelayInfo, request *dto.ClaudeRequest) error {
	modelName := request.Model
	if info != nil && info.ChannelMeta != nil && info.UpstreamModelName != "" {
		modelName = info.UpstreamModelName
	}
	if model_setting.ShouldPreserveThinkingSuffix(modelName) || info != nil && model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) {
		return nil
	}
	baseModel, thinkingType, effort, ok := reasoning.ParseDeepSeekV4ThinkingSuffix(modelName)
	if !ok {
		return nil
	}
	request.Model = baseModel
	request.Thinking = &dto.Thinking{Type: thinkingType}
	if effort == "" {
		request.OutputConfig = nil
	} else {
		outputConfig, err := common.Marshal(map[string]string{
			"effort": effort,
		})
		if err != nil {
			return fmt.Errorf("error marshalling output_config: %w", err)
		}
		request.OutputConfig = outputConfig
	}
	if info != nil {
		if info.ChannelMeta != nil {
			info.UpstreamModelName = baseModel
		}
		info.SetReasoningEffort(effort)
	}
	return nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	applyDeepSeekV4ResponsesThinkingSuffix(info, &request)
	previousResponseID := strings.TrimSpace(request.PreviousResponseID)
	if previousResponseID != "" {
		// OpenCode does not expand previous_response_id. Replay stored history
		// for incremental Codex continuations, then stop mutating item identity.
		if in, out, ok := relaycommon.LoadDeepSeekHistory(previousResponseID); ok && shouldExpandDeepSeekHistory(in, request.Input) {
			request.Input = mergeDeepSeekHistory(in, out, request.Input)
			request.PreviousResponseID = ""
		}
	} else {
		request.Input = dropUnpairedDeepSeekToolCalls(request.Input)
	}
	// Canonicalize after the replay so an output whose call lives in the replayed
	// history is not mistaken for an orphan and dropped.
	request.Input = canonicalizeDeepSeekToolRuns(request.Input)
	// Both paths share one reasoning pass, and it only touches the trailing tool
	// run: items that already went upstream stay byte-identical for the cache.
	injectCachedDeepSeekReasoning(info, &request, previousResponseID)
	request.Input = canonicalizeDeepSeekToolRuns(request.Input)
	relaycommon.StashDeepSeekResponsesInput(c, request.Input)
	relaycommon.StashDeepSeekResponsesRequest(c, request)
	return request, nil
}

type deepSeekToolPairItem struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
}

func dropUnpairedDeepSeekToolCalls(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return input
	}
	var items []json.RawMessage
	if err := common.Unmarshal(input, &items); err != nil {
		return input
	}
	outputIDs := make(map[string]struct{})
	for _, item := range items {
		var peek deepSeekToolPairItem
		if common.Unmarshal(item, &peek) != nil {
			continue
		}
		if peek.Type != "function_call_output" && peek.Type != "custom_tool_call_output" {
			continue
		}
		if peek.CallID != "" {
			outputIDs[peek.CallID] = struct{}{}
		}
	}
	kept := make([]json.RawMessage, 0, len(items))
	changed := false
	for _, item := range items {
		var peek deepSeekToolPairItem
		if common.Unmarshal(item, &peek) != nil {
			kept = append(kept, item)
			continue
		}
		if peek.Type == "function_call" || peek.Type == "custom_tool_call" {
			if _, ok := outputIDs[peek.CallID]; !ok {
				changed = true
				continue
			}
		}
		kept = append(kept, item)
	}
	if !changed {
		return input
	}
	raw, err := common.Marshal(kept)
	if err != nil {
		return input
	}
	return raw
}

func applyDeepSeekV4ResponsesThinkingSuffix(info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest) {
	modelName := request.Model
	if info != nil && info.ChannelMeta != nil && info.UpstreamModelName != "" {
		modelName = info.UpstreamModelName
	}
	if model_setting.ShouldPreserveThinkingSuffix(modelName) || info != nil && model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) {
		return
	}
	baseModel, thinkingType, effort, ok := reasoning.ParseDeepSeekV4ThinkingSuffix(modelName)
	if ok {
		if thinkingType == "disabled" {
			effort = "none"
		}
		request.Model = baseModel
		if request.Reasoning == nil {
			request.Reasoning = &dto.Reasoning{}
		}
		request.Reasoning.Effort = effort
		if info != nil && info.ChannelMeta != nil {
			info.UpstreamModelName = baseModel
		}
	}
	if info != nil && request.Reasoning != nil {
		info.SetReasoningEffort(request.Reasoning.Effort)
	}
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	resp, err := channel.DoApiRequest(a, c, info, requestBody)
	if err != nil || resp == nil || resp.StatusCode != http.StatusBadRequest {
		return resp, err
	}
	// The upstream rejects a thinking-mode turn that lost its reasoning_text.
	// Send that one turn again with the reasoning filled in instead of failing it.
	retryBody, ok := buildReasoningRetryBody(c, info, resp)
	if !ok {
		return resp, nil
	}
	body, closer, err := relaycommon.NewOutboundJSONBody(retryBody)
	if err != nil {
		return resp, nil
	}
	defer closer.Close()
	retryResp, retryErr := channel.DoApiRequest(a, c, info, body)
	if retryErr != nil || retryResp == nil {
		logger.LogError(c, fmt.Sprintf("deepseek_reasoning_retry resend failed: %v", retryErr))
		return resp, nil
	}
	logger.LogWarn(c, fmt.Sprintf("deepseek_reasoning_retry status=%d bytes=%d", retryResp.StatusCode, len(retryBody)))
	if retryResp.StatusCode != http.StatusOK {
		// The rebuilt payload is the interesting one when it is refused too.
		relaycommon.DumpDeepSeekPayload(c, "deepseek_reasoning_retry payload", retryBody)
	}
	service.CloseResponseBodyGracefully(resp)
	return retryResp, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		adaptor := claude.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	default:
		adaptor := openai.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
