package service

import (
	"fmt"
	"math"
	"unicode/utf8"
)

func EstimateCloudflareNeurons(content string) int64 {
	inputTokens := int64(math.Ceil(float64(utf8.RuneCountInString(content)) / 4))
	estimated := math.Ceil(float64(inputTokens)*44003/1_000_000 + 16*2730.0/1_000_000)
	return maxInt64(1, int64(estimated))
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
