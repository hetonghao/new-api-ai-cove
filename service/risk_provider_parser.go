package service

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

var errInvalidPlatformInternalRiskVerdict = errors.New("invalid platform internal risk verdict")

type platformInternalRiskVerdict struct {
	Verdict    json.RawMessage `json:"verdict"`
	Categories json.RawMessage `json:"categories"`
}

func parsePlatformInternalRiskResult(content string) (RiskReviewResult, error) {
	var accepted *RiskReviewResult
	for _, candidate := range platformInternalRiskJSONObjects(content) {
		result, err := parsePlatformInternalRiskJSONObject(candidate)
		if err != nil {
			continue
		}
		if accepted != nil && (accepted.Status != result.Status || !slices.Equal(accepted.Categories, result.Categories)) {
			return RiskReviewResult{}, errInvalidPlatformInternalRiskVerdict
		}
		accepted = &result
	}
	if accepted == nil {
		return RiskReviewResult{}, errInvalidPlatformInternalRiskVerdict
	}
	return *accepted, nil
}

func parsePlatformInternalRiskJSONObject(candidate string) (RiskReviewResult, error) {
	var raw platformInternalRiskVerdict
	if err := common.UnmarshalJsonStr(candidate, &raw); err != nil || raw.Verdict == nil || raw.Categories == nil {
		return RiskReviewResult{}, errInvalidPlatformInternalRiskVerdict
	}
	var verdict string
	var categories []string
	if err := common.Unmarshal(raw.Verdict, &verdict); err != nil {
		return RiskReviewResult{}, errInvalidPlatformInternalRiskVerdict
	}
	if err := common.Unmarshal(raw.Categories, &categories); err != nil || categories == nil {
		return RiskReviewResult{}, errInvalidPlatformInternalRiskVerdict
	}
	for index, category := range categories {
		category = strings.TrimSpace(category)
		if category == "" {
			return RiskReviewResult{}, errInvalidPlatformInternalRiskVerdict
		}
		categories[index] = category
	}
	switch verdict {
	case string(RiskReviewSafe):
		return RiskReviewResult{Status: RiskReviewSafe, Categories: categories}, nil
	case string(RiskReviewUnsafe):
		return RiskReviewResult{Status: RiskReviewUnsafe, Categories: categories}, nil
	default:
		return RiskReviewResult{}, errInvalidPlatformInternalRiskVerdict
	}
}

func platformInternalRiskJSONObjects(content string) []string {
	objects := make([]string, 0, 1)
	start := -1
	depth := 0
	arrayDepth := 0
	inString := false
	escaped := false
	for index := 0; index < len(content); index++ {
		character := content[index]
		if depth == 0 {
			switch character {
			case '[':
				arrayDepth++
			case ']':
				if arrayDepth > 0 {
					arrayDepth--
				}
			case '{':
				if arrayDepth > 0 {
					continue
				}
				start = index
				depth = 1
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == '"' {
				inString = false
			}
			continue
		}
		switch character {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				objects = append(objects, content[start:index+1])
				start = -1
			}
		}
	}
	return objects
}
