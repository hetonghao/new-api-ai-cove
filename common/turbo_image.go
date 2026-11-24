package common

import (
	"github.com/gin-gonic/gin"
	"strconv"
	"strings"
)

const (
	TurboImageHeader      = "X-AI-Cove-Image-Processing"
	TurboImageMetadata    = "x-ai-cove-image-processing"
	TurboImageChanged     = "v1/changed/"
	TurboWireZstdKey      = "turbo_wire_zstd"
	TurboImageCountKey    = "turbo_image_count"
	TurboImageKept        = "v1/kept"
	turboImageDecisionKey = "turbo_image_processing"
)

// SetTurboImageDecision records provenance for one logical request, not the connection or an auth grant.
func SetTurboImageDecision(c *gin.Context, values []string) {
	value := ""
	count := 0
	if len(values) == 1 && c.GetHeader("X-AI-Cove-Client") == "turbo" {
		if values[0] == TurboImageKept {
			value = TurboImageKept
		} else if raw, ok := strings.CutPrefix(values[0], TurboImageChanged); ok {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 256 && strconv.Itoa(n) == raw {
				value = TurboImageChanged
				count = n
			}
		}
	}
	c.Set(TurboImageCountKey, count)
	c.Set(turboImageDecisionKey, value)
}

func TurboImageDecision(c *gin.Context) string { return c.GetString(turboImageDecisionKey) }
