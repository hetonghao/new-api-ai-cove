package service

import (
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var riskContentURLPattern = regexp.MustCompile(`(?:https?|wss?)://[^\s"<>{}]+`)

const riskRecordContentHMACDomain = "ai-cove:risk-record-content-hmac:v1"

type RiskRecordContentMetadata struct {
	Preview     string
	ContentHash string
}

func BuildRiskRecordContentMetadata(content string) RiskRecordContentMetadata {
	preview := common.MaskSensitiveInfo(strings.TrimSpace(content), riskContentURLPattern.FindAllString(content, -1)...)

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
	summary := []rune(common.MaskSensitiveInfo(strings.TrimSpace(content), riskContentURLPattern.FindAllString(content, -1)...))
	if len(summary) > model.RiskRecordChunkSummaryMaxRunes {
		summary = summary[:model.RiskRecordChunkSummaryMaxRunes]
	}
	return string(summary)
}
