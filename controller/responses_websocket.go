package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var responsesWebSocketUpgrader = websocket.Upgrader{
	ReadBufferSize:    4096,
	WriteBufferSize:   4096,
	EnableCompression: true,
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

func ResponsesWebSocket(c *gin.Context) {
	clientConn, err := responsesWebSocketUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer clientConn.Close()

	if err := runResponsesWebSocketSession(c, clientConn); err != nil {
		logger.LogError(c, fmt.Sprintf("responses websocket session ended: %s", common.LocalLogPreview(err.Error())))
	}
}

func runResponsesWebSocketSession(baseCtx *gin.Context, clientConn *websocket.Conn) error {
	connectionStarted := time.Now()
	sessionCtx, cancel := context.WithCancel(baseCtx.Request.Context())
	defer cancel()

	clientFrames := make(chan responsesWebSocketFrame, responsesWebSocketQueueSize)
	configureResponsesWebSocketReader(clientConn, clientFrames, sessionCtx.Done())
	gopool.Go(func() {
		readResponsesWebSocketFrames(clientConn, clientFrames, sessionCtx.Done())
	})

	var (
		upstreamConn   *websocket.Conn
		upstreamFrames <-chan responsesWebSocketFrame
		pinnedCtx      *gin.Context
		pinnedChannel  *model.Channel
		sessionModel   string
		active         *responsesWebSocketRequestState
	)

	defer func() {
		if active != nil {
			failResponsesWebSocketRequest(active, nil, "session closed before terminal response")
		}
		if upstreamConn != nil {
			_ = upstreamConn.Close()
		}
	}()

	for {
		select {
		case frame, ok := <-clientFrames:
			if !ok {
				return nil
			}
			if frame.controlType == websocket.PongMessage {
				if err := writeResponsesWebSocketControl(clientConn, websocket.PongMessage, frame.payload); err != nil {
					return err
				}
				continue
			}
			if frame.err != nil {
				propagateResponsesWebSocketClose(upstreamConn, frame.err)
				if isNormalResponsesWebSocketClose(frame.err) {
					return nil
				}
				return frame.err
			}
			if frame.messageType != websocket.TextMessage {
				apiErr := types.NewErrorWithStatusCode(errors.New("Responses WebSocket 仅接受 JSON 文本事件"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
				_ = writeResponsesWebSocketError(clientConn, baseCtx, apiErr)
				_ = writeResponsesWebSocketClose(clientConn, websocket.CloseUnsupportedData, apiErr.Error())
				return apiErr
			}

			eventType, err := responsesWebSocketEventType(frame.payload)
			if err != nil {
				apiErr := types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
				_ = writeResponsesWebSocketError(clientConn, baseCtx, apiErr)
				_ = writeResponsesWebSocketClose(clientConn, websocket.CloseInvalidFramePayloadData, apiErr.Error())
				return apiErr
			}

			if eventType == "response.create" {
				if active != nil {
					apiErr := types.NewErrorWithStatusCode(errors.New("上一条 response.create 尚未结束，不支持并发创建"), types.ErrorCodeInvalidRequest, http.StatusConflict, types.ErrOptionWithSkipRetry())
					if err := writeResponsesWebSocketError(clientConn, active.ctx, apiErr); err != nil {
						return err
					}
					continue
				}

				var outgoing []byte
				var state *responsesWebSocketRequestState
				if upstreamConn == nil {
					var connectMs int64
					state, outgoing, upstreamConn, pinnedChannel, connectMs, err = prepareFirstResponsesWebSocketRequest(baseCtx, frame.payload, connectionStarted)
					if err == nil {
						pinnedCtx = state.ctx
						sessionModel = state.info.OriginModelName
						common.SetContextKey(state.ctx, constant.ContextKeyWebSocketUpstreamConnectMs, connectMs)
					}
				} else {
					state, outgoing, err = preparePinnedResponsesWebSocketRequest(pinnedCtx, frame.payload, sessionModel, pinnedChannel)
				}
				if err != nil {
					apiErr := asResponsesWebSocketAPIError(err)
					errorCtx := baseCtx
					if state != nil && state.ctx != nil {
						errorCtx = state.ctx
					}
					_ = writeResponsesWebSocketError(clientConn, errorCtx, apiErr)
					_ = writeResponsesWebSocketClose(clientConn, websocket.CloseTryAgainLater, apiErr.Error())
					return err
				}

				active = state
				if err := writeResponsesWebSocketMessage(upstreamConn, websocket.TextMessage, outgoing); err != nil {
					common.SetContextKey(active.ctx, constant.ContextKeyWebSocketCloseReason, "upstream write failed")
					failResponsesWebSocketRequest(active, pinnedChannel, "upstream write failed")
					active = nil
					_ = writeResponsesWebSocketError(clientConn, state.ctx, types.NewError(err, types.ErrorCodeDoRequestFailed))
					_ = writeResponsesWebSocketClose(clientConn, websocket.CloseServiceRestart, "upstream write failed")
					return err
				}
				common.CleanupBodyStorage(state.ctx)

				if upstreamFrames == nil {
					frames := make(chan responsesWebSocketFrame, responsesWebSocketQueueSize)
					configureResponsesWebSocketReader(upstreamConn, frames, sessionCtx.Done())
					gopool.Go(func() {
						readResponsesWebSocketFrames(upstreamConn, frames, sessionCtx.Done())
					})
					upstreamFrames = frames
				}
				continue
			}

			if upstreamConn == nil {
				apiErr := types.NewErrorWithStatusCode(errors.New("首个 WebSocket 事件必须是 response.create"), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
				_ = writeResponsesWebSocketError(clientConn, baseCtx, apiErr)
				_ = writeResponsesWebSocketClose(clientConn, websocket.ClosePolicyViolation, apiErr.Error())
				return apiErr
			}
			if active == nil {
				apiErr := types.NewErrorWithStatusCode(errors.New("当前没有活动的 response.create，请先创建响应"), types.ErrorCodeInvalidRequest, http.StatusConflict, types.ErrOptionWithSkipRetry())
				if err := writeResponsesWebSocketError(clientConn, baseCtx, apiErr); err != nil {
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
				return errors.New("upstream websocket closed")
			}
			if frame.controlType == websocket.PongMessage {
				if err := writeResponsesWebSocketControl(upstreamConn, websocket.PongMessage, frame.payload); err != nil {
					return err
				}
				continue
			}
			if frame.err != nil {
				if active != nil {
					common.SetContextKey(active.ctx, constant.ContextKeyWebSocketCloseReason, "upstream disconnected")
					failResponsesWebSocketRequest(active, responsesWebSocketFailureChannel(pinnedChannel, frame.err), "upstream disconnected")
					active = nil
				}
				if !isNormalResponsesWebSocketClose(frame.err) {
					_ = writeResponsesWebSocketError(clientConn, baseCtx, types.NewError(errors.New("upstream websocket disconnected"), types.ErrorCodeDoRequestFailed))
				}
				propagateResponsesWebSocketClose(clientConn, frame.err)
				if isNormalResponsesWebSocketClose(frame.err) {
					return nil
				}
				return frame.err
			}

			terminal := false
			var usage *dto.Usage
			if active != nil {
				if !active.info.HasSendResponse() {
					active.info.SetFirstResponseTime()
					common.SetContextKey(active.ctx, constant.ContextKeyWebSocketFirstOutputMs, active.info.FirstResponseTime.Sub(active.info.StartTime).Milliseconds())
				}
				var observeErr error
				terminal, usage, observeErr = active.tracker.Observe(frame.payload)
				if observeErr != nil {
					common.SetContextKey(active.ctx, constant.ContextKeyWebSocketCloseReason, "invalid upstream event")
					failResponsesWebSocketRequest(active, pinnedChannel, "invalid upstream event")
					active = nil
					_ = writeResponsesWebSocketError(clientConn, baseCtx, types.NewError(observeErr, types.ErrorCodeBadResponseBody))
					_ = writeResponsesWebSocketClose(clientConn, websocket.CloseInvalidFramePayloadData, "invalid upstream event")
					return observeErr
				}
			}
			if terminal && active != nil {
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
			if err := writeResponsesWebSocketMessage(clientConn, frame.messageType, frame.payload); err != nil {
				return err
			}
		}
	}
}
