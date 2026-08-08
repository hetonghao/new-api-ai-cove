package middleware

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

func ResponsesWebSocketPreflight() gin.HandlerFunc {
	return func(c *gin.Context) {
		if channelIDValue, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); ok {
			channelID, err := strconv.Atoi(fmt.Sprint(channelIDValue))
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "指定渠道无效", types.ErrorCodeGetChannelFailed)
				return
			}
			channel, err := model.GetChannelById(channelID, true)
			if err != nil {
				abortWithOpenAiMessage(c, http.StatusBadRequest, "指定渠道无效", types.ErrorCodeGetChannelFailed)
				return
			}
			if !model.ChannelSupportsResponsesWebSocket(channel) {
				abortWithOpenAiMessage(c, http.StatusUpgradeRequired, "指定渠道未启用非语音 Responses WebSocket", types.ErrorCode("websocket_not_supported"))
				return
			}
			c.Next()
			return
		}

		hasChannel, err := model.HasEnabledResponsesWebSocketChannel()
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "检查 WebSocket 渠道能力失败", types.ErrorCodeGetChannelFailed)
			return
		}
		if !hasChannel {
			abortWithOpenAiMessage(c, http.StatusUpgradeRequired, "当前没有启用非语音 Responses WebSocket 渠道", types.ErrorCode("websocket_not_supported"))
			return
		}
		c.Next()
	}
}
