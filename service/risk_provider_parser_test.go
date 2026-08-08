package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePlatformInternalRiskResultNormalizesUppercaseVerdict(t *testing.T) {
	// Given
	content := `{"verdict":"SAFE","categories":[]}`

	// When
	result, err := parsePlatformInternalRiskResult(content)

	// Then
	require.NoError(t, err)
	assert.Equal(t, RiskReviewSafe, result.Status)
}

func TestParsePlatformInternalRiskResultNormalizesPaddedVerdict(t *testing.T) {
	// Given
	content := `{"verdict":" safe ","categories":[]}`

	// When
	result, err := parsePlatformInternalRiskResult(content)

	// Then
	require.NoError(t, err)
	assert.Equal(t, RiskReviewSafe, result.Status)
}

func TestParsePlatformInternalRiskResultAcceptsFencedJSON(t *testing.T) {
	// Given
	content := "```json\n{\"verdict\":\"unsafe\",\"categories\":[\"S1\"]}\n```"

	// When
	result, err := parsePlatformInternalRiskResult(content)

	// Then
	require.NoError(t, err)
	assert.Equal(t, RiskReviewUnsafe, result.Status)
	assert.Equal(t, []string{"S1"}, result.Categories)
}

func TestParsePlatformInternalRiskResultAcceptsRecoverableObjects(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantStatus RiskReviewStatus
		want       []string
	}{
		{name: "pure JSON", content: `{"verdict":"safe","categories":[]}`, wantStatus: RiskReviewSafe, want: []string{}},
		{name: "prose wrapped", content: `Analysis complete. {"verdict":"unsafe","categories":[" S1 "]} End.`, wantStatus: RiskReviewUnsafe, want: []string{"S1"}},
		{name: "extra fields", content: `{"verdict":"safe","categories":[],"reason":"short","score":0.9}`, wantStatus: RiskReviewSafe, want: []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			result, err := parsePlatformInternalRiskResult(test.content)

			// Then
			require.NoError(t, err)
			assert.Equal(t, test.wantStatus, result.Status)
			assert.Equal(t, test.want, result.Categories)
		})
	}
}

func TestParsePlatformInternalRiskResultRejectsInvalidObjects(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{name: "empty", content: ""},
		{name: "no JSON", content: "safe"},
		{name: "array wrapped", content: `[{"verdict":"safe","categories":[]}]`},
		{name: "missing categories", content: `{"verdict":"safe"}`},
		{name: "categories null", content: `{"verdict":"safe","categories":null}`},
		{name: "categories contain non string", content: `{"verdict":"unsafe","categories":["S1",2]}`},
		{name: "conflicting conclusions", content: `{"verdict":"safe","categories":[]} then {"verdict":"unsafe","categories":["S1"]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			_, err := parsePlatformInternalRiskResult(test.content)

			// Then
			require.ErrorIs(t, err, errInvalidPlatformInternalRiskVerdict)
		})
	}
}
