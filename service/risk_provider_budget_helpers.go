package service

import (
	"fmt"
	"math"
)

const (
	riskProviderNeuronsPromptOverheadTokens = 128
	riskProviderNeuronsMaxCompletionTokens  = 16
)

func NormalizeRiskProviderNeurons(value float64) int64 {
	if math.IsNaN(value) || math.IsInf(value, -1) || value <= 0 {
		return 0
	}
	rounded := math.Round(value)
	maxNeurons := int64(^uint64(0) >> 1)
	if math.IsInf(rounded, 1) || rounded >= float64(maxNeurons) {
		return maxNeurons
	}
	return int64(rounded)
}

func EstimateCloudflareNeurons(content string) int64 {
	inputTokens := float64(len(content) + riskProviderNeuronsPromptOverheadTokens)
	estimated := math.Ceil(inputTokens*44003/1_000_000 + float64(riskProviderNeuronsMaxCompletionTokens)*2730.0/1_000_000)
	return maxInt64(1, NormalizeRiskProviderNeurons(estimated))
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func parseInt64(value string) int64 {
	var result int64
	if _, err := fmt.Sscan(value, &result); err != nil {
		return 0
	}
	return result
}
