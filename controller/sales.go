package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

type updateSalesUserGroupRequest struct {
	Group string `json:"group"`
}

func GetSalesUsers(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	users, total, err := model.ListSalesInvitedUsers(
		c.GetInt("id"),
		c.Query("keyword"),
		c.Query("group"),
		pageInfo.GetStartIdx(),
		pageInfo.GetPageSize(),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(users)
	common.ApiSuccess(c, pageInfo)
}

func UpdateSalesUserGroup(c *gin.Context) {
	targetUserID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}

	var req updateSalesUserGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateSalesInvitedUserGroup(c.GetInt("id"), targetUserID, strings.TrimSpace(req.Group)); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

func GetSalesData(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	dates, err := model.GetSalesQuotaData(c.GetInt("id"), startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, dates)
}

func GetSalesDataByUser(c *gin.Context) {
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	dates, err := model.GetSalesQuotaDataGroupByUser(c.GetInt("id"), startTimestamp, endTimestamp)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, dates)
}

func GetSalesGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	common.ApiSuccess(c, groupNames)
}
