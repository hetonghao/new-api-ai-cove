package controller

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"net/http"
	"strings"
	"testing"
	"time"
)

// One actual private connection carries all three states, through a pinned channel and its public usage logs.
func TestTurboImageCompressionPrivateTurns(t *testing.T) {
	received := make(chan string, 3)
	fixture := newResponsesWSBillingFixture(t, true, "tier(\"request\", fixed(0.002))", func(ws *websocket.Conn, _ *http.Request) {
		for i := 1; i <= 3; i++ {
			_, data, err := ws.ReadMessage()
			if err != nil {
				return
			}
			received <- string(data)
			id := fmt.Sprintf("resp_%d", i)
			if ws.WriteJSON(map[string]any{"type": "response.created", "response": map[string]any{"id": id, "status": "in_progress"}}) != nil {
				return
			}
			if ws.WriteJSON(map[string]any{"type": "response.completed", "response": map[string]any{"id": id, "status": "completed", "output": []any{}, "usage": map[string]int{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15}}}) != nil {
				return
			}
		}
		_, _, _ = ws.ReadMessage()
	})
	codec, err := newResponsesWebSocketPrivateCodec()
	require.NoError(t, err)
	turns := []struct {
		input, marker string
		wire          bool
	}{{strings.Repeat("image ", 700), "v1/changed/20", true}, {"text", "", false}, {strings.Repeat("text ", 700), "", true}}
	for _, turn := range turns {
		req := map[string]any{"type": "response.create", "model": "ws-billing", "input": turn.input}
		if turn.marker != "" {
			req["client_metadata"] = map[string]string{common.TurboImageMetadata: turn.marker, "keep": "yes"}
		}
		payload, err := common.Marshal(req)
		require.NoError(t, err)
		typ, frame, err := codec.Encode(websocket.TextMessage, payload)
		require.NoError(t, err)
		require.Equal(t, turn.wire, frame[5]&1 != 0)
		require.NoError(t, fixture.client.WriteMessage(typ, frame))
		seenReceipt := false
		for {
			require.NoError(t, fixture.client.SetReadDeadline(time.Now().Add(5*time.Second)))
			typ, wire, err := fixture.client.ReadMessage()
			require.NoError(t, err)
			_, data, err := codec.Decode(typ, wire)
			require.NoError(t, err)
			switch gjson.GetBytes(data, "type").Str {
			case "ai_cove.image_receipt":
				seenReceipt = true
				require.NotEmpty(t, gjson.GetBytes(data, "request_id").Str)
			case "error":
				t.Fatalf("unexpected gateway response: %s", data)
			case "response.completed":
				goto completed
			}
		}
	completed:
		require.Equal(t, turn.marker != "", seenReceipt)
		forwarded := <-received
		require.NotContains(t, forwarded, common.TurboImageMetadata)
		if turn.marker != "" {
			require.Equal(t, "yes", gjson.Get(forwarded, "client_metadata.keep").Str)
		}
	}
	var logs []model.Log
	require.Eventually(t, func() bool {
		return model.LOG_DB.Where("token_id = ? AND type = ?", fixture.token.Id, model.LogTypeConsume).Order("id asc").Find(&logs).Error == nil && len(logs) == 3
	}, time.Second*5, time.Millisecond*20)
	for i, row := range logs {
		require.Equal(t, turns[i].wire, gjson.Get(row.Other, "turbo_zstd").Bool())
		want := int64(0)
		if i == 0 {
			want = 20
		}
		require.Equal(t, want, gjson.Get(row.Other, "turbo_images_compressed").Int())
	}
}
