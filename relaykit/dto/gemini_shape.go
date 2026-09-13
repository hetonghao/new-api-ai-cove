package dto

import (
	"encoding/json"
	"fmt"
	"strings"
)

// GeminiRequestShape is a one-line, redacted outline of a Gemini generateContent body.
func GeminiRequestShape(req *GeminiChatRequest) string {
	if req == nil {
		return "nil"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "contents=%d tools=%t sys=%t", len(req.Contents), len(req.Tools) > 2, req.SystemInstructions != nil)
	for i, content := range req.Contents {
		role := content.Role
		if role == "" {
			role = "?"
		}
		nText, nThought, nFC, nFR, nInline := 0, 0, 0, 0, 0
		var sigLens []int
		var fcNames, frNames []string
		for _, part := range content.Parts {
			if part.Thought {
				nThought++
			} else if part.Text != "" {
				nText++
			}
			if part.FunctionCall != nil {
				nFC++
				fcNames = append(fcNames, part.FunctionCall.FunctionName)
			}
			if part.FunctionResponse != nil {
				nFR++
				frNames = append(frNames, part.FunctionResponse.Name)
			}
			if part.InlineData != nil {
				nInline++
			}
			if n := thoughtSignatureBytes(part.ThoughtSignature); n > 0 {
				sigLens = append(sigLens, n)
			}
		}
		fmt.Fprintf(&b, " c%d=%s(p=%d text=%d thought=%d fc=%d fr=%d inline=%d", i, role, len(content.Parts), nText, nThought, nFC, nFR, nInline)
		if len(sigLens) > 0 {
			fmt.Fprintf(&b, " sig=%s", joinInts(sigLens))
		}
		if len(fcNames) > 0 {
			fmt.Fprintf(&b, " fc_names=%s", strings.Join(fcNames, ","))
		}
		if len(frNames) > 0 {
			fmt.Fprintf(&b, " fr_names=%s", strings.Join(frNames, ","))
		}
		b.WriteByte(')')
	}
	return b.String()
}

func thoughtSignatureBytes(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" || s == `""` {
		return 0
	}
	var decoded string
	if err := json.Unmarshal(raw, &decoded); err == nil {
		return len(decoded)
	}
	return len(raw)
}

func joinInts(values []int) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = fmt.Sprintf("%d", v)
	}
	return strings.Join(parts, ",")
}
