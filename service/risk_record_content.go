package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
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
