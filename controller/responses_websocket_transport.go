package controller

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"

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

func configureResponsesWebSocketReader(conn *websocket.Conn, frames chan<- responsesWebSocketFrame, done <-chan struct{}, codec *responsesWebSocketPrivateCodec) {
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
		return enqueueResponsesWebSocketFrame(frames, responsesWebSocketFrame{controlType: websocket.PongMessage, payload: []byte(data)}, done)
	})
	conn.SetCloseHandler(func(int, string) error { return nil })
}

func readResponsesWebSocketFrames(conn *websocket.Conn, frames chan<- responsesWebSocketFrame, done <-chan struct{}, codec *responsesWebSocketPrivateCodec) {
	defer close(frames)
	for {
		messageType, payload, err := conn.ReadMessage()
		if err == nil && codec != nil {
			messageType, payload, err = codec.Decode(messageType, payload)
		}
		frame := responsesWebSocketFrame{messageType: messageType, payload: payload, err: err}
		if enqueueResponsesWebSocketFrame(frames, frame, done) != nil || err != nil {
			return
		}
	}
}

func enqueueResponsesWebSocketFrame(frames chan<- responsesWebSocketFrame, frame responsesWebSocketFrame, done <-chan struct{}) error {
	select {
	case frames <- frame:
		return nil
	case <-done:
		return context.Canceled
	}
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
	openAIError := apiErr.ToOpenAIError()
	requestID := ""
	if c != nil {
		requestID = c.GetString(common.RequestIdKey)
	}
	openAIError.Message = common.MessageWithRequestId(openAIError.Message, requestID)
	payload, err := common.Marshal(gin.H{"type": "error", "error": openAIError})
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
	}
	_ = writeResponsesWebSocketClose(conn, responsesWebSocketCloseCode(code), reason)
}

func isNormalResponsesWebSocketClose(err error) bool {
	return errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway)
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
