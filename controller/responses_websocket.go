package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
)

func runResponsesWebSocketSession(baseCtx *gin.Context, clientConn *websocket.Conn, clientCodec *responsesWebSocketPrivateCodec, observability *responsesWebSocketObservability) error {
	baseCtx.Set(responsesWebSocketObservabilityKey, observability)
	connectionStarted := time.Now()
	sessionCtx, cancel := context.WithCancel(baseCtx.Request.Context())
	defer cancel()
	var (
		upstreamConn      *websocket.Conn
		upstreamFrames    <-chan responsesWebSocketFrame
		pinnedCtx         *gin.Context
		pinnedChannel     *model.Channel
		sessionModel      string
		active            *responsesWebSocketRequestState
		activePayload     []byte
		logicalAttempts   int
		attemptedChannels map[int]bool
		capacityEvidence  string
		cleanupReason     = responsesWebSocketCleanupSessionClosed
	)
	defer func() {
		if active != nil && cleanupReason == responsesWebSocketCleanupClientDisconnected {
			observability.markFailure(responsesWebSocketFailureCode(cleanupReason))
		}
		cleanupResponsesWebSocketSession(&active, &upstreamConn, cleanupReason)
		observability.markCleanup()
		observability.log(baseCtx, "cleanup")
	}()

	drainState := responsesWebSocketDrain.Load()
	drainCh := drainState.notify
	draining := drainState.draining.Load()
	if draining {
		_ = writeResponsesWebSocketClose(clientConn, websocket.CloseServiceRestart, responsesWebSocketDrainReason)
		return nil
	}

	clientFrames := startResponsesWebSocketReader(clientConn, sessionCtx.Done(), clientCodec, observability, false)

	for {
		select {
		case <-drainCh:
			drainCh = nil
			draining = true
			if active == nil {
				_ = writeResponsesWebSocketClose(clientConn, websocket.CloseServiceRestart, responsesWebSocketDrainReason)
				return nil
			}
		case frame, ok := <-clientFrames:
			if !ok {
				cleanupReason = responsesWebSocketCleanupClientDisconnected
				return nil
			}
			if draining && active == nil {
				_ = writeResponsesWebSocketClose(clientConn, websocket.CloseServiceRestart, responsesWebSocketDrainReason)
				return nil
			}
			if frame.controlType == websocket.PongMessage {
				if err := writeResponsesWebSocketControl(clientConn, websocket.PongMessage, frame.payload); err != nil {
					cleanupReason = responsesWebSocketCleanupClientDisconnected
					return err
				}
				continue
			}
			if frame.err != nil {
				if isResponsesWebSocketClientDisconnectError(frame.err) {
					cleanupReason = responsesWebSocketCleanupClientDisconnected
				}
				var protocolErr *responsesWebSocketPrivateProtocolError
				if errors.As(frame.err, &protocolErr) {
					_ = writeResponsesWebSocketClose(clientConn, protocolErr.CloseCode(), protocolErr.Error())
				}
				propagateResponsesWebSocketClose(upstreamConn, frame.err)
				if isNormalResponsesWebSocketClose(frame.err) {
					return nil
				}
				return frame.err
			}
			if frame.messageType != websocket.TextMessage {
				apiErr := types.NewErrorWithStatusCode(errors.New("Responses WebSocket 仅接受 JSON 文本事件"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
				_ = writeResponsesWebSocketError(clientConn, clientCodec, baseCtx, apiErr)
				_ = writeResponsesWebSocketClose(clientConn, websocket.CloseUnsupportedData, apiErr.Error())
				return apiErr
			}

			eventType, err := responsesWebSocketEventType(frame.payload)
			if err != nil {
				apiErr := types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
				_ = writeResponsesWebSocketError(clientConn, clientCodec, baseCtx, apiErr)
				_ = writeResponsesWebSocketClose(clientConn, websocket.CloseInvalidFramePayloadData, apiErr.Error())
				return apiErr
			}
			if drainState.shouldRejectNewResponse(active, eventType) {
				_ = writeResponsesWebSocketClose(clientConn, websocket.CloseServiceRestart, responsesWebSocketDrainReason)
				return nil
			}
			if draining && (active == nil || eventType != "response.cancel") {
				_ = writeResponsesWebSocketClose(clientConn, websocket.CloseServiceRestart, responsesWebSocketDrainReason)
				return nil
			}

			if eventType == "response.create" {
				if active != nil {
					apiErr := types.NewErrorWithStatusCode(errors.New("上一条 response.create 尚未结束，不支持并发创建"), types.ErrorCodeInvalidRequest, http.StatusConflict, types.ErrOptionWithSkipRetry())
					if err := writeResponsesWebSocketError(clientConn, clientCodec, active.ctx, apiErr); err != nil {
						cleanupReason = responsesWebSocketCleanupClientDisconnected
						return err
					}
					continue
				}

				// The upstream pins model, channel, and response state per connection.
				// A stateless model switch can rebuild before submission; a stateful one
				// must ask Codex to resend the complete context.
				explicitModel := strings.TrimSpace(gjson.GetBytes(frame.payload, "model").String())
				previousResponseID := strings.TrimSpace(gjson.GetBytes(frame.payload, "previous_response_id").String())
				if upstreamConn != nil && explicitModel != "" && explicitModel != sessionModel {
					if previousResponseID != "" {
						if err := writeResponsesWebSocketStateMissing(clientConn, clientCodec, baseCtx); err != nil {
							cleanupReason = responsesWebSocketCleanupClientDisconnected
							return err
						}
						continue
					}
					_ = upstreamConn.Close()
					upstreamConn = nil
					upstreamFrames = nil
					pinnedCtx = nil
					pinnedChannel = nil
					sessionModel = ""
				}
				observability.acceptResponseCreate()
				activePayload = append(activePayload[:0], frame.payload...)
				logicalAttempts = 0
				attemptedChannels = make(map[int]bool)
				capacityEvidence = ""

				var outgoing []byte
				var state *responsesWebSocketRequestState
				if upstreamConn == nil {
					var connectMs int64
					state, outgoing, upstreamConn, pinnedChannel, connectMs, err = prepareFirstResponsesWebSocketRequest(baseCtx, frame.payload, connectionStarted)
					if err == nil {
						observability.upstreamDial(common.GetContextKeyString(state.ctx, common.ResponsesWebSocketUpstreamTraceKey))
						pinnedCtx = state.ctx
						sessionModel = state.info.OriginModelName
						common.SetContextKey(state.ctx, constant.ContextKeyWebSocketUpstreamConnectMs, connectMs)
					}
				} else {
					state, outgoing, err = preparePinnedResponsesWebSocketRequest(pinnedCtx, frame.payload, sessionModel, pinnedChannel)
				}
				if err != nil {
					observability.markFailure("response_create_prepare_failed")
					apiErr := asResponsesWebSocketAPIError(err)
					errorCtx := baseCtx
					if state != nil && state.ctx != nil {
						errorCtx = state.ctx
					}
					_ = writeResponsesWebSocketError(clientConn, clientCodec, errorCtx, apiErr)
					_ = writeResponsesWebSocketClose(clientConn, websocket.CloseTryAgainLater, apiErr.Error())
					return err
				}

				active = state
				logicalAttempts = state.logicalAttempts
				for _, rawChannelID := range state.ctx.GetStringSlice("use_channel") {
					if channelID, parseErr := strconv.Atoi(rawChannelID); parseErr == nil {
						attemptedChannels[channelID] = true
					}
				}
				if err := writeResponsesWebSocketMessage(upstreamConn, websocket.TextMessage, outgoing); err != nil {
					common.SetContextKey(active.ctx, constant.ContextKeyWebSocketCloseReason, "upstream write failed")
					failResponsesWebSocketRequest(active, pinnedChannel, "upstream write failed")
					active = nil
					_ = writeResponsesWebSocketError(clientConn, clientCodec, state.ctx, types.NewError(err, types.ErrorCodeDoRequestFailed))
					_ = writeResponsesWebSocketClose(clientConn, websocket.CloseServiceRestart, "upstream write failed")
					return err
				}
				observability.commitUpstreamRequest()
				common.CleanupBodyStorage(state.ctx)

				if upstreamFrames == nil {
					upstreamFrames = startResponsesWebSocketReader(upstreamConn, sessionCtx.Done(), nil, observability, true)
				}
				continue
			}

			if upstreamConn == nil {
				apiErr := types.NewErrorWithStatusCode(errors.New("首个 WebSocket 事件必须是 response.create"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
				_ = writeResponsesWebSocketError(clientConn, clientCodec, baseCtx, apiErr)
				_ = writeResponsesWebSocketClose(clientConn, websocket.ClosePolicyViolation, apiErr.Error())
				return apiErr
			}
			if active == nil {
				apiErr := types.NewErrorWithStatusCode(errors.New("当前没有活动的 response.create，请先创建响应"), types.ErrorCodeInvalidRequest, http.StatusConflict, types.ErrOptionWithSkipRetry())
				if err := writeResponsesWebSocketError(clientConn, clientCodec, baseCtx, apiErr); err != nil {
					return err
				}
				continue
			}
			if eventType == "response.cancel" {
				active.cancelRequested = true
			}
			active.replayDisallowed = true
			if err := writeResponsesWebSocketMessage(upstreamConn, websocket.TextMessage, frame.payload); err != nil {
				if active != nil {
					common.SetContextKey(active.ctx, constant.ContextKeyWebSocketCloseReason, responsesWebSocketCleanupUpstreamWriteFailed)
				}
				cleanupReason = responsesWebSocketCleanupUpstreamWriteFailed
				return err
			}

		case frame, ok := <-upstreamFrames:
			if !ok {
				cleanupReason = responsesWebSocketCleanupUpstreamDisconnected
				return errors.New("upstream websocket closed")
			}
			observability.upstreamQueue.dequeue(len(frame.payload))
			if draining && active == nil {
				_ = writeResponsesWebSocketClose(clientConn, websocket.CloseServiceRestart, responsesWebSocketDrainReason)
				return nil
			}
			if frame.controlType == websocket.PongMessage {
				if err := writeResponsesWebSocketControl(upstreamConn, websocket.PongMessage, frame.payload); err != nil {
					cleanupReason = responsesWebSocketCleanupUpstreamWriteFailed
					return err
				}
				continue
			}
			if frame.err != nil {
				if active == nil {
					observability.markFailure("upstream_disconnected")
					propagateResponsesWebSocketClose(clientConn, frame.err)
					return nil
				}
				if draining {
					common.SetContextKey(active.ctx, constant.ContextKeyWebSocketCloseReason, responsesWebSocketDrainReason)
					failPreparedResponsesWebSocketRequest(active, pinnedChannel, nil)
					refundResponsesWebSocketBillingIfPending(baseCtx, active.info.Billing)
					active = nil
					activePayload = nil
					logicalAttempts = 0
					attemptedChannels = nil
					capacityEvidence = ""
					_ = writeResponsesWebSocketClose(clientConn, websocket.CloseServiceRestart, responsesWebSocketDrainReason)
					return nil
				}
				capacityCode, capacityRejected := responsesWebSocketCapacityCode(frame.err)
				if capacityRejected {
					capacityEvidence = mergeResponsesWebSocketCapacityCode(capacityEvidence, capacityCode)
				}
				if capacityRejected && !draining &&
					!active.applicationOutputSeen &&
					!active.cancelRequested &&
					!active.replayDisallowed &&
					strings.TrimSpace(gjson.GetBytes(activePayload, "previous_response_id").String()) == "" &&
					!responsesWebSocketHasSpecificChannel(active.ctx) &&
					logicalAttempts <= common.RetryTimes &&
					sessionCtx.Err() == nil &&
					baseCtx.Request.Context().Err() == nil {
					capacityErr := newResponsesWebSocketCapacityError(capacityCode)
					recordResponsesWebSocketRetryFailure(active, pinnedChannel, capacityErr)
					oldState := active
					oldPayload := append([]byte(nil), activePayload...)
					oldAttempts := logicalAttempts
					oldBilling := oldState.info.Billing
					oldRateLimit := oldState.rateLimit
					_ = upstreamConn.Close()
					upstreamConn = nil
					upstreamFrames = nil

					excludedRetryChannels := make(map[int]bool, len(attemptedChannels)+1)
					for channelID := range attemptedChannels {
						excludedRetryChannels[channelID] = true
					}
					if pinnedChannel != nil {
						excludedRetryChannels[pinnedChannel.Id] = true
					}
					retryParam := &service.RetryParam{
						Ctx:                oldState.ctx,
						TokenGroup:         oldState.info.TokenGroup,
						ModelName:          oldState.info.OriginModelName,
						RequestPath:        baseCtx.Request.URL.Path,
						RequireWebSockets:  true,
						ExcludedChannelIDs: nil,
						TriedChannelIDs:    make(map[int]bool, len(attemptedChannels)),
						Retry:              common.GetPointer(oldAttempts),
					}
					for channelID := range attemptedChannels {
						retryParam.TriedChannelIDs[channelID] = true
					}
					if pinnedChannel != nil {
						retryParam.RecordChannel(pinnedChannel)
					}
					retryState, retryOutgoing, retryConn, retryChannel, connectMs, retryErr := prepareFirstResponsesWebSocketRequestWithBilling(baseCtx, oldPayload, connectionStarted, oldBilling, oldRateLimit, oldState.info, retryParam, excludedRetryChannels, oldAttempts, oldState.ctx.GetStringSlice("use_channel"), false)
					if retryErr == nil {
						active = retryState
						logicalAttempts = retryState.logicalAttempts
						pinnedCtx = retryState.ctx
						pinnedChannel = retryChannel
						for _, rawChannelID := range retryState.ctx.GetStringSlice("use_channel") {
							if channelID, parseErr := strconv.Atoi(rawChannelID); parseErr == nil {
								attemptedChannels[channelID] = true
							}
						}
						sessionModel = retryState.info.OriginModelName
						upstreamConn = retryConn
						observability.upstreamDial(common.GetContextKeyString(retryState.ctx, common.ResponsesWebSocketUpstreamTraceKey))
						common.SetContextKey(retryState.ctx, constant.ContextKeyWebSocketUpstreamConnectMs, connectMs)
						if err := writeResponsesWebSocketMessage(upstreamConn, websocket.TextMessage, retryOutgoing); err != nil {
							common.SetContextKey(retryState.ctx, constant.ContextKeyWebSocketCloseReason, "upstream write failed")
							failResponsesWebSocketRequest(retryState, retryChannel, "upstream write failed")
							refundResponsesWebSocketBillingIfPending(baseCtx, oldBilling)
							active = nil
							activePayload = nil
							logicalAttempts = 0
							attemptedChannels = nil
							_ = writeResponsesWebSocketError(clientConn, clientCodec, baseCtx, newResponsesWebSocketCapacityError(capacityEvidence))
							_ = writeResponsesWebSocketClose(clientConn, websocket.CloseInternalServerErr, "upstream capacity rejected")
							return err
						}
						observability.commitUpstreamRequest()
						common.CleanupBodyStorage(oldState.ctx)
						common.CleanupBodyStorage(retryState.ctx)
						upstreamFrames = startResponsesWebSocketReader(upstreamConn, sessionCtx.Done(), nil, observability, true)
						continue
					}

					active = nil
					activePayload = nil
					logicalAttempts = 0
					attemptedChannels = nil
					refundResponsesWebSocketBillingIfPending(baseCtx, oldBilling)
					retryAPIError := asResponsesWebSocketAPIError(retryErr)
					finalErr := responsesWebSocketCapacityFallbackError(capacityEvidence, retryAPIError)
					_ = writeResponsesWebSocketError(clientConn, clientCodec, baseCtx, finalErr)
					_ = writeResponsesWebSocketClose(clientConn, websocket.CloseInternalServerErr, "upstream capacity rejected")
					return finalErr
				}
				if capacityRejected && !active.cancelRequested {
					capacityErr := newResponsesWebSocketCapacityError(capacityCode)
					common.SetContextKey(active.ctx, constant.ContextKeyWebSocketCloseReason, "capacity rejected")
					failPreparedResponsesWebSocketRequest(active, pinnedChannel, capacityErr)
					active = nil
					activePayload = nil
					logicalAttempts = 0
					attemptedChannels = nil
					capacityEvidence = ""
					_ = writeResponsesWebSocketError(clientConn, clientCodec, baseCtx, capacityErr)
					_ = writeResponsesWebSocketClose(clientConn, websocket.CloseInternalServerErr, "upstream capacity rejected")
					return frame.err
				}
				observability.markFailure("upstream_disconnected")
				common.SetContextKey(active.ctx, constant.ContextKeyWebSocketCloseReason, "upstream disconnected")
				finalCapacityCode := ""
				if !active.applicationOutputSeen && !active.cancelRequested {
					finalCapacityCode = capacityEvidence
				}
				failResponsesWebSocketRequest(active, responsesWebSocketFailureChannel(pinnedChannel, frame.err), "upstream disconnected")
				active = nil
				activePayload = nil
				logicalAttempts = 0
				attemptedChannels = nil
				capacityEvidence = ""
				if finalCapacityCode != "" {
					capacityErr := newResponsesWebSocketCapacityError(finalCapacityCode)
					_ = writeResponsesWebSocketError(clientConn, clientCodec, baseCtx, capacityErr)
					_ = writeResponsesWebSocketClose(clientConn, websocket.CloseInternalServerErr, "upstream capacity rejected")
					return frame.err
				}
				if !isNormalResponsesWebSocketClose(frame.err) {
					fallbackErr := types.NewError(errors.New("upstream websocket disconnected"), types.ErrorCodeDoRequestFailed)
					finalErr := responsesWebSocketCapacityFallbackError(finalCapacityCode, fallbackErr)
					_ = writeResponsesWebSocketError(clientConn, clientCodec, baseCtx, finalErr)
				}
				propagateResponsesWebSocketClose(clientConn, frame.err)
				if isNormalResponsesWebSocketClose(frame.err) {
					return nil
				}
				return frame.err
			}

			terminal := false
			terminalActive := false
			var usage *dto.Usage
			if active != nil && active.logicalAttempts > 1 && responsesWebSocketEventIsPreOutputState(frame.payload) {
				// A retry attempt has already established upstream response state;
				// only the first attempt may use a later capacity sideband to retry.
				active.replayDisallowed = true
			}
			if active != nil && capacityEvidence != "" && responsesWebSocketTerminalErrorEvent(frame.payload) &&
				!active.applicationOutputSeen && !active.cancelRequested && !responsesWebSocketEventHasApplicationOutput(frame.payload) {
				capacityErr := newResponsesWebSocketCapacityError(capacityEvidence)
				common.SetContextKey(active.ctx, constant.ContextKeyWebSocketCloseReason, "capacity rejected")
				failPreparedResponsesWebSocketRequest(active, pinnedChannel, capacityErr)
				active = nil
				activePayload = nil
				logicalAttempts = 0
				attemptedChannels = nil
				capacityEvidence = ""
				_ = writeResponsesWebSocketError(clientConn, clientCodec, baseCtx, capacityErr)
				_ = writeResponsesWebSocketClose(clientConn, websocket.CloseInternalServerErr, "upstream capacity rejected")
				return nil
			}
			framesToForward := []responsesWebSocketFrame{{messageType: frame.messageType, payload: frame.payload}}
			if active != nil {
				if responsesWebSocketEventAllowsCapacityRetry(frame.payload) && !active.applicationOutputSeen &&
					len(active.pendingFrames) < responsesWebSocketPendingFrameMax &&
					active.pendingBytes+len(frame.payload) <= responsesWebSocketPendingBytesMax {
					active.pendingFrames = append(active.pendingFrames, framesToForward[0])
					active.pendingBytes += len(frame.payload)
					framesToForward = nil
				} else {
					active.applicationOutputSeen = true
					framesToForward = append(append([]responsesWebSocketFrame(nil), active.pendingFrames...), framesToForward...)
					active.pendingFrames = nil
					active.pendingBytes = 0
				}
				if !active.info.HasSendResponse() {
					active.info.SetFirstResponseTime()
					common.SetContextKey(active.ctx, constant.ContextKeyWebSocketFirstOutputMs, active.info.FirstResponseTime.Sub(active.info.StartTime).Milliseconds())
				}
				var observeErr error
				terminal, usage, observeErr = active.tracker.Observe(frame.payload)
				terminalActive = terminal
				if observeErr != nil {
					common.SetContextKey(active.ctx, constant.ContextKeyWebSocketCloseReason, "invalid upstream event")
					failResponsesWebSocketRequest(active, pinnedChannel, "invalid upstream event")
					active = nil
					_ = writeResponsesWebSocketError(clientConn, clientCodec, baseCtx, types.NewError(observeErr, types.ErrorCodeBadResponseBody))
					_ = writeResponsesWebSocketClose(clientConn, websocket.CloseInvalidFramePayloadData, "invalid upstream event")
					return observeErr
				}
			}
			if terminal && active != nil {
				observability.markTerminal()
				common.SetContextKey(active.ctx, constant.ContextKeyWebSocketCompleteMs, time.Since(active.info.StartTime).Milliseconds())
				service.PostTextConsumeQuota(active.ctx, active.info, usage, nil)
				if active.tracker.Succeeded() {
					active.rateLimit.RecordSuccess()
				}
				if pinnedChannel != nil {
					service.RecordChannelAffinity(active.ctx, pinnedChannel.Id)
				}
				active = nil
				activePayload = nil
				logicalAttempts = 0
				attemptedChannels = nil
				capacityEvidence = ""
			}
			for _, frameToForward := range framesToForward {
				if err := writeResponsesWebSocketClientMessage(clientConn, clientCodec, frameToForward.messageType, frameToForward.payload); err != nil {
					cleanupReason = responsesWebSocketCleanupClientDisconnected
					return err
				}
			}
			if terminalActive && draining {
				_ = writeResponsesWebSocketClose(clientConn, websocket.CloseServiceRestart, responsesWebSocketDrainReason)
				return nil
			}
		}
	}
}
