package controller

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	"github.com/klauspost/compress/zstd"
)

const (
	responsesWebSocketPrivateSubprotocol = "ai-cove-zstd.v1"
	responsesWebSocketPrivateHeaderSize  = 10
	responsesWebSocketPrivateFlagsOffset = 5
	responsesWebSocketPrivateMaxBytes    = 128 << 20

	responsesWebSocketPrivateFlagCompressed byte = 0x01
	responsesWebSocketPrivateFlagBinary     byte = 0x02
	responsesWebSocketPrivateAllowedFlags        = responsesWebSocketPrivateFlagCompressed | responsesWebSocketPrivateFlagBinary
)

var (
	responsesWebSocketPrivateMagic = []byte("AICZ")
	responsesWebSocketPrivateOnce  = sync.OnceValues(func() (*responsesWebSocketPrivateCodec, error) {
		encoder, err := zstd.NewWriter(nil,
			zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(3)),
			zstd.WithEncoderCRC(false),
		)
		if err != nil {
			return nil, fmt.Errorf("create websocket zstd encoder: %w", err)
		}
		decoder, err := zstd.NewReader(nil,
			zstd.WithDecoderMaxMemory(responsesWebSocketPrivateMaxBytes),
			zstd.WithDecoderMaxWindow(responsesWebSocketPrivateMaxBytes),
			zstd.WithDecodeAllCapLimit(true),
		)
		if err != nil {
			encoder.Close()
			return nil, fmt.Errorf("create websocket zstd decoder: %w", err)
		}
		return &responsesWebSocketPrivateCodec{encoder: encoder, decoder: decoder}, nil
	})
)

type responsesWebSocketPrivateCodec struct {
	encoder *zstd.Encoder
	decoder *zstd.Decoder
}

type responsesWebSocketPrivateProtocolError struct {
	closeCode int
	reason    string
	cause     error
}

func (e *responsesWebSocketPrivateProtocolError) Error() string {
	if e.cause == nil {
		return e.reason
	}
	return fmt.Sprintf("%s: %v", e.reason, e.cause)
}

func (e *responsesWebSocketPrivateProtocolError) Unwrap() error {
	return e.cause
}

func (e *responsesWebSocketPrivateProtocolError) CloseCode() int {
	return e.closeCode
}

func newResponsesWebSocketPrivateCodec() (*responsesWebSocketPrivateCodec, error) {
	return responsesWebSocketPrivateOnce()
}

func (c *responsesWebSocketPrivateCodec) Encode(messageType int, payload []byte) (int, []byte, error) {
	if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
		return 0, nil, newResponsesWebSocketPrivateProtocolError(websocket.CloseProtocolError, "unsupported application message type", nil)
	}
	if len(payload) > responsesWebSocketPrivateMaxBytes {
		return 0, nil, newResponsesWebSocketPrivateProtocolError(websocket.CloseMessageTooBig, "application message exceeds 128 MiB", nil)
	}
	if messageType == websocket.TextMessage && !utf8.Valid(payload) {
		return 0, nil, newResponsesWebSocketPrivateProtocolError(websocket.CloseInvalidFramePayloadData, "text message is not valid UTF-8", nil)
	}

	flags := byte(0)
	if messageType == websocket.BinaryMessage {
		flags |= responsesWebSocketPrivateFlagBinary
	}
	wirePayload := payload
	compressed := c.encoder.EncodeAll(payload, nil)
	if len(compressed) < len(payload) {
		flags |= responsesWebSocketPrivateFlagCompressed
		wirePayload = compressed
	}

	envelope := make([]byte, responsesWebSocketPrivateHeaderSize+len(wirePayload))
	copy(envelope, responsesWebSocketPrivateMagic)
	envelope[4] = 1
	envelope[responsesWebSocketPrivateFlagsOffset] = flags
	binary.BigEndian.PutUint32(envelope[6:10], uint32(len(payload)))
	copy(envelope[responsesWebSocketPrivateHeaderSize:], wirePayload)
	return websocket.BinaryMessage, envelope, nil
}

func (c *responsesWebSocketPrivateCodec) Decode(messageType int, envelope []byte) (int, []byte, error) {
	return c.decode(messageType, envelope, responsesWebSocketPrivateMaxBytes)
}

func (c *responsesWebSocketPrivateCodec) DecodeWithMaxBytes(messageType int, envelope []byte, maxBytes int) (int, []byte, error) {
	return c.decode(messageType, envelope, maxBytes)
}

func (c *responsesWebSocketPrivateCodec) decode(messageType int, envelope []byte, maxBytes int) (int, []byte, error) {
	if messageType != websocket.BinaryMessage {
		return 0, nil, newResponsesWebSocketPrivateProtocolError(websocket.CloseProtocolError, "private application message must be binary", nil)
	}
	if maxBytes <= 0 || len(envelope) > maxBytes {
		return 0, nil, newResponsesWebSocketPrivateProtocolError(websocket.CloseMessageTooBig, "private message exceeds configured limit", nil)
	}
	if len(envelope) < responsesWebSocketPrivateHeaderSize {
		return 0, nil, newResponsesWebSocketPrivateProtocolError(websocket.CloseProtocolError, "private message header is incomplete", nil)
	}
	if !bytes.Equal(envelope[:4], responsesWebSocketPrivateMagic) || envelope[4] != 1 {
		return 0, nil, newResponsesWebSocketPrivateProtocolError(websocket.CloseProtocolError, "private message magic or version is invalid", nil)
	}
	flags := envelope[responsesWebSocketPrivateFlagsOffset]
	if flags&^responsesWebSocketPrivateAllowedFlags != 0 {
		return 0, nil, newResponsesWebSocketPrivateProtocolError(websocket.CloseProtocolError, "private message flags are invalid", nil)
	}

	originalLength := uint64(binary.BigEndian.Uint32(envelope[6:10]))
	wirePayload := envelope[responsesWebSocketPrivateHeaderSize:]
	if originalLength > uint64(maxBytes) || len(wirePayload) > maxBytes {
		return 0, nil, newResponsesWebSocketPrivateProtocolError(websocket.CloseMessageTooBig, "private message exceeds configured limit", nil)
	}

	var payload []byte
	if flags&responsesWebSocketPrivateFlagCompressed != 0 {
		if uint64(len(wirePayload)) >= originalLength {
			return 0, nil, newResponsesWebSocketPrivateProtocolError(websocket.CloseProtocolError, "compressed payload is not smaller than original", nil)
		}
		decoded, err := c.decoder.DecodeAll(wirePayload, make([]byte, 0, int(originalLength)))
		if err != nil {
			return 0, nil, newResponsesWebSocketPrivateProtocolError(websocket.CloseInvalidFramePayloadData, "zstd payload is invalid", err)
		}
		payload = decoded
	} else {
		if uint64(len(wirePayload)) != originalLength {
			return 0, nil, newResponsesWebSocketPrivateProtocolError(websocket.CloseProtocolError, "uncompressed payload length does not match", nil)
		}
		payload = append([]byte(nil), wirePayload...)
	}
	if uint64(len(payload)) != originalLength {
		closeCode := websocket.CloseProtocolError
		if flags&responsesWebSocketPrivateFlagCompressed != 0 {
			closeCode = websocket.CloseInvalidFramePayloadData
		}
		return 0, nil, newResponsesWebSocketPrivateProtocolError(closeCode, "decoded payload length does not match", nil)
	}

	decodedType := websocket.TextMessage
	if flags&responsesWebSocketPrivateFlagBinary != 0 {
		decodedType = websocket.BinaryMessage
	} else if !utf8.Valid(payload) {
		return 0, nil, newResponsesWebSocketPrivateProtocolError(websocket.CloseInvalidFramePayloadData, "text payload is not valid UTF-8", nil)
	}
	return decodedType, payload, nil
}

func newResponsesWebSocketPrivateProtocolError(code int, reason string, cause error) *responsesWebSocketPrivateProtocolError {
	return &responsesWebSocketPrivateProtocolError{closeCode: code, reason: reason, cause: cause}
}
