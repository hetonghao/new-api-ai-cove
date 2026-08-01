package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type riskRecordListQuery struct {
	StartTimestamp int64                  `form:"start_timestamp"`
	EndTimestamp   int64                  `form:"end_timestamp"`
	ChannelID      int                    `form:"channel_id"`
	UserID         int                    `form:"user_id"`
	Username       string                 `form:"username"`
	ProviderID     *int                   `form:"provider_id"`
	ProviderType   model.RiskProviderType `form:"provider_type"`
	Result         model.RiskRecordResult `form:"result"`
	Source         model.RiskRecordSource `form:"source"`
}

func ListRiskRecords(c *gin.Context) {
	var query riskRecordListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		common.ApiErrorMsg(c, "无效的风控记录筛选条件")
		return
	}
	pageInfo := common.GetPageQuery(c)
	records, total, err := model.QueryRiskRecords(c.Request.Context(), model.RiskRecordQuery{
		Offset: pageInfo.GetStartIdx(), Limit: pageInfo.GetPageSize(),
		StartTimestamp: query.StartTimestamp, EndTimestamp: query.EndTimestamp,
		ChannelID: query.ChannelID, UserID: query.UserID, Username: query.Username, ProviderID: query.ProviderID,
		ProviderType: query.ProviderType,
		Result:       query.Result, Source: query.Source,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(records)
	common.ApiSuccess(c, pageInfo)
}
