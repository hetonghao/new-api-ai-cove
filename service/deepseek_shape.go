package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func LogDeepSeekInputShape(ctx context.Context, converted any) {
	var req *dto.OpenAIResponsesRequest
	switch v := converted.(type) {
	case *dto.OpenAIResponsesRequest:
		req = v
	case dto.OpenAIResponsesRequest:
		req = &v
	default:
		return
	}
	if req == nil || !strings.HasPrefix(req.Model, "deepseek") {
		return
	}
	logger.LogError(ctx, "deepseek input shape: "+deepSeekInputShape(req.Input))
}

func deepSeekInputShape(input json.RawMessage) string {
	if len(input) == 0 {
		return "items=0"
	}
	var items []map[string]any
	if json.Unmarshal(input, &items) != nil {
		return fmt.Sprintf("input=non-array len=%d", len(input))
	}
	nReason, nText, nEmpty, nFC, nFCO := 0, 0, 0, 0, 0
	types := make([]string, 0, len(items))
	for _, item := range items {
		t, _ := item["type"].(string)
		if t == "" {
			t = "?"
		}
		types = append(types, t)
		switch t {
		case "reasoning":
			nReason++
			if reasoningMapHasText(item) {
				nText++
			} else {
				nEmpty++
			}
		case "function_call", "custom_tool_call":
			nFC++
		case "function_call_output", "custom_tool_call_output":
			nFCO++
		}
	}
	last := types
	if len(last) > 8 {
		last = last[len(last)-8:]
	}
	return fmt.Sprintf("items=%d reasoning=%d text=%d empty=%d fc=%d fco=%d last=%s", len(items), nReason, nText, nEmpty, nFC, nFCO, strings.Join(last, ","))
}

func reasoningMapHasText(item map[string]any) bool {
	content, _ := item["content"].([]any)
	for _, part := range content {
		fields, _ := part.(map[string]any)
		if t, _ := fields["type"].(string); t != "reasoning_text" {
			continue
		}
		if text, _ := fields["text"].(string); strings.TrimSpace(text) != "" {
			return true
		}
	}
	return false
}
