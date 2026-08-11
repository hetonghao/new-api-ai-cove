package controller

import (
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
	clientConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer clientConn.Close()

	var clientCodec *responsesWebSocketPrivateCodec
	if clientConn.Subprotocol() == responsesWebSocketPrivateSubprotocol {
		clientCodec, err = newResponsesWebSocketPrivateCodec()
		if err != nil {
			_ = writeResponsesWebSocketClose(clientConn, websocket.CloseInternalServerErr, "private websocket codec unavailable")
			return
		}
	}

	if err := runResponsesWebSocketSession(c, clientConn, clientCodec); err != nil {
		logger.LogError(c, fmt.Sprintf("responses websocket session ended: %s", common.LocalLogPreview(err.Error())))
	}
}
