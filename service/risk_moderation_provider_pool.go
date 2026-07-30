package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
)

const (
	riskModerationPromptSemantics         = "cloudflare-user-message-max16-temp0-v1"
	riskModerationClassificationSemantics = "safe-unsafe-error-unsafe-first-v1"
	riskModerationRoundRobinNamespace     = "new-api:risk-moderation-round-robin:v1"
)

var advanceRiskModerationProviderCursorScript = redis.NewScript(`
local current = redis.call("GET", KEYS[1])
if current == false then
  current = "0"
end
if tonumber(current) ~= tonumber(ARGV[1]) then
  return 0
end
redis.call("INCR", KEYS[1])
return 1
`)

type riskModerationProviderCursor struct {
	key    string
	value  int64
	shared bool
}

func RiskModerationPolicyVersion(input RiskModerationInput) (string, error) {
	if len(input.Providers) > 0 {
		if input.Provider != nil {
			return "", ErrInvalidRiskModerationInput
		}
		versions := make([]string, 0, len(input.Providers))
		for _, provider := range input.Providers {
			version, err := RiskModerationPolicyVersion(RiskModerationInput{
				Provider: provider, ReviewMode: input.ReviewMode, FullReviewChunkRunes: input.FullReviewChunkRunes,
			})
			if err != nil {
				return "", err
			}
			versions = append(versions, version)
		}
		material, err := common.Marshal(struct {
			ProviderVersions []string `json:"provider_versions"`
		}{ProviderVersions: versions})
		if err != nil {
			return "", fmt.Errorf("encode risk moderation provider pool policy: %w", err)
		}
		return hex.EncodeToString(common.Sha256Raw(material)), nil
	}
	if input.Provider == nil {
		return "", ErrInvalidRiskModerationInput
	}
	chunkLimit, err := riskModerationChunkLimit(input)
	if err != nil {
		return "", err
	}
	provider := input.Provider
	accountID := ""
	channelID := 0
	promptSemantics := riskModerationPromptSemantics
	switch provider.ProviderType {
	case model.RiskProviderCloudflare:
		accountID, err = provider.CloudflareAccountID()
		if err != nil {
			return "", fmt.Errorf("resolve risk provider account ID: %w", err)
		}
	case model.RiskProviderPlatformInternal:
		channelID = provider.ChannelID
		promptSemantics = platformInternalRiskPromptSemantics
	default:
		return "", fmt.Errorf("%w: provider type", ErrInvalidRiskModerationInput)
	}
	material, err := common.Marshal(struct {
		ProviderID              int                    `json:"provider_id"`
		ProviderType            model.RiskProviderType `json:"provider_type"`
		AccountID               string                 `json:"account_id"`
		ChannelID               int                    `json:"channel_id"`
		Model                   string                 `json:"model"`
		ReviewMode              model.RiskReviewMode   `json:"review_mode"`
		ChunkLimit              int                    `json:"chunk_limit"`
		PromptSemantics         string                 `json:"prompt_semantics"`
		ClassificationSemantics string                 `json:"classification_semantics"`
	}{
		ProviderID: provider.Id, ProviderType: provider.ProviderType,
		AccountID: accountID, ChannelID: channelID, Model: provider.Model,
		ReviewMode: input.ReviewMode, ChunkLimit: chunkLimit,
		PromptSemantics:         promptSemantics,
		ClassificationSemantics: riskModerationClassificationSemantics,
	})
	if err != nil {
		return "", fmt.Errorf("encode risk moderation policy: %w", err)
	}
	return hex.EncodeToString(common.Sha256Raw(material)), nil
}

func riskModerationProviders(input RiskModerationInput) ([]*model.RiskProvider, error) {
	if len(input.Providers) > 0 {
		if input.Provider != nil {
			return nil, ErrInvalidRiskModerationInput
		}
		for _, provider := range input.Providers {
			if provider == nil {
				return nil, ErrInvalidRiskModerationInput
			}
		}
		return input.Providers, nil
	}
	if input.Provider == nil {
		return nil, ErrInvalidRiskModerationInput
	}
	return []*model.RiskProvider{input.Provider}, nil
}

func loadRiskModerationProviderCursor(ctx context.Context, policyVersion string, size int) (riskModerationProviderCursor, error) {
	if size < 2 || !common.RedisEnabled || common.RDB == nil {
		return riskModerationProviderCursor{}, nil
	}
	key := riskModerationRoundRobinNamespace + ":" + policyVersion
	value, err := common.RDB.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return riskModerationProviderCursor{key: key, shared: true}, nil
	}
	if err != nil || value < 0 {
		return riskModerationProviderCursor{}, err
	}
	return riskModerationProviderCursor{key: key, value: value, shared: true}, nil
}

func (cursor riskModerationProviderCursor) index(size int) int {
	if !cursor.shared || size < 2 {
		return 0
	}
	return int(cursor.value % int64(size))
}

func (cursor riskModerationProviderCursor) advance(ctx context.Context) (bool, error) {
	if !cursor.shared {
		return true, nil
	}
	advanced, err := advanceRiskModerationProviderCursorScript.Run(
		ctx, common.RDB, []string{cursor.key}, cursor.value,
	).Int()
	if err != nil {
		return false, err
	}
	return advanced == 1, nil
}

func riskReviewResultWithProvider(result RiskReviewResult, provider *model.RiskProvider) RiskReviewResult {
	result.ProviderID = provider.Id
	result.ProviderName = provider.Name
	result.ProviderType = provider.ProviderType
	return result
}
