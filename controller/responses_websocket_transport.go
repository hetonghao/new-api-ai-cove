package controller

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	responsesWebSocketQueueSize    = 8
	responsesWebSocketWriteTimeout = 30 * time.Second
	responsesWebSocketCloseMax     = 123
	responsesWebSocketDrainReason  = "service restarting"
)

type responsesWebSocketDrainState struct {
	draining atomic.Bool
	notify   chan struct{}
}

func newResponsesWebSocketDrainState() *responsesWebSocketDrainState {
	return &responsesWebSocketDrainState{notify: make(chan struct{})}
}

func (state *responsesWebSocketDrainState) shouldRejectNewResponse(active *responsesWebSocketRequestState, eventType string) bool {
	return active == nil && eventType == "response.create" && state.draining.Load()
}

var responsesWebSocketDrain atomic.Pointer[responsesWebSocketDrainState]

func init() {
	responsesWebSocketDrain.Store(newResponsesWebSocketDrainState())
}

// BeginResponsesWebSocketDrain stops new sessions while allowing active ones
// to forward their terminal event before closing with Service Restart.
func BeginResponsesWebSocketDrain() {
	state := responsesWebSocketDrain.Load()
	if state.draining.CompareAndSwap(false, true) {
		close(state.notify)
	}
}

type responsesWebSocketFrame struct {
	messageType int
	payload     []byte
	err         error
	controlType int
}

func configureResponsesWebSocketReader(conn *websocket.Conn, frames chan<- responsesWebSocketFrame, done <-chan struct{}, codec *responsesWebSocketPrivateCodec, queue *responsesWebSocketQueueStats) {
	maxMB := constant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = 128
	}
	readLimit := int64(maxMB) << 20
	if codec != nil {
		readLimit = min(readLimit, responsesWebSocketPrivateMaxBytes) + responsesWebSocketPrivateHeaderSize
	}
	conn.SetReadLimit(readLimit)
	conn.SetPingHandler(func(data string) error {
		return enqueueResponsesWebSocketFrame(frames, responsesWebSocketFrame{controlType: websocket.PongMessage, payload: []byte(data)}, done, queue)
	})
	conn.SetCloseHandler(func(int, string) error { return nil })
}

func readResponsesWebSocketFrames(conn *websocket.Conn, frames chan<- responsesWebSocketFrame, done <-chan struct{}, codec *responsesWebSocketPrivateCodec, observability *responsesWebSocketObservability, upstream bool, queue *responsesWebSocketQueueStats) {
	defer close(frames)
	for {
		messageType, payload, err := conn.ReadMessage()
		if err == nil && codec != nil {
			messageType, payload, err = codec.Decode(messageType, payload)
		}
		if err == nil && upstream && observability != nil {
			observability.markApplicationRx()
		}
		frame := responsesWebSocketFrame{messageType: messageType, payload: payload, err: err}
		if enqueueResponsesWebSocketFrame(frames, frame, done, queue) != nil || err != nil {
			return
		}
	}
}

func startResponsesWebSocketReader(conn *websocket.Conn, done <-chan struct{}, codec *responsesWebSocketPrivateCodec, observability *responsesWebSocketObservability, upstream bool) <-chan responsesWebSocketFrame {
	frames := make(chan responsesWebSocketFrame, responsesWebSocketQueueSize)
	var queue *responsesWebSocketQueueStats
	if upstream && observability != nil {
		queue = observability.upstreamQueueStats()
	}
	configureResponsesWebSocketReader(conn, frames, done, codec, queue)
	readerConn := conn
	gopool.Go(func() {
		readResponsesWebSocketFrames(readerConn, frames, done, codec, observability, upstream, queue)
	})
	return frames
}

func cleanupResponsesWebSocketSession(active **responsesWebSocketRequestState, upstream **websocket.Conn, reason string) {
	if *active != nil {
		if reason != responsesWebSocketCleanupClientDisconnected {
			common.SetContextKey((*active).ctx, constant.ContextKeyWebSocketCloseReason, reason)
		}
		if reason == responsesWebSocketCleanupClientDisconnected {
			finalizeFailedResponsesWebSocketRequest(*active)
		} else {
			failResponsesWebSocketRequest(*active, nil, reason)
		}
		*active = nil
	}
	if *upstream != nil {
		_ = (*upstream).Close()
	}
}

func enqueueResponsesWebSocketFrame(frames chan<- responsesWebSocketFrame, frame responsesWebSocketFrame, done <-chan struct{}, queue *responsesWebSocketQueueStats) error {
	return enqueueResponsesWebSocketFrameWithSender(frame, queue, func() bool {
		select {
		case frames <- frame:
			return true
		case <-done:
			return false
		}
	})
}

func enqueueResponsesWebSocketFrameWithSender(frame responsesWebSocketFrame, queue *responsesWebSocketQueueStats, send func() bool) error {
	payloadBytes := len(frame.payload)
	if queue != nil {
		queue.enqueue(payloadBytes)
	}
	if send() {
		return nil
	}
	if queue != nil {
		queue.dequeue(payloadBytes)
	}
	return context.Canceled
}

func writeResponsesWebSocketMessage(conn *websocket.Conn, messageType int, payload []byte) error {
	if conn == nil {
		return errors.New("websocket connection is nil")
	}
	if err := conn.SetWriteDeadline(time.Now().Add(responsesWebSocketWriteTimeout)); err != nil {
		return err
	}
	return conn.WriteMessage(messageType, payload)
}

func writeResponsesWebSocketClientMessage(conn *websocket.Conn, codec *responsesWebSocketPrivateCodec, messageType int, payload []byte) error {
	if codec == nil {
		return writeResponsesWebSocketMessage(conn, messageType, payload)
	}
	wireType, wirePayload, err := codec.Encode(messageType, payload)
	if err != nil {
		return err
	}
	return writeResponsesWebSocketMessage(conn, wireType, wirePayload)
}

func writeResponsesWebSocketControl(conn *websocket.Conn, messageType int, payload []byte) error {
	if conn == nil {
		return errors.New("websocket connection is nil")
	}
	return conn.WriteControl(messageType, payload, time.Now().Add(responsesWebSocketWriteTimeout))
}

func writeResponsesWebSocketError(conn *websocket.Conn, codec *responsesWebSocketPrivateCodec, c *gin.Context, apiErr *types.NewAPIError) error {
	if apiErr == nil {
		return nil
	}
	status := 0
	if apiErr.GetErrorCode() == types.ErrorCodeResponsesWebSocketUnavailable {
		status = apiErr.StatusCode
	}
	return writeResponsesWebSocketErrorEvent(conn, codec, c, apiErr.ToOpenAIError(), status)
}

func writeResponsesWebSocketStateMissing(conn *websocket.Conn, codec *responsesWebSocketPrivateCodec, c *gin.Context) error {
	return writeResponsesWebSocketErrorEvent(conn, codec, c, types.OpenAIError{
		Message: "Previous response is not available on this websocket",
		Type:    "invalid_request",
		Code:    "previous_response_not_found",
	}, http.StatusConflict)
}

func writeResponsesWebSocketErrorEvent(conn *websocket.Conn, codec *responsesWebSocketPrivateCodec, c *gin.Context, openAIError types.OpenAIError, status int) error {
	requestID := ""
	if c != nil {
		requestID = c.GetString(common.RequestIdKey)
	}
	openAIError.Message = common.MessageWithRequestId(openAIError.Message, requestID)
	event := gin.H{"type": "error", "error": openAIError}
	if openAIError.Code == types.ErrorCodeResponsesWebSocketUnavailable {
		event["transport"] = "http"
		event["request_state"] = "not_submitted"
	}
	if status != 0 {
		event["status"] = status
	}
	payload, err := common.Marshal(event)
	if err != nil {
		return err
	}
	return writeResponsesWebSocketClientMessage(conn, codec, websocket.TextMessage, payload)
}

func writeResponsesWebSocketClose(conn *websocket.Conn, code int, reason string) error {
	reason = truncateResponsesWebSocketCloseReason(reason)
	return writeResponsesWebSocketControl(conn, websocket.CloseMessage, websocket.FormatCloseMessage(code, reason))
}

func propagateResponsesWebSocketClose(conn *websocket.Conn, err error) {
	if conn == nil {
		return
	}
	code := websocket.CloseGoingAway
	reason := "peer disconnected"
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		code = closeErr.Code
		reason = closeErr.Text
		if closeErr.Code == responsesWebSocketCapacityCloseCode {
			code = websocket.CloseInternalServerErr
			reason = "upstream websocket disconnected"
		}
	}
	_ = writeResponsesWebSocketClose(conn, responsesWebSocketCloseCode(code), reason)
}

func isNormalResponsesWebSocketClose(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway)
}

func isResponsesWebSocketClientDisconnectError(err error) bool {
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		return false
	}
	switch closeErr.Code {
	case websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
		websocket.CloseAbnormalClosure,
		websocket.CloseTLSHandshake:
		return true
	default:
		return false
	}
}

func responsesWebSocketCloseCode(code int) int {
	switch code {
	case websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseProtocolError,
		websocket.CloseUnsupportedData,
		websocket.CloseInvalidFramePayloadData,
		websocket.ClosePolicyViolation,
		websocket.CloseMessageTooBig,
		websocket.CloseMandatoryExtension,
		websocket.CloseInternalServerErr,
		websocket.CloseServiceRestart,
		websocket.CloseTryAgainLater:
		return code
	}
	if code >= 3000 && code <= 4999 {
		return code
	}
	return websocket.CloseInternalServerErr
}

func truncateResponsesWebSocketCloseReason(reason string) string {
	if len(reason) <= responsesWebSocketCloseMax {
		return reason
	}
	var result strings.Builder
	result.Grow(responsesWebSocketCloseMax)
	for len(reason) > 0 {
		_, size := utf8.DecodeRuneInString(reason)
		if size <= 0 || result.Len()+size > responsesWebSocketCloseMax {
			break
		}
		result.WriteString(reason[:size])
		reason = reason[size:]
	}
	return result.String()
}
