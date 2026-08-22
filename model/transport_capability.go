package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

type TransportCapability struct {
	Allowed            bool
	HTTP               bool
	ResponsesWebSocket bool
}

const transportCapabilityResponsesPath = "/v1/responses"

func channelSupportsResponsesHTTP(channel *Channel, modelName string) bool {
	if channel == nil {
		return false
	}
	if channel.Type == constant.ChannelTypeAdvancedCustom {
		config := channel.GetOtherSettings().AdvancedCustom
		return config != nil && config.SupportsPathForModel(transportCapabilityResponsesPath, modelName)
	}
	for _, endpointType := range common.GetEndpointTypesByChannelType(channel.Type, modelName) {
		if endpointType == constant.EndpointTypeOpenAIResponse {
			return true
		}
	}
	return false
}

func GetTransportCapability(groups []string, modelName string) (TransportCapability, error) {
	modelName = strings.TrimSpace(modelName)
	groups = normalizeTransportGroups(groups)
	if modelName == "" || len(groups) == 0 {
		return TransportCapability{}, nil
	}
	matchingModel := ratio_setting.FormatMatchingModelName(modelName)
	modelNames := []string{modelName}
	if matchingModel != modelName {
		modelNames = append(modelNames, matchingModel)
	}

	if common.MemoryCacheEnabled {
		channelSyncLock.RLock()
		defer channelSyncLock.RUnlock()
		var enabledAbilities []struct{ ChannelID int }
		if err := DB.Table("abilities").Select("channel_id").
			Where("model IN ? AND enabled = ?", modelNames, true).
			Where("abilities."+commonGroupCol+" IN ?", groups).Scan(&enabledAbilities).Error; err != nil {
			return TransportCapability{}, err
		}
		enabledChannelIDs := make(map[int]struct{}, len(enabledAbilities))
		for _, ability := range enabledAbilities {
			enabledChannelIDs[ability.ChannelID] = struct{}{}
		}
		capability := TransportCapability{Allowed: len(enabledChannelIDs) > 0}
		for _, group := range groups {
			channelIDs := group2model2channels[group][modelName]
			if len(channelIDs) == 0 && matchingModel != modelName {
				channelIDs = group2model2channels[group][matchingModel]
			}
			channelIDs = filterChannelsByRequestPathAndModel(channelIDs, transportCapabilityResponsesPath, modelName)
			for _, channelID := range channelIDs {
				if _, ok := enabledChannelIDs[channelID]; !ok {
					continue
				}
				channel, ok := channelsIDM[channelID]
				if !ok || channel.Status != common.ChannelStatusEnabled || !channelSupportsResponsesHTTP(channel, modelName) {
					continue
				}
				capability.HTTP = true
				if ChannelSupportsResponsesWebSocket(channel) {
					capability.ResponsesWebSocket = true
				}
			}
		}
		return capability, nil
	}

	var abilities []Ability
	query := DB.Where("model IN ? AND enabled = ?", modelNames, true).Where(commonGroupCol+" IN ?", groups)
	if err := query.Find(&abilities).Error; err != nil {
		return TransportCapability{}, err
	}
	capability := TransportCapability{Allowed: len(abilities) > 0}
	abs := filterAbilitiesByRequestPathAndModel(abilities, transportCapabilityResponsesPath, modelName)
	if len(abs) == 0 {
		return capability, nil
	}
	channelIDs := make([]int, 0, len(abs))
	for _, ability := range abs {
		channelIDs = append(channelIDs, ability.ChannelId)
	}
	var channels []Channel
	if err := DB.Where("id IN ? AND status = ?", channelIDs, common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return TransportCapability{}, err
	}
	channelsByID := make(map[int]*Channel, len(channels))
	for i := range channels {
		channelsByID[channels[i].Id] = &channels[i]
	}
	for _, ability := range abs {
		channel := channelsByID[ability.ChannelId]
		if !channelSupportsResponsesHTTP(channel, ability.Model) {
			continue
		}
		capability.HTTP = true
		if ChannelSupportsResponsesWebSocket(channel) {
			capability.ResponsesWebSocket = true
		}
	}
	return capability, nil
}

func normalizeTransportGroups(groups []string) []string {
	seen := make(map[string]struct{}, len(groups))
	normalized := make([]string, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}
		normalized = append(normalized, group)
	}
	return normalized
}
