package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	claudechannel "github.com/QuantumNous/new-api/relay/channel/claude"
	codexchannel "github.com/QuantumNous/new-api/relay/channel/codex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier: "base",
	})

	require.Equal(t, "tiered_expr", other["billing_mode"])
	require.Equal(t, "base", other["matched_tier"])
	require.NotEmpty(t, other["expr_b64"])
}

func TestNormalizeAutomaticChannelTestUsageForBillingStripsCacheTokens(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:         223,
		CompletionTokens:     13,
		TotalTokens:          5100,
		InputTokens:          223,
		PromptCacheHitTokens: 4864,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         4864,
			CachedCreationTokens: 128,
			TextTokens:           223,
		},
		InputTokensDetails: &dto.InputTokenDetails{
			CachedTokens:         4864,
			CachedCreationTokens: 128,
			TextTokens:           223,
		},
		ClaudeCacheCreation5mTokens: 64,
		ClaudeCacheCreation1hTokens: 32,
	}

	normalized := normalizeAutomaticChannelTestUsageForBilling(usage, 3, true)

	require.NotSame(t, usage, normalized)
	require.Equal(t, 3, normalized.PromptTokens)
	require.Equal(t, 3, normalized.InputTokens)
	require.Equal(t, 13, normalized.CompletionTokens)
	require.Equal(t, 16, normalized.TotalTokens)
	require.Zero(t, normalized.PromptCacheHitTokens)
	require.Zero(t, normalized.PromptTokensDetails.CachedTokens)
	require.Zero(t, normalized.PromptTokensDetails.CachedCreationTokens)
	require.NotNil(t, normalized.InputTokensDetails)
	require.Zero(t, normalized.InputTokensDetails.CachedTokens)
	require.Zero(t, normalized.InputTokensDetails.CachedCreationTokens)
	require.Zero(t, normalized.ClaudeCacheCreation5mTokens)
	require.Zero(t, normalized.ClaudeCacheCreation1hTokens)

	require.Equal(t, 4864, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 4864, usage.InputTokensDetails.CachedTokens)
}

func TestNormalizeAutomaticChannelTestUsageForBillingUsesZeroEstimate(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     223,
		CompletionTokens: 13,
		TotalTokens:      5100,
		InputTokens:      223,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 4864,
			TextTokens:   223,
		},
		InputTokensDetails: &dto.InputTokenDetails{
			CachedTokens: 4864,
			TextTokens:   223,
		},
	}

	normalized := normalizeAutomaticChannelTestUsageForBilling(usage, 0, true)

	require.Equal(t, 0, normalized.PromptTokens)
	require.Equal(t, 0, normalized.InputTokens)
	require.Equal(t, 13, normalized.CompletionTokens)
	require.Equal(t, 13, normalized.TotalTokens)
	require.Zero(t, normalized.PromptTokensDetails.TextTokens)
	require.NotNil(t, normalized.InputTokensDetails)
	require.Zero(t, normalized.InputTokensDetails.TextTokens)
}

func TestResolveChannelTestUserIDUsesRequestUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("id", 2)

	userID, err := resolveChannelTestUserID(ctx)

	require.NoError(t, err)
	require.Equal(t, 2, userID)
}

func TestResolveChannelTestModelAutomaticChoosesLowestCostModel(t *testing.T) {
	ratio_setting.InitRatioSettings()
	channel := &model.Channel{
		Models: "claude-opus-4-7,gpt-5-mini,gpt-5-nano",
	}

	require.Equal(t, "gpt-5-nano", resolveChannelTestModel(channel, "", true))
}

func TestResolveChannelTestModelKeepsExplicitTestModel(t *testing.T) {
	testModel := "claude-opus-4-7"
	channel := &model.Channel{
		TestModel: &testModel,
		Models:    "gpt-5-nano,claude-opus-4-7",
	}

	require.Equal(t, "claude-opus-4-7", resolveChannelTestModel(channel, "", true))
	require.Equal(t, "gpt-5-mini", resolveChannelTestModel(channel, "gpt-5-mini", true))
}

func TestResolveChannelTestModelManualKeepsFirstChannelModel(t *testing.T) {
	channel := &model.Channel{
		Models: "claude-opus-4-7,gpt-5-nano",
	}

	require.Equal(t, "claude-opus-4-7", resolveChannelTestModel(channel, "", false))
}

func TestBuildAutomaticChannelTestRequestCapsOutputTokens(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI}

	req := buildTestRequest("o3-mini", "", channel, false, true)
	chatReq, ok := req.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, chatReq.MaxCompletionTokens)
	require.EqualValues(t, 1, *chatReq.MaxCompletionTokens)

	geminiReq := buildTestRequest("gemini-2.5-flash", string(constant.EndpointTypeGemini), channel, false, true)
	geminiChatReq, ok := geminiReq.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.NotNil(t, geminiChatReq.MaxTokens)
	require.EqualValues(t, 1, *geminiChatReq.MaxTokens)

	responsesReq := buildTestRequest("gpt-5-mini", string(constant.EndpointTypeOpenAIResponse), channel, false, true)
	oaiResponsesReq, ok := responsesReq.(*dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.NotNil(t, oaiResponsesReq.MaxOutputTokens)
	require.EqualValues(t, 1, *oaiResponsesReq.MaxOutputTokens)
}

func TestBuildAutomaticChannelTestRequestAvoidsClaudeThinkingBudget(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeAnthropic}

	req := buildTestRequest("claude-sonnet-4-20250514-thinking", "", channel, false, true)
	chatReq, ok := req.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, "claude-sonnet-4-20250514", chatReq.Model)
	require.NotNil(t, chatReq.MaxTokens)
	require.EqualValues(t, 1, *chatReq.MaxTokens)

	claudeReq, err := claudechannel.RequestOpenAI2ClaudeMessage(nil, *chatReq)
	require.NoError(t, err)
	require.NotNil(t, claudeReq.MaxTokens)
	require.EqualValues(t, 1, *claudeReq.MaxTokens)
	require.Nil(t, claudeReq.Thinking)
}

func TestBuildAutomaticCodexChannelTestUsesCompactRequestWithOutputLimit(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeCodex}

	req := buildTestRequest("gpt-5-codex", "", channel, false, true)
	compactReq, ok := req.(*dto.OpenAIResponsesCompactionRequest)
	require.True(t, ok)

	jsonBytes, err := json.Marshal(req)
	require.NoError(t, err)
	require.Contains(t, string(jsonBytes), `"max_output_tokens":1`)

	converted, err := (&codexchannel.Adaptor{}).ConvertOpenAIResponsesRequest(nil, &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponsesCompact,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}, dto.OpenAIResponsesRequest{
		Model:           compactReq.Model,
		Input:           compactReq.Input,
		MaxOutputTokens: compactReq.MaxOutputTokens,
	})
	require.NoError(t, err)
	convertedReq, ok := converted.(dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.NotNil(t, convertedReq.MaxOutputTokens)
	require.EqualValues(t, 1, *convertedReq.MaxOutputTokens)
}
