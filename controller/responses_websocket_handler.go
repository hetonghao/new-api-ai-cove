package controller

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"slices"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var responsesWebSocketUpgrader = websocket.Upgrader{
	ReadBufferSize:    4096,
	WriteBufferSize:   4096,
	EnableCompression: true,
	Subprotocols:      []string{responsesWebSocketPrivateSubprotocol},
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

func ResponsesWebSocket(c *gin.Context) {
	upgrader := responsesWebSocketUpgrader
	if slices.Contains(websocket.Subprotocols(c.Request), responsesWebSocketPrivateSubprotocol) {
		upgrader.EnableCompression = false
	}
	traceBytes := make([]byte, 16)
	if _, err := rand.Read(traceBytes); err != nil {
		logger.LogError(c, fmt.Sprintf("responses websocket trace generation failed: %s", err.Error()))
		return
	}
	responseHeader := http.Header{}
	responseHeader.Set(common.ResponsesWebSocketTraceHeader, hex.EncodeToString(traceBytes))
	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, responseHeader)
	if err != nil {
		return
	}
	defer clientConn.Close()
	observability := newResponsesWebSocketObservability(responseHeader.Get(common.ResponsesWebSocketTraceHeader))

	var clientCodec *responsesWebSocketPrivateCodec
	if clientConn.Subprotocol() == responsesWebSocketPrivateSubprotocol {
		clientCodec, err = newResponsesWebSocketPrivateCodec()
		if err != nil {
			_ = writeResponsesWebSocketClose(clientConn, websocket.CloseInternalServerErr, "private websocket codec unavailable")
			observability.markFailure("private_codec_unavailable")
			observability.markCleanup()
			observability.log(c, "cleanup")
			return
		}
	}

	if err := runResponsesWebSocketSession(c, clientConn, clientCodec, observability); err != nil {
		logger.LogError(c, fmt.Sprintf("responses websocket session ended: %s", common.LocalLogPreview(err.Error())))
	}
}
