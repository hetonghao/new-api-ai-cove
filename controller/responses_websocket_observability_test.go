package controller

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestResponsesWebSocketQueueAccountsBeforeSend(t *testing.T) {
	queue := newResponsesWebSocketQueueStats(responsesWebSocketQueueSize)
	var observedMessages int64
	var observedBytes int64

	err := enqueueResponsesWebSocketFrameWithSender(
		responsesWebSocketFrame{payload: []byte("abc")},
		queue,
		func() bool {
			observedMessages = queue.messages.Load()
			observedBytes = queue.bytes.Load()
			return true
		},
	)

	require.NoError(t, err)
	require.EqualValues(t, 1, observedMessages)
	require.EqualValues(t, 3, observedBytes)
}

func TestResponsesWebSocketRuntimeFailureKeepsSpecificReason(t *testing.T) {
	observability := newResponsesWebSocketObservability("0123456789abcdef0123456789abcdef")

	observability.markFailure(responsesWebSocketFailureCode("upstream write failed"))
	observability.markFailure("response_prepare_failed")
	snapshot := observability.snapshot(time.Now())

	require.Equal(t, "upstream_write_failed", snapshot.FailureReason)
}

func TestResponsesWebSocketObservabilityUsesStableMessageWithFields(t *testing.T) {
	var output bytes.Buffer
	previousWriter := gin.DefaultWriter
	gin.DefaultWriter = &output
	t.Cleanup(func() {
		gin.DefaultWriter = previousWriter
	})

	observability := newResponsesWebSocketObservability("0123456789abcdef0123456789abcdef")
	observability.acceptResponseCreate()
	observability.markCleanup()
	observability.log(context.Background(), "cleanup")

	require.Contains(t, output.String(), "| responses websocket observability | fields=")
	require.Contains(t, output.String(), `"event":"cleanup"`)
	require.Contains(t, output.String(), `"downstream_trace":"0123456789abcdef0123456789abcdef"`)
}
