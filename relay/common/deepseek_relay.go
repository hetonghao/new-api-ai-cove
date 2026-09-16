package common

import (
	"strings"

	"github.com/QuantumNous/new-api/constant"
)

// IsDeepSeekReasoningRelay reports whether this relay drives a DeepSeek
// thinking-mode upstream: the only path whose /responses item contract needs
// canonicalization and whose rejected payload is worth dumping.
func IsDeepSeekReasoningRelay(info *RelayInfo, model string) bool {
	if info != nil && info.ChannelMeta != nil && info.ChannelType == constant.ChannelTypeDeepSeek {
		return true
	}
	if info != nil && strings.HasPrefix(info.OriginModelName, "deepseek") {
		return true
	}
	return strings.HasPrefix(model, "deepseek")
}
