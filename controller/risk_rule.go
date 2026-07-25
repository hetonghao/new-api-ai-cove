package controller

import (
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type riskRuleRequest struct {
	RuleType model.RiskRuleType `json:"rule_type" binding:"required,oneof=keyword phrase regex"`
	Pattern  string             `json:"pattern" binding:"required"`
	Enabled  *bool              `json:"enabled"`
}

type riskRuleTestRequest struct {
	RuleType model.RiskRuleType `json:"rule_type" binding:"required,oneof=keyword phrase regex"`
	Pattern  string             `json:"pattern" binding:"required"`
	Text     string             `json:"text" binding:"required"`
}

func ListRiskRules(c *gin.Context) {
	rules, err := model.GetRiskRules()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rules)
}

func CreateRiskRule(c *gin.Context) {
	var request riskRuleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "无效的本地风控规则")
		return
	}
	enabled := true
	if request.Enabled != nil {
		enabled = *request.Enabled
	}
	rule, err := model.CreateRiskRule(model.RiskRuleInput{RuleType: request.RuleType, Pattern: request.Pattern, Enabled: enabled})
	if err != nil {
		writeRiskRuleError(c, err)
		return
	}
	common.ApiSuccess(c, rule)
}

func UpdateRiskRule(c *gin.Context) {
	id, ok := parseRiskRuleID(c)
	if !ok {
		return
	}
	var request riskRuleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "无效的本地风控规则")
		return
	}
	rule, err := model.UpdateRiskRule(id, model.RiskRuleUpdateInput{RuleType: request.RuleType, Pattern: request.Pattern, Enabled: request.Enabled})
	if err != nil {
		writeRiskRuleError(c, err)
		return
	}
	common.ApiSuccess(c, rule)
}

func DeleteRiskRule(c *gin.Context) {
	id, ok := parseRiskRuleID(c)
	if !ok {
		return
	}
	if err := model.DeleteRiskRule(id); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func TestRiskRule(c *gin.Context) {
	var request riskRuleTestRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "无效的本地风控规则测试")
		return
	}
	result, err := service.TestRiskRule(service.RiskRuleTestInput{RuleType: request.RuleType, Pattern: request.Pattern, Text: request.Text})
	if err != nil {
		writeRiskRuleError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}

func parseRiskRuleID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		common.ApiErrorMsg(c, "无效的本地风控规则 ID")
		return 0, false
	}
	return id, true
}

func writeRiskRuleError(c *gin.Context, err error) {
	if errors.Is(err, model.ErrInvalidRiskRulePattern) {
		common.ApiErrorMsg(c, "无效的本地风控规则")
		return
	}
	common.ApiError(c, err)
}
