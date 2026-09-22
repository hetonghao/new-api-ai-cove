package qualityinspect

import (
	"os"
	"testing"
)

func TestDebugUnsafe250(t *testing.T) {
	data, err := os.ReadFile("/tmp/unsafe250.txt")
	if err != nil {
		t.Skip("no artifact file")
	}
	svg, code := ExtractSafeSVG(string(data))
	t.Logf("code=%q svg_len=%d", code, len(svg))
}
