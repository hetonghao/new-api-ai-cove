package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

func TestRiskRuleAPI_creates_enabled_rule_by_default(t *testing.T) {
	// Given
	setupRiskPolicyControllerTest(t)

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodPost, Target: "/api/risk/rules", Body: map[string]any{
		"rule_type": "keyword", "pattern": "  Example  ",
	}, Handler: CreateRiskRule})

	// Then
	require.True(t, response.Success, response.Message)
	var rule model.RiskRule
	require.NoError(t, common.Unmarshal(response.Data, &rule))
	require.Equal(t, "Example", rule.Pattern)
	require.True(t, rule.Enabled)
	require.Equal(t, model.RiskRuleActionReview, rule.Action)
}

func TestRiskRuleAPI_updates_enabled_state(t *testing.T) {
	// Given
	setupRiskPolicyControllerTest(t)
	created, err := model.CreateRiskRule(model.RiskRuleInput{RuleType: model.RiskRuleKeyword, Pattern: "example", Enabled: true})
	require.NoError(t, err)

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodPut, Target: "/api/risk/rules/1", Id: created.Id, Body: map[string]any{
		"rule_type": "phrase", "pattern": "example phrase", "enabled": false, "action": "skip",
	}, Handler: UpdateRiskRule})

	// Then
	require.True(t, response.Success, response.Message)
	var rule model.RiskRule
	require.NoError(t, common.Unmarshal(response.Data, &rule))
	require.Equal(t, model.RiskRulePhrase, rule.RuleType)
	require.False(t, rule.Enabled)
	require.Equal(t, model.RiskRuleActionSkip, rule.Action)
}

func TestRiskRuleAPI_tests_normalized_text(t *testing.T) {
	// Given
	setupRiskPolicyControllerTest(t)

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodPost, Target: "/api/risk/rules/test", Body: map[string]any{
		"rule_type": "phrase", "pattern": "ignore previous", "text": "  ＩＧＮＯＲＥ   previous instructions  ", "action": "skip",
	}, Handler: TestRiskRule})

	// Then
	require.True(t, response.Success, response.Message)
	var result service.RiskRuleTestResult
	require.NoError(t, common.Unmarshal(response.Data, &result))
	require.True(t, result.Matched)
	require.Equal(t, "ignore previous instructions", result.NormalizedText)
	require.Equal(t, model.RiskRuleActionSkip, result.Action)
}

func TestRiskRuleAPI_tests_regex_against_original_text(t *testing.T) {
	// Given
	setupRiskPolicyControllerTest(t)

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodPost, Target: "/api/risk/rules/test", Body: map[string]any{
		"rule_type": "regex",
		"pattern":   `^Calculate and respond with ONLY the number, nothing else\.$`,
		"text":      "Calculate and respond with ONLY the number, nothing else.",
	}, Handler: TestRiskRule})

	// Then
	require.True(t, response.Success, response.Message)
	var result service.RiskRuleTestResult
	require.NoError(t, common.Unmarshal(response.Data, &result))
	require.True(t, result.Matched)
}

func TestRiskRuleAPI_rejects_invalid_regex(t *testing.T) {
	// Given
	setupRiskPolicyControllerTest(t)

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodPost, Target: "/api/risk/rules", Body: map[string]any{
		"rule_type": "regex", "pattern": "[",
	}, Handler: CreateRiskRule})

	// Then
	require.False(t, response.Success)
}

func TestRiskRuleAPI_rejects_invalid_action(t *testing.T) {
	// Given
	setupRiskPolicyControllerTest(t)

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodPost, Target: "/api/risk/rules", Body: map[string]any{
		"rule_type": "keyword", "pattern": "example", "action": "block",
	}, Handler: CreateRiskRule})

	// Then
	require.False(t, response.Success)
}

func TestRiskRuleAPI_lists_rules_by_id(t *testing.T) {
	// Given
	setupRiskPolicyControllerTest(t)
	_, err := model.CreateRiskRule(model.RiskRuleInput{RuleType: model.RiskRuleKeyword, Pattern: "first", Enabled: true})
	require.NoError(t, err)
	_, err = model.CreateRiskRule(model.RiskRuleInput{
		RuleType: model.RiskRulePhrase, Pattern: "second rule", Enabled: false, Action: model.RiskRuleActionSkip,
	})
	require.NoError(t, err)

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodGet, Target: "/api/risk/rules", Handler: ListRiskRules})

	// Then
	require.True(t, response.Success, response.Message)
	var rules []model.RiskRule
	require.NoError(t, common.Unmarshal(response.Data, &rules))
	require.Len(t, rules, 2)
	require.Less(t, rules[0].Id, rules[1].Id)
	require.Equal(t, model.RiskRuleActionSkip, rules[1].Action)
}

func TestRiskRuleAPI_excludes_deleted_rule_from_list(t *testing.T) {
	// Given
	setupRiskPolicyControllerTest(t)
	created, err := model.CreateRiskRule(model.RiskRuleInput{RuleType: model.RiskRuleKeyword, Pattern: "delete me", Enabled: true})
	require.NoError(t, err)
	deleted := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodDelete, Target: "/api/risk/rules/1", Id: created.Id, Handler: DeleteRiskRule})
	require.True(t, deleted.Success, deleted.Message)

	// When
	response := callRiskProviderHandler(t, riskProviderTestCall{Method: http.MethodGet, Target: "/api/risk/rules", Handler: ListRiskRules})

	// Then
	require.True(t, response.Success, response.Message)
	var rules []model.RiskRule
	require.NoError(t, common.Unmarshal(response.Data, &rules))
	require.Empty(t, rules)
}
