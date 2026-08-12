package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const riskRecordContentHMACDomain = "ai-cove:risk-record-content-hmac:v1"

type RiskRecordContentMetadata struct {
	Preview     string
	ContentHash string
}

func BuildRiskRecordContentMetadata(content string) RiskRecordContentMetadata {
	preview := common.MaskSensitiveInfo(strings.TrimSpace(content))

	normalized := NormalizeRiskText(content)
	if normalized == "" {
		return RiskRecordContentMetadata{Preview: preview}
	}
	key := common.HmacSha256Raw([]byte(riskRecordContentHMACDomain), []byte(common.CryptoSecret))
	return RiskRecordContentMetadata{
		Preview:     preview,
		ContentHash: common.GenerateHMACWithKey(key, normalized),
	}
}

func BuildRiskRecordChunkSummary(content string) string {
	summary := []rune(common.MaskSensitiveInfo(strings.TrimSpace(content)))
	if len(summary) > model.RiskRecordChunkSummaryMaxRunes {
		summary = summary[:model.RiskRecordChunkSummaryMaxRunes]
	}
	return string(summary)
}
