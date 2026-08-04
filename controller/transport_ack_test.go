package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

const transportAckTestPath = "/v1/transport/ack"

type transportAckTestResponse struct {
	OK           bool    `json:"ok"`
	Transport    string  `json:"transport"`
	WireBytes    int     `json:"wire_bytes"`
	DecodedBytes int     `json:"decoded_bytes"`
	ReceiveMs    float64 `json:"receive_ms"`
	DecodeMs     float64 `json:"decode_ms"`
}

func requireTransportAckResponseKeys(t *testing.T, payload []byte) {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(payload, &fields))
	approved := []string{"ok", "transport", "wire_bytes", "decoded_bytes", "receive_ms", "decode_ms"}
	require.Len(t, fields, len(approved))
	for _, key := range approved {
		require.Contains(t, fields, key)
	}
}

func TestTransportAckMillisecondsPreservesSubMillisecondPrecision(t *testing.T) {
	require.Equal(t, 1.5, transportAckMilliseconds(1500*time.Microsecond))
}

func newTransportAckTestRouter() *gin.Engine {
	router := gin.New()
	router.Use(middleware.TransportAckRequestBodyLimit())
	router.Use(middleware.DecompressRequestMiddleware())
	router.POST(transportAckTestPath, TransportAckHTTP)
	router.GET(transportAckTestPath, TransportAckWebSocket)
	return router
}

func TestTransportAckHTTPReadsAndAcknowledgesWithoutEchoingBody(t *testing.T) {
	router := newTransportAckTestRouter()
	payload := `{"kind":"transport-ack","secret":"do-not-echo"}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, transportAckTestPath, strings.NewReader(payload))

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	requireTransportAckResponseKeys(t, recorder.Body.Bytes())
	var response transportAckTestResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.OK)
	require.Equal(t, "http", response.Transport)
	require.Equal(t, len(payload), response.WireBytes)
	require.Equal(t, len(payload), response.DecodedBytes)
	require.NotContains(t, recorder.Body.String(), "do-not-echo")
}

func TestTransportAckHTTPRejectsDecodedPayloadOverOneMiB(t *testing.T) {
	router := newTransportAckTestRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, transportAckTestPath, strings.NewReader(strings.Repeat("x", 1<<20+1)))

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}

func TestTransportAckHTTPReportsEncodedBytesAfterZstdDecode(t *testing.T) {
	router := newTransportAckTestRouter()
	payload := []byte(strings.Repeat(`{"kind":"transport-ack"}`, 256))
	encoder, err := zstd.NewWriter(nil)
	require.NoError(t, err)
	encoded := encoder.EncodeAll(payload, nil)
	encoder.Close()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, transportAckTestPath, strings.NewReader(string(encoded)))
	request.Header.Set("Content-Encoding", "zstd")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response transportAckTestResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, len(encoded), response.WireBytes)
	require.Equal(t, len(payload), response.DecodedBytes)
	require.NotContains(t, recorder.Body.String(), "transport-ack")
}

func TestTransportAckHTTPAcceptsCaseInsensitiveZstdEncoding(t *testing.T) {
	router := newTransportAckTestRouter()
	payload := []byte(`{"kind":"transport-ack"}`)
	encoder, err := zstd.NewWriter(nil)
	require.NoError(t, err)
	encoded := encoder.EncodeAll(payload, nil)
	encoder.Close()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, transportAckTestPath, strings.NewReader(string(encoded)))
	request.Header.Set("Content-Encoding", "ZSTD")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response transportAckTestResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, len(encoded), response.WireBytes)
	require.Equal(t, len(payload), response.DecodedBytes)
}

func TestTransportAckHTTPRejectsDecodedPayloadOverOneMiBWhenWirePayloadIsSmall(t *testing.T) {
	router := newTransportAckTestRouter()
	payload := []byte(strings.Repeat("x", constant.TransportAckMaxBytes+1))
	encoder, err := zstd.NewWriter(nil)
	require.NoError(t, err)
	encoded := encoder.EncodeAll(payload, nil)
	encoder.Close()
	require.Less(t, len(encoded), constant.TransportAckMaxBytes)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, transportAckTestPath, strings.NewReader(string(encoded)))
	request.Header.Set("Content-Encoding", "zstd")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}

func TestTransportAckHTTPRejectsInvalidEncoding(t *testing.T) {
	router := newTransportAckTestRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, transportAckTestPath, strings.NewReader("not-zstd"))
	request.Header.Set("Content-Encoding", "zstd")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestTransportAckHTTPRejectsUnsupportedEncoding(t *testing.T) {
	router := newTransportAckTestRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, transportAckTestPath, strings.NewReader("payload"))
	request.Header.Set("Content-Encoding", "deflate")

	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestTransportAckWebSocketAcknowledgesStandardMessage(t *testing.T) {
	server := httptest.NewServer(newTransportAckTestRouter())
	defer server.Close()
	dialer := websocket.Dialer{}
	client, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+transportAckTestPath, nil)
	require.NoError(t, err)
	defer client.Close()

	payload := []byte(`{"kind":"transport-ack","secret":"do-not-echo"}`)
	require.NoError(t, client.WriteMessage(websocket.TextMessage, payload))
	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	messageType, responsePayload, err := client.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, websocket.TextMessage, messageType)
	requireTransportAckResponseKeys(t, responsePayload)
	var response transportAckTestResponse
	require.NoError(t, common.Unmarshal(responsePayload, &response))
	require.True(t, response.OK)
	require.Equal(t, "websocket", response.Transport)
	require.Equal(t, len(payload), response.WireBytes)
	require.Equal(t, len(payload), response.DecodedBytes)
	require.NotContains(t, string(responsePayload), "do-not-echo")
}

func TestTransportAckWebSocketAcknowledgesPrivateZstdMessage(t *testing.T) {
	server := httptest.NewServer(newTransportAckTestRouter())
	defer server.Close()
	dialer := websocket.Dialer{Subprotocols: []string{responsesWebSocketPrivateSubprotocol}}
	client, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+transportAckTestPath, nil)
	require.NoError(t, err)
	defer client.Close()

	codec, err := newResponsesWebSocketPrivateCodec()
	require.NoError(t, err)
	payload := []byte(strings.Repeat(`{"kind":"transport-ack","secret":"do-not-echo"}`, 32))
	wireType, wirePayload, err := codec.Encode(websocket.TextMessage, payload)
	require.NoError(t, err)
	require.NoError(t, client.WriteMessage(wireType, wirePayload))
	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	responseType, responsePayload, err := client.ReadMessage()
	require.NoError(t, err)
	_, decodedResponse, err := codec.Decode(responseType, responsePayload)
	require.NoError(t, err)
	requireTransportAckResponseKeys(t, decodedResponse)
	var response transportAckTestResponse
	require.NoError(t, common.Unmarshal(decodedResponse, &response))
	require.True(t, response.OK)
	require.Equal(t, "websocket", response.Transport)
	require.Equal(t, len(wirePayload), response.WireBytes)
	require.Equal(t, len(payload), response.DecodedBytes)
	require.NotContains(t, string(decodedResponse), "do-not-echo")
}

func TestTransportAckWebSocketRejectsOversizedStandardMessage(t *testing.T) {
	server := httptest.NewServer(newTransportAckTestRouter())
	defer server.Close()
	client, _, err := (&websocket.Dialer{}).Dial("ws"+strings.TrimPrefix(server.URL, "http")+transportAckTestPath, nil)
	require.NoError(t, err)
	defer client.Close()

	require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", constant.TransportAckMaxBytes+1))))
	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, _, err = client.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.CloseMessageTooBig, closeErr.Code)
}

func TestTransportAckWebSocketRejectsOversizedPrivateDecodedMessage(t *testing.T) {
	server := httptest.NewServer(newTransportAckTestRouter())
	defer server.Close()
	client, _, err := (&websocket.Dialer{Subprotocols: []string{responsesWebSocketPrivateSubprotocol}}).Dial("ws"+strings.TrimPrefix(server.URL, "http")+transportAckTestPath, nil)
	require.NoError(t, err)
	defer client.Close()
	codec, err := newResponsesWebSocketPrivateCodec()
	require.NoError(t, err)
	wireType, wirePayload, err := codec.Encode(websocket.TextMessage, []byte(strings.Repeat("x", constant.TransportAckMaxBytes+1)))
	require.NoError(t, err)
	require.NoError(t, client.WriteMessage(wireType, wirePayload))
	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, _, err = client.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.CloseMessageTooBig, closeErr.Code)
}

func TestTransportAckWebSocketRejectsOversizedPrivateEnvelope(t *testing.T) {
	server := httptest.NewServer(newTransportAckTestRouter())
	defer server.Close()
	client, _, err := (&websocket.Dialer{Subprotocols: []string{responsesWebSocketPrivateSubprotocol}}).Dial("ws"+strings.TrimPrefix(server.URL, "http")+transportAckTestPath, nil)
	require.NoError(t, err)
	defer client.Close()
	codec, err := newResponsesWebSocketPrivateCodec()
	require.NoError(t, err)
	wireType, wirePayload, err := codec.Encode(websocket.TextMessage, []byte("small"))
	require.NoError(t, err)
	wirePayload = append(wirePayload, make([]byte, constant.TransportAckMaxBytes)...)
	require.NoError(t, client.WriteMessage(wireType, wirePayload))
	require.NoError(t, client.SetReadDeadline(time.Now().Add(5*time.Second)))
	_, _, err = client.ReadMessage()
	var closeErr *websocket.CloseError
	require.ErrorAs(t, err, &closeErr)
	require.Equal(t, websocket.CloseMessageTooBig, closeErr.Code)
}
