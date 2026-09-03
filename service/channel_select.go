package service

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/gin-gonic/gin"
)

func GetChannelConstraints(c *gin.Context) *dto.ChannelConstraints {
	if c == nil {
		return &dto.ChannelConstraints{}
	}
	if existing, ok := common.GetContextKeyType[*dto.ChannelConstraints](c, constant.ContextKeyChannelConstraints); ok && existing != nil {
		return existing
	}
	constraints := &dto.ChannelConstraints{}
	common.SetContextKey(c, constant.ContextKeyChannelConstraints, constraints)
	return constraints
}

func AppendTaskPluginIdentityFilter(c *gin.Context, pluginKey string) {
	if c == nil {
		return
	}
	GetChannelConstraints(c).AddFilter(dto.ChannelFilter{
		Kind:                   dto.FilterTaskPluginIdentity,
		TaskPluginKey:          pluginKey,
		TaskPluginChannelTypes: pinnedTaskPluginChannelTypes(c, pluginKey),
	})
}

type RetryParam struct {
	Ctx                *gin.Context
	TokenGroup         string
	ModelName          string
	RequestPath        string
	RequireWebSockets  bool
	ExcludedChannelIDs map[int]bool
	TriedChannelIDs    map[int]bool
	LastChannelID      int
	LastChannelRoute   string
	Retry              *int
	resetNextTry       bool
}

func (p *RetryParam) GetRetry() int {
	if p.Retry == nil {
		return 0
	}
	return *p.Retry
}

func (p *RetryParam) SetRetry(retry int) {
	p.Retry = &retry
}

func (p *RetryParam) IncreaseRetry() {
	if p.resetNextTry {
		p.resetNextTry = false
		return
	}
	if p.Retry == nil {
		p.Retry = new(int)
	}
	*p.Retry++
}

func (p *RetryParam) ResetRetryNextTry() {
	p.resetNextTry = true
}

func (p *RetryParam) RecordChannel(channel *model.Channel) {
	if channel == nil {
		return
	}
	if p.TriedChannelIDs == nil {
		p.TriedChannelIDs = make(map[int]bool)
	}
	p.TriedChannelIDs[channel.Id] = true
	p.LastChannelID = channel.Id
	p.LastChannelRoute = channelRetryRouteKey(channel)
}

func channelRetryRouteKey(channel *model.Channel) string {
	if channel == nil || channel.BaseURL == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(channel.GetBaseURL()), "/")
}

func orderRetryCandidates(candidates []*model.Channel, lastChannelID int, lastRoute string) []*model.Channel {
	if len(candidates) == 0 || lastChannelID == 0 {
		return append([]*model.Channel(nil), candidates...)
	}
	ordered := make([]*model.Channel, 0, len(candidates))
	for bucket := 0; bucket <= 3; bucket++ {
		for _, candidate := range candidates {
			if retryCandidateBucket(candidate, lastChannelID, lastRoute) == bucket {
				ordered = append(ordered, candidate)
			}
		}
	}
	return ordered
}

func retryCandidateBucket(candidate *model.Channel, lastChannelID int, lastRoute string) int {
	if candidate == nil {
		return 2
	}
	if candidate.Id == lastChannelID {
		return 3
	}
	route := channelRetryRouteKey(candidate)
	if route == "" {
		return 2
	}
	if lastRoute != "" && route != lastRoute {
		return 0
	}
	if lastRoute != "" {
		return 1
	}
	return 2
}

func retryExcludedChannelIDs(param *RetryParam) map[int]bool {
	excluded := make(map[int]bool, len(param.ExcludedChannelIDs)+len(param.TriedChannelIDs))
	for channelID, isExcluded := range param.ExcludedChannelIDs {
		if isExcluded {
			excluded[channelID] = true
		}
	}
	for channelID := range param.TriedChannelIDs {
		excluded[channelID] = true
	}
	return excluded
}

func channelSelectionFilters(param *RetryParam) []dto.ChannelFilter {
	if param == nil {
		return nil
	}
	return GetChannelConstraints(param.Ctx).Filters
}

func collectRetryCandidates(param *RetryParam, group string) ([]*model.Channel, error) {
	excluded := retryExcludedChannelIDs(param)
	candidates := make([]*model.Channel, 0)
	seen := make(map[int]bool)
	for {
		candidate, err := model.GetRandomSatisfiedChannelWithSelection(group, param.ModelName, 0, param.RequestPath, param.RequireWebSockets, excluded, channelSelectionFilters(param))
		if err != nil {
			return nil, err
		}
		if candidate == nil || seen[candidate.Id] {
			return candidates, nil
		}
		seen[candidate.Id] = true
		excluded[candidate.Id] = true
		candidates = append(candidates, candidate)
	}
}

func selectRetryCandidateAcrossGroups(param *RetryParam, groups []string, startGroupIndex int) (*model.Channel, string, int, error) {
	if startGroupIndex < 0 {
		startGroupIndex = 0
	}
	if startGroupIndex >= len(groups) {
		return nil, "", -1, nil
	}
	candidates := make([]*model.Channel, 0)
	groupByChannelID := make(map[int]string)
	groupIndexByChannelID := make(map[int]int)
	for index := startGroupIndex; index < len(groups); index++ {
		group := groups[index]
		groupCandidates, err := collectRetryCandidates(param, group)
		if err != nil {
			return nil, group, -1, err
		}
		for _, candidate := range groupCandidates {
			if _, seen := groupByChannelID[candidate.Id]; seen {
				continue
			}
			candidates = append(candidates, candidate)
			groupByChannelID[candidate.Id] = group
			groupIndexByChannelID[candidate.Id] = index
		}
	}
	ordered := orderRetryCandidates(candidates, param.LastChannelID, param.LastChannelRoute)
	if len(ordered) == 0 {
		return nil, "", -1, nil
	}
	selected := ordered[0]
	return selected, groupByChannelID[selected.Id], groupIndexByChannelID[selected.Id], nil
}

func selectRetryChannel(param *RetryParam, group string, priorityRetry int, allowSameChannelFallback bool) (*model.Channel, error) {
	if len(param.TriedChannelIDs) == 0 {
		return model.GetRandomSatisfiedChannelWithSelection(group, param.ModelName, priorityRetry, param.RequestPath, param.RequireWebSockets, param.ExcludedChannelIDs, channelSelectionFilters(param))
	}
	candidates, err := collectRetryCandidates(param, group)
	if err != nil {
		return nil, err
	}
	if len(candidates) > 0 {
		ordered := orderRetryCandidates(candidates, param.LastChannelID, param.LastChannelRoute)
		return ordered[0], nil
	}
	if !allowSameChannelFallback {
		return nil, nil
	}
	// No unused candidate remains. Retry the most recent channel as the last bucket.
	if param.LastChannelID != 0 {
		excluded := make(map[int]bool, len(param.ExcludedChannelIDs)+len(param.TriedChannelIDs))
		for channelID, isExcluded := range param.ExcludedChannelIDs {
			if isExcluded {
				excluded[channelID] = true
			}
		}
		for channelID := range param.TriedChannelIDs {
			if channelID != param.LastChannelID {
				excluded[channelID] = true
			}
		}
		if !excluded[param.LastChannelID] {
			lastChannel, err := model.GetRandomSatisfiedChannelWithSelection(group, param.ModelName, 0, param.RequestPath, param.RequireWebSockets, excluded, channelSelectionFilters(param))
			if err != nil || lastChannel != nil {
				return lastChannel, err
			}
		}
	}
	return model.GetRandomSatisfiedChannelWithSelection(group, param.ModelName, priorityRetry, param.RequestPath, param.RequireWebSockets, param.ExcludedChannelIDs, channelSelectionFilters(param))
}

func selectLastChannelFallbackInGroups(param *RetryParam, groups []string) (*model.Channel, string, error) {
	if param.LastChannelID == 0 || param.ExcludedChannelIDs[param.LastChannelID] {
		return nil, "", nil
	}
	excluded := retryExcludedChannelIDs(param)
	delete(excluded, param.LastChannelID)
	for _, group := range groups {
		channel, err := model.GetRandomSatisfiedChannelWithSelection(group, param.ModelName, 0, param.RequestPath, param.RequireWebSockets, excluded, channelSelectionFilters(param))
		if err != nil {
			return nil, group, err
		}
		if channel != nil && channel.Id == param.LastChannelID {
			return channel, group, nil
		}
	}
	return nil, "", nil
}

// CacheGetRandomSatisfiedChannel tries to get a random channel that satisfies the requirements.
// 尝试获取一个满足要求的随机渠道。
//
// For "auto" tokenGroup with cross-group Retry enabled:
// 对于启用了跨分组重试的 "auto" tokenGroup：
//
//   - Each group will exhaust all its priorities before moving to the next group.
//     每个分组会用完所有优先级后才会切换到下一个分组。
//
//   - Uses ContextKeyAutoGroupIndex to track current group index.
//     使用 ContextKeyAutoGroupIndex 跟踪当前分组索引。
//
//   - Uses ContextKeyAutoGroupRetryIndex to track the global Retry count when current group started.
//     使用 ContextKeyAutoGroupRetryIndex 跟踪当前分组开始时的全局重试次数。
//
//   - priorityRetry = Retry - startRetryIndex, represents the priority level within current group.
//     priorityRetry = Retry - startRetryIndex，表示当前分组内的优先级级别。
//
//   - When GetRandomSatisfiedChannel returns nil (priorities exhausted), moves to next group.
//     当 GetRandomSatisfiedChannel 返回 nil（优先级用完）时，切换到下一个分组。
//
// Example flow (2 groups, each with 2 priorities, RetryTimes=3):
// 示例流程（2个分组，每个有2个优先级，RetryTimes=3）：
//
//	Retry=0: GroupA, priority0 (startRetryIndex=0, priorityRetry=0)
//	         分组A, 优先级0
//
//	Retry=1: GroupA, priority1 (startRetryIndex=0, priorityRetry=1)
//	         分组A, 优先级1
//
//	Retry=2: GroupA exhausted → GroupB, priority0 (startRetryIndex=2, priorityRetry=0)
//	         分组A用完 → 分组B, 优先级0
//
//	Retry=3: GroupB, priority1 (startRetryIndex=2, priorityRetry=1)
//	         分组B, 优先级1
func CacheGetRandomSatisfiedChannel(param *RetryParam) (*model.Channel, string, error) {
	var channel *model.Channel
	var err error
	selectGroup := param.TokenGroup
	userGroup := common.GetContextKeyString(param.Ctx, constant.ContextKeyUserGroup)

	if param.TokenGroup == "auto" {
		autoGroups := GetRequestAutoGroups(param.Ctx, userGroup)
		if len(autoGroups) == 0 {
			return nil, selectGroup, errors.New("auto groups is not enabled")
		}

		// startGroupIndex: the group index to start searching from
		// startGroupIndex: 开始搜索的分组索引
		startGroupIndex := 0
		crossGroupRetry := common.GetContextKeyBool(param.Ctx, constant.ContextKeyTokenCrossGroupRetry)

		if lastGroupIndex, exists := common.GetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex); exists {
			if idx, ok := lastGroupIndex.(int); ok {
				startGroupIndex = idx
			}
		}

		if crossGroupRetry && len(param.TriedChannelIDs) > 0 {
			channel, selectGroup, selectedGroupIndex, err := selectRetryCandidateAcrossGroups(param, autoGroups, startGroupIndex)
			if err != nil {
				return nil, selectGroup, err
			}
			if channel != nil {
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, selectGroup)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, param.GetRetry())
				if param.GetRetry() >= common.RetryTimes {
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, selectedGroupIndex+1)
				} else {
					common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, selectedGroupIndex)
				}
				return channel, selectGroup, nil
			}
			channel, selectGroup, err = selectLastChannelFallbackInGroups(param, autoGroups)
			if err != nil {
				return nil, selectGroup, err
			}
			if channel != nil {
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, selectGroup)
			}
			return channel, selectGroup, nil
		}

		for i := startGroupIndex; i < len(autoGroups); i++ {
			autoGroup := autoGroups[i]
			// Calculate priorityRetry for current group
			// 计算当前分组的 priorityRetry
			priorityRetry := param.GetRetry()
			// If moved to a new group, reset priorityRetry and update startRetryIndex
			// 如果切换到新分组，重置 priorityRetry 并更新 startRetryIndex
			if i > startGroupIndex {
				priorityRetry = 0
			}
			logger.LogDebug(param.Ctx, "Auto selecting group: %s, priorityRetry: %d", autoGroup, priorityRetry)

			channel, err = selectRetryChannel(param, autoGroup, priorityRetry, !crossGroupRetry)
			if err != nil {
				return nil, selectGroup, err
			}
			if channel == nil {
				// Current group has no available channel for this model, try next group
				// 当前分组没有该模型的可用渠道，尝试下一个分组
				logger.LogDebug(param.Ctx, "No available channel in group %s for model %s at priorityRetry %d, trying next group", autoGroup, param.ModelName, priorityRetry)
				// 重置状态以尝试下一个分组
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, param.GetRetry())
				continue
			}
			common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, autoGroup)
			selectGroup = autoGroup
			logger.LogDebug(param.Ctx, "Auto selected group: %s", autoGroup)

			// Prepare state for next retry
			// 为下一次重试准备状态
			if crossGroupRetry && priorityRetry >= common.RetryTimes {
				// Current group has exhausted all retries, prepare to switch to next group
				// This request still uses current group, but next retry will use next group
				// 当前分组已用完所有重试次数，准备切换到下一个分组
				// 本次请求仍使用当前分组，但下次重试将使用下一个分组
				logger.LogDebug(param.Ctx, "Current group %s retries exhausted (priorityRetry=%d >= RetryTimes=%d), preparing switch to next group for next retry", autoGroup, priorityRetry, common.RetryTimes)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i+1)
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupRetryIndex, param.GetRetry())
			} else {
				// Stay in current group, save current state
				// 保持在当前分组，保存当前状态
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroupIndex, i)
			}
			break
		}
		if channel == nil && crossGroupRetry {
			channel, selectGroup, err = selectLastChannelFallbackInGroups(param, autoGroups)
			if err != nil {
				return nil, selectGroup, err
			}
			if channel != nil {
				common.SetContextKey(param.Ctx, constant.ContextKeyAutoGroup, selectGroup)
			}
		}
	} else {
		channel, err = selectRetryChannel(param, param.TokenGroup, param.GetRetry(), true)
		if err != nil {
			return nil, param.TokenGroup, err
		}
	}
	return channel, selectGroup, nil
}

func pinnedTaskPluginChannelTypes(c *gin.Context, expected string) []int {
	if c == nil || expected == "" {
		return nil
	}
	if value, exists := c.Get(jsplugin.ContextKeyPinnedEndpoint); exists {
		pinned, ok := value.(jsplugin.PinnedEndpoint)
		if ok && pinned.Generation != nil && len(pinned.Candidates) > 1 {
			expectedFound := false
			channelTypes := make([]int, 0, len(pinned.Candidates))
			seen := make(map[int]struct{}, len(pinned.Candidates))
			for _, candidate := range pinned.Candidates {
				if candidate.Plugin == nil {
					continue
				}
				if candidate.Plugin.Meta.Key == expected {
					expectedFound = true
				}
				for _, channelType := range candidate.Plugin.Meta.ChannelTypes {
					if channelType == 0 || channelType == constant.ChannelTypeTaskPlugin {
						continue
					}
					if _, duplicate := seen[channelType]; duplicate {
						continue
					}
					if plugin, indexed := pinned.Generation.GetByChannelType(channelType); indexed && plugin == candidate.Plugin {
						seen[channelType] = struct{}{}
						channelTypes = append(channelTypes, channelType)
					}
				}
			}
			if expectedFound {
				return channelTypes
			}
		}
	}
	value, exists := c.Get(jsplugin.ContextKeyPinnedPlugin)
	pinned, ok := value.(jsplugin.PinnedPlugin)
	if !exists || !ok || pinned.Generation == nil || pinned.Plugin == nil || pinned.Plugin.Meta.Key != expected {
		return nil
	}
	channelTypes := make([]int, 0, len(pinned.Plugin.Meta.ChannelTypes))
	for _, channelType := range pinned.Plugin.Meta.ChannelTypes {
		if channelType == 0 || channelType == constant.ChannelTypeTaskPlugin {
			continue
		}
		channelTypes = append(channelTypes, channelType)
	}
	if len(channelTypes) == 0 {
		return nil
	}
	return channelTypes
}
