package controller

import (
	"context"
	"errors"
	"strings"
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
)

type responsesWebSocketFrame struct {
	messageType int
	payload     []byte
	err         error
	controlType int
}

func configureResponsesWebSocketReader(conn *websocket.Conn, frames chan<- responsesWebSocketFrame, done <-chan struct{}) {
	maxMB := constant.MaxRequestBodyMB
	if maxMB <= 0 {
		maxMB = 128
	}
	conn.SetReadLimit(int64(maxMB) << 20)
	conn.SetPingHandler(func(data string) error {
		return enqueueResponsesWebSocketFrame(frames, responsesWebSocketFrame{controlType: websocket.PongMessage, payload: []byte(data)}, done)
	})
	conn.SetCloseHandler(func(int, string) error { return nil })
}

func readResponsesWebSocketFrames(conn *websocket.Conn, frames chan<- responsesWebSocketFrame, done <-chan struct{}) {
	defer close(frames)
	for {
		messageType, payload, err := conn.ReadMessage()
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

func writeResponsesWebSocketControl(conn *websocket.Conn, messageType int, payload []byte) error {
	if conn == nil {
		return errors.New("websocket connection is nil")
	}
	return conn.WriteControl(messageType, payload, time.Now().Add(responsesWebSocketWriteTimeout))
}

func writeResponsesWebSocketError(conn *websocket.Conn, c *gin.Context, apiErr *types.NewAPIError) error {
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
	return writeResponsesWebSocketMessage(conn, websocket.TextMessage, payload)
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
