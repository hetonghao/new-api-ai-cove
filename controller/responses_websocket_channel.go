package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/tidwall/sjson"
)

func prepareFirstResponsesWebSocketRequest(baseCtx *gin.Context, payload []byte, connectionStarted time.Time) (*responsesWebSocketRequestState, []byte, *websocket.Conn, *model.Channel, int64, error) {
	return prepareFirstResponsesWebSocketRequestWithBilling(baseCtx, payload, connectionStarted, nil, nil, nil, nil, nil, 0)
}

func prepareFirstResponsesWebSocketRequestWithBilling(baseCtx *gin.Context, payload []byte, connectionStarted time.Time, billing relaycommon.BillingSettler, rateLimit *middleware.ModelRequestRateLimitTicket, billingInfo *relaycommon.RelayInfo, retryParam *service.RetryParam, excludedChannelIDs map[int]bool, attemptsUsed int) (*responsesWebSocketRequestState, []byte, *websocket.Conn, *model.Channel, int64, error) {
	state, request, err := prepareResponsesWebSocketRequestWithInheritedState(baseCtx, payload, "", rateLimit, billingInfo)
	if err != nil {
		apiErr := asResponsesWebSocketAPIError(err)
		failPreparedResponsesWebSocketRequest(state, nil, apiErr)
		refundResponsesWebSocketBillingIfPending(baseCtx, billing)
		return state, nil, nil, nil, 0, apiErr
	}
	state.info.Billing = billing
	state.billingReused = billing != nil
	if retryParam != nil {
		retryParam.Ctx = state.ctx
	}
	common.SetContextKey(state.ctx, constant.ContextKeyWebSocketFirstEventMs, time.Since(connectionStarted).Milliseconds())

	excluded := make(map[int]bool, len(excludedChannelIDs))
	for channelID, isExcluded := range excludedChannelIDs {
		if isExcluded {
			excluded[channelID] = true
		}
	}
	for attempt := 0; attemptsUsed+attempt <= common.RetryTimes; attempt++ {
		state.logicalAttempts = attemptsUsed + attempt + 1
		var selectedChannel *model.Channel
		var selectErr *types.NewAPIError
		if retryParam != nil {
			var selectedGroup string
			selectedChannel, selectedGroup, err = service.CacheGetRandomSatisfiedChannel(retryParam)
			if err != nil {
				selectErr = types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的 WebSocket 渠道失败: %w", selectedGroup, state.info.OriginModelName, err), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
			} else if selectedChannel == nil {
				selectErr = types.NewErrorWithStatusCode(fmt.Errorf("分组 %s 下模型 %s 没有可用的 WebSocket 渠道", selectedGroup, state.info.OriginModelName), types.ErrorCodeResponsesWebSocketUnavailable, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
			} else if apiErr := middleware.SetupContextForSelectedChannel(state.ctx, selectedChannel, state.info.OriginModelName); apiErr != nil {
				selectErr = apiErr
			}
		} else {
			selectedChannel, selectErr = selectResponsesWebSocketChannel(state.ctx, state.info, excluded)
		}
		if selectErr != nil {
			failPreparedResponsesWebSocketRequest(state, nil, selectErr)
			return state, nil, nil, nil, 0, selectErr
		}
		if retryParam != nil {
			retryParam.RecordChannel(selectedChannel)
		}
		addUsedChannel(state.ctx, selectedChannel.Id)
		if billingErr := prepareResponsesWebSocketBilling(state); billingErr != nil {
			failPreparedResponsesWebSocketRequest(state, nil, billingErr)
			return state, nil, nil, selectedChannel, 0, billingErr
		}
		state.info.InitChannelMeta(state.ctx)

		outgoing, adaptor, payloadErr := prepareResponsesWebSocketPayload(state.ctx, state.info, request, payload)
		if payloadErr != nil {
			failPreparedResponsesWebSocketRequest(state, selectedChannel, payloadErr)
			return state, nil, nil, selectedChannel, 0, payloadErr
		}

		var target *websocket.Conn
		dialAttempted := false
		connectStarted := time.Now()
		dialErr := executeRelayAttempt(state.ctx, relayRiskContext{request: request, info: state.info, originalModel: state.info.OriginModelName}, defaultResponsesWebSocketRiskGate(), func() *types.NewAPIError {
			dialAttempted = true
			response, err := adaptor.DoRequest(state.ctx, state.info, nil)
			if err != nil {
				return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusBadGateway)
			}
			var ok bool
			target, ok = response.(*websocket.Conn)
			if !ok || target == nil {
				return types.NewError(errors.New("upstream did not return a websocket connection"), types.ErrorCodeBadResponse)
			}
			return nil
		})
		connectMs := time.Since(connectStarted).Milliseconds()
		if dialErr == nil {
			return state, outgoing, target, selectedChannel, connectMs, nil
		}

		excluded[selectedChannel.Id] = true
		if dialAttempted {
			processChannelFailure(state.ctx, *types.NewChannelError(selectedChannel.Id, selectedChannel.Type, selectedChannel.Name, selectedChannel.ChannelInfo.IsMultiKey, common.GetContextKeyString(state.ctx, constant.ContextKeyChannelKey), selectedChannel.GetAutoBan()), dialErr)
		}
		if target != nil {
			_ = target.Close()
		}
		if !shouldRetry(state.ctx, dialErr, common.RetryTimes-attempt) {
			failPreparedResponsesWebSocketRequest(state, nil, dialErr)
			return state, nil, nil, selectedChannel, 0, dialErr
		}
		if retryParam != nil {
			retryParam.IncreaseRetry()
		}
	}

	apiErr := types.NewError(errors.New("no available Responses WebSocket channel"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	failPreparedResponsesWebSocketRequest(state, nil, apiErr)
	return state, nil, nil, nil, 0, apiErr
}

func preparePinnedResponsesWebSocketRequest(pinnedCtx *gin.Context, payload []byte, sessionModel string, pinnedChannel *model.Channel) (*responsesWebSocketRequestState, []byte, error) {
	state, request, err := prepareResponsesWebSocketRequest(pinnedCtx, payload, sessionModel)
	if err != nil {
		apiErr := asResponsesWebSocketAPIError(err)
		failPreparedResponsesWebSocketRequest(state, nil, apiErr)
		return state, nil, apiErr
	}
	if billingErr := prepareResponsesWebSocketBilling(state); billingErr != nil {
		failPreparedResponsesWebSocketRequest(state, nil, billingErr)
		return state, nil, billingErr
	}
	state.info.InitChannelMeta(state.ctx)
	outgoing, _, payloadErr := prepareResponsesWebSocketPayload(state.ctx, state.info, request, payload)
	if payloadErr != nil {
		failPreparedResponsesWebSocketRequest(state, pinnedChannel, payloadErr)
		return state, nil, payloadErr
	}
	if riskErr := executeRelayAttempt(state.ctx, relayRiskContext{request: request, info: state.info, originalModel: state.info.OriginModelName}, defaultResponsesWebSocketRiskGate(), func() *types.NewAPIError { return nil }); riskErr != nil {
		failPreparedResponsesWebSocketRequest(state, pinnedChannel, riskErr)
		return state, nil, riskErr
	}
	return state, outgoing, nil
}

func prepareResponsesWebSocketPayload(c *gin.Context, info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest, payload []byte) ([]byte, channel.Adaptor, *types.NewAPIError) {
	requestCopy, err := common.DeepCopy(request)
	if err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ModelMappedHelper(c, info, requestCopy); err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}
	adaptor := relay.GetAdaptor(info.ApiType)
	if adaptor == nil {
		return nil, nil, types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		return payload, adaptor, nil
	}
	convertedValue, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *requestCopy)
	if err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	converted, ok := convertedValue.(dto.OpenAIResponsesRequest)
	if !ok {
		return nil, nil, types.NewError(fmt.Errorf("unexpected converted request type %T", convertedValue), types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	outgoing, err := sjson.SetBytes(payload, "model", converted.Model)
	if err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if converted.Reasoning != nil {
		outgoing, err = sjson.SetBytes(outgoing, "reasoning", converted.Reasoning)
		if err != nil {
			return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
	}
	outgoing, err = relaycommon.RemoveDisabledFields(outgoing, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if len(info.ParamOverride) > 0 {
		outgoing, err = relaycommon.ApplyParamOverrideWithRelayInfo(outgoing, info)
		if err != nil {
			return nil, nil, relay.NewAPIErrorFromParamOverride(err)
		}
	}
	return outgoing, adaptor, nil
}

func selectResponsesWebSocketChannel(c *gin.Context, info *relaycommon.RelayInfo, excluded map[int]bool) (*model.Channel, *types.NewAPIError) {
	if channelIDValue, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); ok {
		channelID, err := strconv.Atoi(fmt.Sprint(channelIDValue))
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		}
		selectedChannel, err := model.GetChannelById(channelID, true)
		if err != nil || !model.ChannelSupportsResponsesWebSocket(selectedChannel) {
			return nil, types.NewError(errors.New("指定渠道不支持 Responses WebSocket"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
		}
		if apiErr := middleware.SetupContextForSelectedChannel(c, selectedChannel, info.OriginModelName); apiErr != nil {
			return nil, apiErr
		}
		return selectedChannel, nil
	}

	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if preferredID, found := service.GetPreferredChannelByAffinity(c, info.OriginModelName, usingGroup); found && !excluded[preferredID] {
		preferred, err := model.CacheGetChannel(preferredID)
		if err == nil && model.ChannelSupportsResponsesWebSocket(preferred) {
			if usingGroup == "auto" {
				userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
				for _, group := range service.GetRequestAutoGroups(c, userGroup) {
					if model.IsChannelEnabledForGroupModel(group, info.OriginModelName, preferred.Id) {
						common.SetContextKey(c, constant.ContextKeyAutoGroup, group)
						service.MarkChannelAffinityUsed(c, group, preferred.Id)
						if apiErr := middleware.SetupContextForSelectedChannel(c, preferred, info.OriginModelName); apiErr != nil {
							return nil, apiErr
						}
						return preferred, nil
					}
				}
			} else if model.IsChannelEnabledForGroupModel(usingGroup, info.OriginModelName, preferred.Id) {
				service.MarkChannelAffinityUsed(c, usingGroup, preferred.Id)
				if apiErr := middleware.SetupContextForSelectedChannel(c, preferred, info.OriginModelName); apiErr != nil {
					return nil, apiErr
				}
				return preferred, nil
			}
		}
		if !service.ShouldKeepChannelAffinityOnChannelDisabled() {
			service.ClearCurrentChannelAffinityCache(c)
		}
	}

	retry := 0
	selectedChannel, selectedGroup, err := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
		Ctx:                c,
		TokenGroup:         usingGroup,
		ModelName:          info.OriginModelName,
		RequestPath:        c.Request.URL.Path,
		RequireWebSockets:  true,
		ExcludedChannelIDs: excluded,
		Retry:              &retry,
	})
	if err != nil {
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的 WebSocket 渠道失败: %w", selectedGroup, info.OriginModelName, err), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if selectedChannel == nil {
		return nil, types.NewErrorWithStatusCode(fmt.Errorf("分组 %s 下模型 %s 没有可用的 WebSocket 渠道", selectedGroup, info.OriginModelName), types.ErrorCodeResponsesWebSocketUnavailable, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	if apiErr := middleware.SetupContextForSelectedChannel(c, selectedChannel, info.OriginModelName); apiErr != nil {
		return nil, apiErr
	}
	return selectedChannel, nil
}
