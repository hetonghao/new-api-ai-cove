package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type severeRiskRecordListQuery struct {
	StartTimestamp int64                        `form:"start_timestamp"`
	EndTimestamp   int64                        `form:"end_timestamp"`
	ChannelID      int                          `form:"channel_id"`
	UserID         int                          `form:"user_id"`
	Model          string                       `form:"model"`
	RequestID      string                       `form:"request_id"`
	ActionStatus   model.SevereRiskActionStatus `form:"action_status"`
}

func ListSevereRiskRecords(c *gin.Context) {
	var query severeRiskRecordListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		common.ApiErrorMsg(c, "无效的严重风险记录筛选条件")
		return
	}
	pageInfo := common.GetPageQuery(c)
	records, total, err := model.QuerySevereRiskRecords(c.Request.Context(), model.SevereRiskRecordQuery{
		Offset: pageInfo.GetStartIdx(), Limit: pageInfo.GetPageSize(), ChannelID: query.ChannelID,
		UserID: query.UserID, Model: query.Model, RequestID: query.RequestID, ActionStatus: query.ActionStatus,
		StartTimestamp: query.StartTimestamp, EndTimestamp: query.EndTimestamp,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(records)
	common.ApiSuccess(c, pageInfo)
}

func GetSevereRiskRecord(c *gin.Context) {
	id := c.Param("id")
	var recordID int
	var err error
	if recordID, err = strconv.Atoi(id); err != nil {
		common.ApiErrorMsg(c, "无效的严重风险记录 ID")
		return
	}
	record, err := model.GetSevereRiskRecord(c.Request.Context(), recordID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	contextSnapshot, err := common.DecryptCredential(record.ContextEncrypted)
	if err != nil {
		common.ApiErrorMsg(c, "严重风险上下文读取失败")
		return
	}
	model.RecordOperationAuditLog(c.GetInt("id"), "Viewed severe risk record", c.ClientIP(), "risk.severe_record_view", map[string]interface{}{
		"record_id": record.Id, "request_id": record.RequestID,
	}, map[string]interface{}{"admin_id": c.GetInt("id"), "admin_username": c.GetString("username"), "admin_role": c.GetInt("role")}, nil)
	common.ApiSuccess(c, gin.H{"record": record, "context": contextSnapshot})
}
