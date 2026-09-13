package service

import (
	"context"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func LogGeminiOutboundShape(ctx context.Context, converted any) {
	var req *dto.GeminiChatRequest
	switch v := converted.(type) {
	case *dto.GeminiChatRequest:
		req = v
	case dto.GeminiChatRequest:
		req = &v
	default:
		return
	}
	logger.LogError(ctx, "gemini outbound shape: "+dto.GeminiRequestShape(req))
}
