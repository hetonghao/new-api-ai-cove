package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/model"
	"golang.org/x/text/unicode/norm"
)

type RiskRuleTestInput struct {
	RuleType model.RiskRuleType
	Pattern  string
	Text     string
	Action   model.RiskRuleAction
}

type RiskRuleTestResult struct {
	NormalizedText string               `json:"normalized_text"`
	Matched        bool                 `json:"matched"`
	Action         model.RiskRuleAction `json:"action"`
}

func NormalizeRiskText(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(norm.NFKC.String(text))), " ")
}

func TestRiskRule(input RiskRuleTestInput) (RiskRuleTestResult, error) {
	if err := model.ValidateRiskRule(input.RuleType, input.Pattern); err != nil {
		return RiskRuleTestResult{}, err
	}
	if input.Action == "" {
		input.Action = model.RiskRuleActionReview
	}
	if err := model.ValidateRiskRuleAction(input.Action); err != nil {
		return RiskRuleTestResult{}, err
	}
	normalizedText := NormalizeRiskText(input.Text)
	result := RiskRuleTestResult{NormalizedText: normalizedText, Action: input.Action}
	switch input.RuleType {
	case model.RiskRuleKeyword, model.RiskRulePhrase:
		result.Matched = strings.Contains(normalizedText, NormalizeRiskText(input.Pattern))
	case model.RiskRuleRegex:
		compiled, err := regexp.Compile(strings.TrimSpace(input.Pattern))
		if err != nil {
			return RiskRuleTestResult{}, fmt.Errorf("compile risk rule regex: %w", err)
		}
		result.Matched = compiled.MatchString(input.Text)
	}
	return result, nil
}
