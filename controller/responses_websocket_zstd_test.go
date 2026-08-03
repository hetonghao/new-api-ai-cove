package controller

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesWebSocketPrivateCodec_matches_uncompressed_text_vector(t *testing.T) {
	codec, err := newResponsesWebSocketPrivateCodec()
	require.NoError(t, err)

	wireType, wirePayload, err := codec.Encode(websocket.TextMessage, []byte("ok"))

	require.NoError(t, err)
	require.Equal(t, websocket.BinaryMessage, wireType)
	require.Equal(t, "4149435a0100000000026f6b", hex.EncodeToString(wirePayload))
	messageType, payload, err := codec.Decode(wireType, wirePayload)
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, messageType)
	require.Equal(t, []byte("ok"), payload)
}

func TestResponsesWebSocketPrivateCodec_decodes_compressed_interop_vector(t *testing.T) {
	codec, err := newResponsesWebSocketPrivateCodec()
	require.NoError(t, err)
	wirePayload := mustDecodeHex(t, "4149435a01010000250028b52ffd6000247d010054027b2274797065223a22726573706f6e73652e637265617465222c22696e707574223a5b5d7d0154160531c52628")

	messageType, payload, err := codec.Decode(websocket.BinaryMessage, wirePayload)

	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, messageType)
	require.Equal(t, strings.Repeat(`{"type":"response.create","input":[]}`, 256), string(payload))
}

func TestResponsesWebSocketPrivateCodec_round_trips_compressed_text(t *testing.T) {
	codec, err := newResponsesWebSocketPrivateCodec()
	require.NoError(t, err)
	payload := []byte(strings.Repeat(`{"type":"response.create","input":[]}`, 256))

	wireType, wirePayload, err := codec.Encode(websocket.TextMessage, payload)

	require.NoError(t, err)
	require.Equal(t, websocket.BinaryMessage, wireType)
	require.Equal(t, byte(responsesWebSocketPrivateFlagCompressed), wirePayload[responsesWebSocketPrivateFlagsOffset])
	messageType, decoded, err := codec.Decode(wireType, wirePayload)
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, messageType)
	require.Equal(t, payload, decoded)
}

func TestResponsesWebSocketPrivateCodec_round_trips_binary(t *testing.T) {
	codec, err := newResponsesWebSocketPrivateCodec()
	require.NoError(t, err)
	payload := []byte{0x00, 0xff, 0x10, 0x20}

	wireType, wirePayload, err := codec.Encode(websocket.BinaryMessage, payload)

	require.NoError(t, err)
	require.Equal(t, websocket.BinaryMessage, wireType)
	require.Equal(t, byte(responsesWebSocketPrivateFlagBinary), wirePayload[responsesWebSocketPrivateFlagsOffset])
	messageType, decoded, err := codec.Decode(wireType, wirePayload)
	require.NoError(t, err)
	require.Equal(t, websocket.BinaryMessage, messageType)
	require.Equal(t, payload, decoded)
}

func TestResponsesWebSocketPrivateCodec_rejects_invalid_messages(t *testing.T) {
	codec, err := newResponsesWebSocketPrivateCodec()
	require.NoError(t, err)

	tests := []struct {
		name        string
		messageType int
		payload     []byte
		closeCode   int
	}{
		{name: "text wire frame", messageType: websocket.TextMessage, payload: []byte("raw"), closeCode: websocket.CloseProtocolError},
		{name: "short header", messageType: websocket.BinaryMessage, payload: []byte("AICZ"), closeCode: websocket.CloseProtocolError},
		{name: "unknown flags", messageType: websocket.BinaryMessage, payload: mustDecodeHex(t, "4149435a018000000000"), closeCode: websocket.CloseProtocolError},
		{name: "invalid zstd", messageType: websocket.BinaryMessage, payload: mustDecodeHex(t, "4149435a0101000000106e6f742d7a737464"), closeCode: websocket.CloseInvalidFramePayloadData},
		{name: "decoded length mismatch", messageType: websocket.BinaryMessage, payload: mustDecodeHex(t, "4149435a01010000250128b52ffd6000247d010054027b2274797065223a22726573706f6e73652e637265617465222c22696e707574223a5b5d7d0154160531c52628"), closeCode: websocket.CloseInvalidFramePayloadData},
		{name: "oversized zstd window", messageType: websocket.BinaryMessage, payload: mustDecodeHex(t, "4149435a01010000006428b52ffd009023030078"), closeCode: websocket.CloseInvalidFramePayloadData},
		{name: "invalid utf8", messageType: websocket.BinaryMessage, payload: mustDecodeHex(t, "4149435a010000000001ff"), closeCode: websocket.CloseInvalidFramePayloadData},
		{name: "too large", messageType: websocket.BinaryMessage, payload: mustDecodeHex(t, "4149435a010008000001"), closeCode: websocket.CloseMessageTooBig},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := codec.Decode(test.messageType, test.payload)
			var protocolErr *responsesWebSocketPrivateProtocolError
			require.ErrorAs(t, err, &protocolErr)
			require.Equalf(t, test.closeCode, protocolErr.CloseCode(), "%s", protocolErr)
		})
	}
}

func TestResponsesWebSocket_negotiates_private_zstd_subprotocol(t *testing.T) {
	webSocketURL, done := newResponsesWebSocketProtocolTestServer(t)

	dialer := websocket.Dialer{EnableCompression: true, Subprotocols: []string{responsesWebSocketPrivateSubprotocol}}
	client, response, err := dialer.Dial(webSocketURL, nil)
	require.NoError(t, err)
	defer closeResponsesWebSocketProtocolTestClient(t, client, done)

	require.Equal(t, responsesWebSocketPrivateSubprotocol, client.Subprotocol())
	require.Equal(t, responsesWebSocketPrivateSubprotocol, response.Header.Get("Sec-WebSocket-Protocol"))
	require.Empty(t, response.Header.Get("Sec-WebSocket-Extensions"))
}

func TestResponsesWebSocket_does_not_select_unknown_private_subprotocol(t *testing.T) {
	webSocketURL, done := newResponsesWebSocketProtocolTestServer(t)

	dialer := websocket.Dialer{Subprotocols: []string{"ai-cove-zstd.v2"}}
	client, response, err := dialer.Dial(webSocketURL, nil)
	require.NoError(t, err)
	defer closeResponsesWebSocketProtocolTestClient(t, client, done)

	require.Empty(t, client.Subprotocol())
	require.Empty(t, response.Header.Get("Sec-WebSocket-Protocol"))
}

func TestResponsesWebSocket_private_zstd_closes_with_protocol_error_for_plain_data_frame(t *testing.T) {
	webSocketURL, done := newResponsesWebSocketProtocolTestServer(t)

	dialer := websocket.Dialer{Subprotocols: []string{responsesWebSocketPrivateSubprotocol}}
	client, _, err := dialer.Dial(webSocketURL, nil)
	require.NoError(t, err)
	defer closeResponsesWebSocketProtocolTestClient(t, client, done)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create"}`)))

	_, _, err = client.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.CloseProtocolError, closeErr.Code)
}

func TestResponsesWebSocket_private_zstd_keeps_control_frames_standard(t *testing.T) {
	webSocketURL, done := newResponsesWebSocketProtocolTestServer(t)
	dialer := websocket.Dialer{Subprotocols: []string{responsesWebSocketPrivateSubprotocol}}
	client, _, err := dialer.Dial(webSocketURL, nil)
	require.NoError(t, err)
	defer closeResponsesWebSocketProtocolTestClient(t, client, done)

	pong := make(chan string, 1)
	client.SetPongHandler(func(data string) error {
		pong <- data
		return nil
	})
	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	readDone := make(chan error, 1)
	go func() {
		_, _, err := client.ReadMessage()
		readDone <- err
	}()
	require.NoError(t, client.WriteControl(websocket.PingMessage, []byte("probe"), time.Now().Add(time.Second)))

	select {
	case payload := <-pong:
		require.Equal(t, "probe", payload)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for private websocket pong")
	}
	_ = client.Close()
	select {
	case <-readDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for websocket reader shutdown")
	}
}

func TestResponsesWebSocket_private_zstd_forwards_responses_messages(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	upstreamRequest := make(chan []byte, 1)
	upstream := newResponsesWebSocketTestUpstream(t, func(conn *websocket.Conn) {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read upstream request: %v", err)
			return
		}
		if messageType != websocket.TextMessage {
			t.Errorf("unexpected upstream message type: %d", messageType)
			return
		}
		upstreamRequest <- payload
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)); err != nil {
			t.Errorf("write upstream response: %v", err)
		}
	})
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 301, baseURL: upstream.server.URL, priority: 0})
	client := dialResponsesWebSocketTestClientWithContextAndSubprotocol(t, nil, responsesWebSocketPrivateSubprotocol)
	codec, err := newResponsesWebSocketPrivateCodec()
	require.NoError(t, err)
	create := []byte(`{"type":"response.create","model":"gpt-4o-mini","input":[]}`)
	wireType, wirePayload, err := codec.Encode(websocket.TextMessage, create)
	require.NoError(t, err)

	require.NoError(t, client.WriteMessage(wireType, wirePayload))
	responseType, responsePayload, err := client.ReadMessage()
	require.NoError(t, err)
	messageType, decoded, err := codec.Decode(responseType, responsePayload)
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, messageType)
	require.Equal(t, "response.completed", gjson.GetBytes(decoded, "type").String())
	require.Equal(t, "response.create", gjson.GetBytes(<-upstreamRequest, "type").String())
	closeResponsesWebSocketTestClient(client)
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	require.NoError(t, err)
	return decoded
}
