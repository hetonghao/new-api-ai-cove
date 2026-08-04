package controller

import (
	"errors"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type transportAckResponse struct {
	OK           bool    `json:"ok"`
	Transport    string  `json:"transport"`
	WireBytes    int     `json:"wire_bytes"`
	DecodedBytes int     `json:"decoded_bytes"`
	ReceiveMs    float64 `json:"receive_ms"`
	DecodeMs     float64 `json:"decode_ms"`
}

func transportAckMilliseconds(duration time.Duration) float64 {
	return duration.Seconds() * 1000
}

func TransportAckHTTP(c *gin.Context) {
	started := time.Now()
	if c.Request.Body == nil {
		c.Request.Body = http.NoBody
	}
	defer c.Request.Body.Close()

	payload, err := io.ReadAll(io.LimitReader(c.Request.Body, constant.TransportAckMaxBytes+1))
	if err != nil {
		if common.IsRequestBodyTooLargeError(err) {
			c.Status(http.StatusRequestEntityTooLarge)
			return
		}
		c.Status(http.StatusBadRequest)
		return
	}
	if len(payload) > constant.TransportAckMaxBytes {
		c.Status(http.StatusRequestEntityTooLarge)
		return
	}

	var readMetrics *common.TransportAckReadMetrics
	if value, ok := c.Get(common.KeyTransportAckReadMetrics); ok {
		readMetrics, _ = value.(*common.TransportAckReadMetrics)
	}
	wireBytes := len(payload)
	receiveDuration := time.Since(started)
	if readMetrics != nil {
		wireBytes = readMetrics.RawBytes
		receiveDuration = readMetrics.RawReadDuration
	}
	decodeDuration := time.Since(started) - receiveDuration
	if decodeDuration < 0 {
		decodeDuration = 0
	}

	response, err := common.Marshal(transportAckResponse{
		OK:           true,
		Transport:    "http",
		WireBytes:    wireBytes,
		DecodedBytes: len(payload),
		ReceiveMs:    transportAckMilliseconds(receiveDuration),
		DecodeMs:     transportAckMilliseconds(decodeDuration),
	})
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	c.Data(http.StatusOK, "application/json", response)
}

func TransportAckWebSocket(c *gin.Context) {
	upgrader := responsesWebSocketUpgrader
	if slices.Contains(websocket.Subprotocols(c.Request), responsesWebSocketPrivateSubprotocol) {
		upgrader.EnableCompression = false
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(responsesWebSocketWriteTimeout)); err != nil {
		return
	}
	conn.SetReadLimit(int64(constant.TransportAckMaxBytes))

	started := time.Now()
	wireType, wirePayload, err := conn.ReadMessage()
	if err != nil {
		var closeErr *websocket.CloseError
		closeCode := websocket.CloseProtocolError
		if errors.As(err, &closeErr) && closeErr.Code == websocket.CloseMessageTooBig {
			closeCode = websocket.CloseMessageTooBig
		}
		_ = writeResponsesWebSocketClose(conn, closeCode, "transport ack message rejected")
		return
	}
	receiveDuration := time.Since(started)
	decodedType := wireType
	decodedPayload := wirePayload
	decodeStarted := time.Now()
	var clientCodec *responsesWebSocketPrivateCodec
	if conn.Subprotocol() == responsesWebSocketPrivateSubprotocol {
		clientCodec, err = newResponsesWebSocketPrivateCodec()
		if err == nil {
			decodedType, decodedPayload, err = clientCodec.DecodeWithMaxBytes(wireType, wirePayload, constant.TransportAckMaxBytes)
		}
	}
	decodeDuration := time.Since(decodeStarted)
	if err != nil {
		closeCode := websocket.CloseProtocolError
		var protocolErr *responsesWebSocketPrivateProtocolError
		if errors.As(err, &protocolErr) {
			closeCode = protocolErr.CloseCode()
		}
		_ = writeResponsesWebSocketClose(conn, closeCode, "transport ack message rejected")
		return
	}
	if decodedType != websocket.TextMessage && decodedType != websocket.BinaryMessage {
		_ = writeResponsesWebSocketClose(conn, websocket.CloseUnsupportedData, "transport ack message rejected")
		return
	}
	response, err := common.Marshal(transportAckResponse{
		OK:           true,
		Transport:    "websocket",
		WireBytes:    len(wirePayload),
		DecodedBytes: len(decodedPayload),
		ReceiveMs:    transportAckMilliseconds(receiveDuration),
		DecodeMs:     transportAckMilliseconds(decodeDuration),
	})
	if err != nil {
		_ = writeResponsesWebSocketClose(conn, websocket.CloseInternalServerErr, "transport ack response failed")
		return
	}
	responseType := websocket.TextMessage
	responsePayload := response
	if clientCodec != nil {
		responseType, responsePayload, err = clientCodec.Encode(websocket.TextMessage, response)
		if err != nil {
			_ = writeResponsesWebSocketClose(conn, websocket.CloseInternalServerErr, "transport ack response failed")
			return
		}
	}
	_ = conn.WriteMessage(responseType, responsePayload)
}
