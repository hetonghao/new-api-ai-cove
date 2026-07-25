package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func RelayWithRiskObservation(format types.RelayFormat) gin.HandlerFunc {
	return relayWithRiskObservation(format, Relay, service.EnqueueRiskObservation)
}

func relayWithRiskObservation(
	format types.RelayFormat,
	relay func(*gin.Context, types.RelayFormat),
	enqueue func(service.RiskObservationJob) bool,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		request, err := helper.GetAndValidateRequest(c, format)
		if err == nil {
			text := service.ExtractRiskObservationText(request)
			if text != "" {
				enqueue(service.RiskObservationJob{
					RequestID:   c.GetString(common.RequestIdKey),
					ChannelID:   c.GetInt("channel_id"),
					ChannelName: c.GetString("channel_name"),
					UserID:      c.GetInt("id"),
					Text:        text,
				})
			}
		}
		relay(c, format)
	}
}
