package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	responsesWebSocketCleanupSessionClosed        = "session closed before terminal response"
	responsesWebSocketCleanupClientDisconnected   = "client disconnected before terminal response"
	responsesWebSocketCleanupUpstreamDisconnected = "upstream disconnected"
	responsesWebSocketCleanupUpstreamWriteFailed  = "upstream write failed"
	responsesWebSocketFailureRuntime              = "runtime_failure"
	responsesWebSocketFailureUpstreamWrite        = "upstream_write_failed"
	responsesWebSocketFailureUpstreamDisconnected = "upstream_disconnected"
	responsesWebSocketFailureInvalidUpstreamEvent = "invalid_upstream_event"
	responsesWebSocketFailureSessionClosed        = "session_closed_before_terminal"
	responsesWebSocketFailureClientDisconnected   = "client_disconnected_before_terminal"
)

func responsesWebSocketFailureCode(reason string) string {
	switch reason {
	case responsesWebSocketCleanupUpstreamWriteFailed:
		return responsesWebSocketFailureUpstreamWrite
	case responsesWebSocketCleanupUpstreamDisconnected:
		return responsesWebSocketFailureUpstreamDisconnected
	case "invalid upstream event":
		return responsesWebSocketFailureInvalidUpstreamEvent
	case responsesWebSocketCleanupSessionClosed:
		return responsesWebSocketFailureSessionClosed
	case responsesWebSocketCleanupClientDisconnected:
		return responsesWebSocketFailureClientDisconnected
	default:
		return responsesWebSocketFailureRuntime
	}
}

func responsesWebSocketFailureChannel(channel *model.Channel, err error) *model.Channel {
	if channel == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		switch closeErr.Code {
		case websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure, websocket.CloseTLSHandshake:
			return channel
		default:
			return nil
		}
	}
	return channel
}

func failPreparedResponsesWebSocketRequest(state *responsesWebSocketRequestState, channel *model.Channel, apiErr *types.NewAPIError) {
	if state == nil {
		return
	}
	if state.observability != nil {
		state.observability.markFailure("response_prepare_failed")
	}
	if apiErr != nil {
		apiErr = service.NormalizeViolationFeeError(apiErr)
		service.ChargeViolationFeeIfNeeded(state.ctx, state.info, apiErr)
		if channel != nil {
			channelError := *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(state.ctx, constant.ContextKeyChannelKey), channel.GetAutoBan())
			service.HandleSevereRiskFromRelay(service.SevereRiskRelayInput{Context: state.ctx, Request: state.info.Request, Channel: channelError, Model: state.info.OriginModelName, UpstreamErr: apiErr, ChannelTest: state.info.IsChannelTest})
			processChannelError(state.ctx, channelError, apiErr)
		} else {
			if len(state.ctx.GetStringSlice("use_channel")) == 0 {
				logger.LogError(state.ctx, fmt.Sprintf("relay error before channel selection (status code: %d): %s", apiErr.StatusCode, common.LocalLogPreview(apiErr.Error())))
			}
			recordRelayErrorLog(state.ctx, apiErr)
		}
	}
	finalizeFailedResponsesWebSocketRequest(state)
}

func finalizeFailedResponsesWebSocketRequest(state *responsesWebSocketRequestState) {
	if state == nil {
		return
	}
	if state.info != nil && state.info.Billing != nil {
		state.info.Billing.Refund(state.ctx)
	}
	common.CleanupBodyStorage(state.ctx)
	gopool.Go(func() {
		perfmetrics.RecordRelaySample(state.info, false, 0)
	})
}

func failResponsesWebSocketRequest(state *responsesWebSocketRequestState, channel *model.Channel, reason string) {
	if state == nil {
		return
	}
	if state.observability != nil {
		state.observability.markFailure(responsesWebSocketFailureCode(reason))
	}
	apiErr := types.NewError(errors.New(reason), types.ErrorCodeDoRequestFailed)
	failPreparedResponsesWebSocketRequest(state, channel, apiErr)
}

func recordResponsesWebSocketRetryFailure(state *responsesWebSocketRequestState, channel *model.Channel, apiErr *types.NewAPIError) {
	if state == nil || state.info == nil || apiErr == nil {
		return
	}
	state.info.LastError = apiErr
	if channel == nil {
		recordRelayErrorLog(state.ctx, apiErr)
		return
	}
	channelError := *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(state.ctx, constant.ContextKeyChannelKey), channel.GetAutoBan())
	service.HandleSevereRiskFromRelay(service.SevereRiskRelayInput{Context: state.ctx, Request: state.info.Request, Channel: channelError, Model: state.info.OriginModelName, UpstreamErr: apiErr, ChannelTest: state.info.IsChannelTest})
	processChannelError(state.ctx, channelError, apiErr)
}

func refundResponsesWebSocketBillingIfPending(ctx *gin.Context, billing relaycommon.BillingSettler) {
	if billing == nil || !billing.NeedsRefund() {
		return
	}
	billing.Refund(ctx)
}

func failIntermediateResponsesWebSocketRequest(state *responsesWebSocketRequestState, channel *model.Channel, apiErr *types.NewAPIError) {
	if state == nil {
		return
	}
	recordResponsesWebSocketRetryFailure(state, channel, apiErr)
	common.CleanupBodyStorage(state.ctx)
}

func asResponsesWebSocketAPIError(err error) *types.NewAPIError {
	var apiErr *types.NewAPIError
	if errors.As(err, &apiErr) && apiErr != nil {
		return apiErr
	}
	return types.NewError(err, types.ErrorCodeBadResponse)
}
