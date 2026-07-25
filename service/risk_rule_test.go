package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestRiskRule_matches_normalized_text(t *testing.T) {
	tests := []struct {
		name       string
		ruleType   model.RiskRuleType
		pattern    string
		text       string
		normalized string
	}{
		{name: "phrase", ruleType: model.RiskRulePhrase, pattern: "ignore previous", text: "  ＩＧＮＯＲＥ   previous\ninstructions  ", normalized: "ignore previous instructions"},
		{name: "go regex", ruleType: model.RiskRuleRegex, pattern: `account\s+\d+`, text: "ACCOUNT  42", normalized: "account 42"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			input := RiskRuleTestInput{RuleType: test.ruleType, Pattern: test.pattern, Text: test.text}

			// When
			result, err := TestRiskRule(input)

			// Then
			require.NoError(t, err)
			require.True(t, result.Matched)
			require.Equal(t, test.normalized, result.NormalizedText)
		})
	}
}
