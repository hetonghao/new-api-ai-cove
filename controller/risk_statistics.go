package controller

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type riskStatisticsQuery struct {
	StartTimestamp int64                           `form:"start_timestamp"`
	EndTimestamp   int64                           `form:"end_timestamp"`
	Granularity    model.RiskStatisticsGranularity `form:"granularity"`
}

func GetRiskStatistics(c *gin.Context) {
	var query riskStatisticsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		common.ApiErrorMsg(c, "无效的统计分析筛选条件")
		return
	}
	statistics, err := model.QueryRiskStatistics(c.Request.Context(), model.RiskStatisticsQuery{
		StartTimestamp: query.StartTimestamp,
		EndTimestamp:   query.EndTimestamp,
		Granularity:    query.Granularity,
	})
	if err != nil {
		if errors.Is(err, model.ErrInvalidRiskStatisticsQuery) {
			common.ApiErrorMsg(c, "统计时间范围或粒度无效")
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, statistics)
}
