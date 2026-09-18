package model

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func GetMediaModelChannels(groups []string, modelName string, constraints *dto.ChannelConstraints) ([]*Channel, error) {
	modelName = strings.TrimSpace(modelName)
	groups = normalizeTransportGroups(groups)
	if modelName == "" || len(groups) == 0 {
		return nil, nil
	}
	names := []string{modelName}
	if matching := ratio_setting.RoutingMatchModelName(modelName); matching != "" && matching != modelName {
		names = append(names, matching)
	}

	var abilities []Ability
	err := DB.Where("model IN ? AND enabled = ?", names, true).Where(commonGroupCol+" IN ?", groups).Find(&abilities).Error
	if err != nil {
		return nil, err
	}
	candidateSet := make(map[int]struct{}, len(abilities))
	candidateIDs := make([]int, 0, len(abilities))
	for _, ability := range abilities {
		if _, seen := candidateSet[ability.ChannelId]; seen {
			continue
		}
		candidateSet[ability.ChannelId] = struct{}{}
		candidateIDs = append(candidateIDs, ability.ChannelId)
	}
	if pin, found, _ := constraints.ResolvedPin(); found {
		candidateIDs = slices.DeleteFunc(candidateIDs, func(id int) bool { return id != pin.ChannelId })
	}
	if len(candidateIDs) == 0 {
		return nil, nil
	}

	var channels []Channel
	if err := DB.Where("id IN ? AND status = ?", candidateIDs, common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return nil, err
	}
	if !common.MemoryCacheEnabled {
		result := make([]*Channel, 0, len(channels))
		for i := range channels {
			result = append(result, &channels[i])
		}
		slices.SortFunc(result, func(a, b *Channel) int { return a.Id - b.Id })
		return result, nil
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	dbEnabled := make(map[int]struct{}, len(channels))
	for i := range channels {
		dbEnabled[channels[i].Id] = struct{}{}
	}
	cached := make(map[int]*Channel, len(channels))
	for _, group := range groups {
		channelIDs := group2model2channels[group][modelName]
		if len(channelIDs) == 0 {
			if matching := ratio_setting.RoutingMatchModelName(modelName); matching != "" && matching != modelName {
				channelIDs = group2model2channels[group][matching]
			}
		}
		for _, id := range channelIDs {
			if _, hasAbility := candidateSet[id]; !hasAbility {
				continue
			}
			if _, enabled := dbEnabled[id]; !enabled {
				continue
			}
			channel, ok := channelsIDM[id]
			if !ok || channel == nil || channel.Status != common.ChannelStatusEnabled {
				continue
			}
			snapshot := *channel
			cached[id] = &snapshot
		}
	}
	result := make([]*Channel, 0, len(cached))
	for _, id := range candidateIDs {
		if channel, ok := cached[id]; ok {
			result = append(result, channel)
		}
	}
	slices.SortFunc(result, func(a, b *Channel) int { return a.Id - b.Id })
	return result, nil
}

func MappedUpstreamModel(channel *Channel, modelName string) string {
	if channel == nil {
		return modelName
	}
	mappingJSON := channel.GetModelMapping()
	if mappingJSON == "" || mappingJSON == "{}" {
		return modelName
	}
	modelMap := make(map[string]string)
	if err := common.UnmarshalJsonStr(mappingJSON, &modelMap); err != nil {
		return ""
	}
	tail, cyclic := followChannelModelMapping(modelMap, modelName)
	if cyclic || tail == "" {
		return ""
	}
	return tail
}
