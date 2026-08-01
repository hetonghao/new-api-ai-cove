package service

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/go-redis/redis/v8"
)

const (
	riskModerationPromptSemantics         = "cloudflare-user-message-max16-temp0-v1"
	riskModerationClassificationSemantics = "safe-unsafe-error-unsafe-first-v1"
	riskModerationRoundRobinNamespace     = "new-api:risk-moderation-round-robin:v1"
	riskModerationCircuitNamespace        = "new-api:risk-moderation-circuit:v1"
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

type riskModerationProviderTier struct {
	priority  int
	providers []*model.RiskProvider
}

func RiskModerationPolicyVersion(input RiskModerationInput) (string, error) {
	chunkLimit, err := riskModerationChunkLimit(input)
	if err != nil {
		return "", err
	}
	providers, err := riskModerationPolicyProviders(input)
	if err != nil {
		return "", err
	}
	promptSemantics := make([]string, 0, len(providers))
	seenSemantics := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		semantics, ok := map[model.RiskProviderType]string{
			model.RiskProviderCloudflare:       riskModerationPromptSemantics,
			model.RiskProviderPlatformInternal: platformInternalRiskPromptSemantics,
		}[provider.ProviderType]
		if !ok {
			return "", fmt.Errorf("%w: provider type", ErrInvalidRiskModerationInput)
		}
		if _, exists := seenSemantics[semantics]; exists {
			continue
		}
		seenSemantics[semantics] = struct{}{}
		promptSemantics = append(promptSemantics, semantics)
	}
	sort.Strings(promptSemantics)
	material, err := common.Marshal(struct {
		ReviewMode              model.RiskReviewMode `json:"review_mode"`
		ChunkLimit              int                  `json:"chunk_limit"`
		PromptSemantics         []string             `json:"prompt_semantics"`
		ClassificationSemantics string               `json:"classification_semantics"`
	}{
		ReviewMode:              input.ReviewMode,
		ChunkLimit:              chunkLimit,
		PromptSemantics:         promptSemantics,
		ClassificationSemantics: riskModerationClassificationSemantics,
	})
	if err != nil {
		return "", fmt.Errorf("encode risk moderation policy: %w", err)
	}
	return hex.EncodeToString(common.Sha256Raw(material)), nil
}

func riskModerationPolicyProviders(input RiskModerationInput) ([]*model.RiskProvider, error) {
	if len(input.Providers) > 0 {
		if input.Provider != nil {
			return nil, ErrInvalidRiskModerationInput
		}
		return validateRiskModerationProviders(input.Providers)
	}
	if input.Provider != nil {
		return validateRiskModerationProviders([]*model.RiskProvider{input.Provider})
	}
	return nil, ErrInvalidRiskModerationInput
}

func validateRiskModerationProviders(providers []*model.RiskProvider) ([]*model.RiskProvider, error) {
	if len(providers) == 0 {
		return nil, ErrInvalidRiskModerationInput
	}
	for _, provider := range providers {
		if provider == nil {
			return nil, ErrInvalidRiskModerationInput
		}
	}
	return providers, nil
}

func riskModerationProviders(input RiskModerationInput) ([]*model.RiskProvider, error) {
	if len(input.Providers) > 0 {
		if input.Provider != nil {
			return nil, ErrInvalidRiskModerationInput
		}
		return validateRiskModerationProviders(input.Providers)
	}
	if input.Provider != nil {
		return validateRiskModerationProviders([]*model.RiskProvider{input.Provider})
	}
	providers, err := model.GetEnabledRiskProviders()
	if err != nil {
		return nil, fmt.Errorf("load enabled risk providers: %w", err)
	}
	if len(providers) == 0 {
		return nil, ErrRiskModerationNoAvailableProvider
	}
	return providers, nil
}

func riskModerationProviderTiers(providers []*model.RiskProvider) []riskModerationProviderTier {
	byPriority := make(map[int][]*model.RiskProvider)
	for _, provider := range providers {
		byPriority[provider.Priority] = append(byPriority[provider.Priority], provider)
	}
	priorities := make([]int, 0, len(byPriority))
	for priority := range byPriority {
		priorities = append(priorities, priority)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(priorities)))
	tiers := make([]riskModerationProviderTier, 0, len(priorities))
	for _, priority := range priorities {
		members := byPriority[priority]
		sort.SliceStable(members, func(left, right int) bool {
			return members[left].Id < members[right].Id
		})
		tiers = append(tiers, riskModerationProviderTier{priority: priority, providers: members})
	}
	return tiers
}

func loadRiskModerationProviderCursor(ctx context.Context, policyVersion string, priority int, size int) (riskModerationProviderCursor, error) {
	if size < 2 || !common.RedisEnabled || common.RDB == nil {
		return riskModerationProviderCursor{}, nil
	}
	key := fmt.Sprintf("%s:%s:%d", riskModerationRoundRobinNamespace, policyVersion, priority)
	value, err := common.RDB.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return riskModerationProviderCursor{key: key, shared: true}, nil
	}
	if err != nil || value < 0 {
		return riskModerationProviderCursor{}, err
	}
	return riskModerationProviderCursor{key: key, value: value, shared: true}, nil
}

func riskModerationProviderCircuitKey(provider *model.RiskProvider) string {
	return fmt.Sprintf("%s:%d", riskModerationCircuitNamespace, provider.Id)
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
