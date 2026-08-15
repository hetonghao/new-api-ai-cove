package controller

import (
	"context"
	"errors"
	"net/http"
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
		upstreamConn   *websocket.Conn
		upstreamFrames <-chan responsesWebSocketFrame
		pinnedCtx      *gin.Context
		pinnedChannel  *model.Channel
		sessionModel   string
		active         *responsesWebSocketRequestState
	)
	defer func() {
		cleanupResponsesWebSocketSession(&active, &upstreamConn)
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
				return nil
			}
			if draining && active == nil {
				_ = writeResponsesWebSocketClose(clientConn, websocket.CloseServiceRestart, responsesWebSocketDrainReason)
				return nil
			}
			if frame.controlType == websocket.PongMessage {
				if err := writeResponsesWebSocketControl(clientConn, websocket.PongMessage, frame.payload); err != nil {
					return err
				}
				continue
			}
			if frame.err != nil {
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
			if err := writeResponsesWebSocketMessage(upstreamConn, websocket.TextMessage, frame.payload); err != nil {
				if active != nil {
					common.SetContextKey(active.ctx, constant.ContextKeyWebSocketCloseReason, "upstream write failed")
				}
				return err
			}

		case frame, ok := <-upstreamFrames:
			if !ok {
				observability.markFailure("upstream_websocket_closed")
				return errors.New("upstream websocket closed")
			}
			observability.upstreamQueue.dequeue(len(frame.payload))
			if draining && active == nil {
				_ = writeResponsesWebSocketClose(clientConn, websocket.CloseServiceRestart, responsesWebSocketDrainReason)
				return nil
			}
			if frame.controlType == websocket.PongMessage {
				if err := writeResponsesWebSocketControl(upstreamConn, websocket.PongMessage, frame.payload); err != nil {
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
				observability.markFailure("upstream_disconnected")
				common.SetContextKey(active.ctx, constant.ContextKeyWebSocketCloseReason, "upstream disconnected")
				failResponsesWebSocketRequest(active, responsesWebSocketFailureChannel(pinnedChannel, frame.err), "upstream disconnected")
				active = nil
				if !isNormalResponsesWebSocketClose(frame.err) {
					_ = writeResponsesWebSocketError(clientConn, clientCodec, baseCtx, types.NewError(errors.New("upstream websocket disconnected"), types.ErrorCodeDoRequestFailed))
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
			if active != nil {
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
			}
			if err := writeResponsesWebSocketClientMessage(clientConn, clientCodec, frame.messageType, frame.payload); err != nil {
				return err
			}
			if terminalActive && draining {
				_ = writeResponsesWebSocketClose(clientConn, websocket.CloseServiceRestart, responsesWebSocketDrainReason)
				return nil
			}
		}
	}
}
