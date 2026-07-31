package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestRiskRule_returns_normalized_text(t *testing.T) {
	tests := []struct {
		name       string
		ruleType   model.RiskRuleType
		pattern    string
		text       string
		normalized string
	}{
		{name: "phrase", ruleType: model.RiskRulePhrase, pattern: "ignore previous", text: "  ＩＧＮＯＲＥ   previous\ninstructions  ", normalized: "ignore previous instructions"},
		{name: "go regex", ruleType: model.RiskRuleRegex, pattern: `account\s+\d+`, text: "account  42", normalized: "account 42"},
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

func TestRiskRule_regex_uses_original_text(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		text    string
		matched bool
	}{
		{
			name:    "preserves case",
			pattern: `^Calculate and respond with ONLY the number, nothing else\.$`,
			text:    "Calculate and respond with ONLY the number, nothing else.",
			matched: true,
		},
		{
			name:    "case remains significant",
			pattern: `^calculate and respond with only the number, nothing else\.$`,
			text:    "Calculate and respond with ONLY the number, nothing else.",
			matched: false,
		},
		{
			name:    "preserves internal whitespace",
			pattern: `^Calculate  and$`,
			text:    "Calculate  and",
			matched: true,
		},
		{
			name:    "internal whitespace remains significant",
			pattern: `^Calculate and$`,
			text:    "Calculate  and",
			matched: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := TestRiskRule(RiskRuleTestInput{
				RuleType: model.RiskRuleRegex,
				Pattern:  test.pattern,
				Text:     test.text,
			})

			require.NoError(t, err)
			require.Equal(t, test.matched, result.Matched)
		})
	}
}
