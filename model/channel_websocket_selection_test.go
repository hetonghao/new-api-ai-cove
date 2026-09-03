package model

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestGetRandomSatisfiedChannelFiltersResponsesWebSocketCapability(t *testing.T) {
	resetPricingEndpointTestTables(t)

	insertWebSocketSelectionChannel(t, 401, constant.ChannelTypeOpenAI, false)
	insertWebSocketSelectionChannel(t, 402, constant.ChannelTypeAnthropic, true)
	insertWebSocketSelectionChannel(t, 403, constant.ChannelTypeOpenAI, true)
	InitChannelCache()

	for _, memoryCacheEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			common.MemoryCacheEnabled = memoryCacheEnabled

			wsChannel, err := GetRandomSatisfiedChannelWithSelection("default", "gpt-5.4", 0, "/v1/responses", true, nil)
			require.NoError(t, err)
			require.NotNil(t, wsChannel)
			require.Equal(t, 403, wsChannel.Id)

			wsFallback, err := GetRandomSatisfiedChannelWithSelection("default", "gpt-5.4", 0, "/v1/responses", true, map[int]bool{403: true})
			require.NoError(t, err)
			require.Nil(t, wsFallback)

			httpChannel, err := GetRandomSatisfiedChannelWithSelection("default", "gpt-5.4", 0, "/v1/responses", false, nil)
			require.NoError(t, err)
			require.NotNil(t, httpChannel)
			require.Contains(t, []int{401, 402, 403}, httpChannel.Id)
		})
	}
}

func TestGetRandomSatisfiedChannelExcludesDisabledWebSocketChannelWithStaleAbility(t *testing.T) {
	resetPricingEndpointTestTables(t)

	insertWebSocketSelectionChannel(t, 404, constant.ChannelTypeOpenAI, true)
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 404).Update("status", common.ChannelStatusManuallyDisabled).Error)
	InitChannelCache()

	for _, memoryCacheEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			common.MemoryCacheEnabled = memoryCacheEnabled

			channel, err := GetRandomSatisfiedChannelWithSelection("default", "gpt-5.4", 0, "/v1/responses", true, nil)
			require.NoError(t, err)
			require.Nil(t, channel)
		})
	}
}

func TestGetRandomSatisfiedChannelSupportsAllResponsesWebSocketChannelTypes(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
	}{
		{name: "OpenAI", channelType: constant.ChannelTypeOpenAI},
		{name: "Codex", channelType: constant.ChannelTypeCodex},
		{name: "Advanced Custom", channelType: constant.ChannelTypeAdvancedCustom},
		{name: "Sub2API", channelType: constant.ChannelTypeSub2API},
		{name: "New API", channelType: constant.ChannelTypeNewAPI},
		{name: "xAI", channelType: constant.ChannelTypeXai},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetPricingEndpointTestTables(t)
			insertWebSocketSelectionChannel(t, 405, tt.channelType, true)
			InitChannelCache()

			for _, memoryCacheEnabled := range []bool{true, false} {
				common.MemoryCacheEnabled = memoryCacheEnabled
				channel, err := GetRandomSatisfiedChannelWithSelection("default", "gpt-5.4", 0, "/v1/responses", true, nil)
				require.NoError(t, err)
				require.NotNil(t, channel)
				require.Equal(t, 405, channel.Id)
			}
		})
	}
}

func TestGetTransportCapabilityReturnsLocalHTTPAndResponsesWebSocketHints(t *testing.T) {
	resetPricingEndpointTestTables(t)
	insertWebSocketSelectionChannel(t, 406, constant.ChannelTypeCodex, false)
	insertWebSocketSelectionChannel(t, 407, constant.ChannelTypeCodex, true)
	InitChannelCache()

	for _, memoryCacheEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			common.MemoryCacheEnabled = memoryCacheEnabled
			capability, err := GetTransportCapability([]string{"default", "default"}, "gpt-5.4")
			require.NoError(t, err)
			require.True(t, capability.Allowed)
			require.True(t, capability.HTTP)
			require.True(t, capability.ResponsesWebSocket)
			unknown, err := GetTransportCapability([]string{"default"}, "missing-model")
			require.NoError(t, err)
			require.False(t, unknown.HTTP)
			require.False(t, unknown.ResponsesWebSocket)
		})
	}
}

func TestGetTransportCapabilityTreatsOpenAIChannelsAsResponsesHTTP(t *testing.T) {
	resetPricingEndpointTestTables(t)
	insertWebSocketSelectionChannel(t, 408, constant.ChannelTypeOpenAI, false)
	insertWebSocketSelectionChannel(t, 409, constant.ChannelTypeOpenAI, true)
	InitChannelCache()

	for _, memoryCacheEnabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("memory_cache_%t", memoryCacheEnabled), func(t *testing.T) {
			common.MemoryCacheEnabled = memoryCacheEnabled
			capability, err := GetTransportCapability([]string{"default"}, "gpt-5.4")
			require.NoError(t, err)
			require.True(t, capability.Allowed)
			require.True(t, capability.HTTP, "OpenAI Responses requests use the channel's ordinary HTTP route")
			require.True(t, capability.ResponsesWebSocket)
		})
	}
}

func insertWebSocketSelectionChannel(t *testing.T, id int, channelType int, supportsWebSockets bool) {
	t.Helper()
	channel := &Channel{
		Id:       id,
		Type:     channelType,
		Key:      fmt.Sprintf("key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", id),
		Models:   "gpt-5.4",
		Group:    "default",
		Priority: common.GetPointer(int64(0)),
		Weight:   common.GetPointer(uint(100)),
	}
	otherSettings := dto.ChannelOtherSettings{SupportsWebSockets: supportsWebSockets}
	if channelType == constant.ChannelTypeAdvancedCustom {
		otherSettings.AdvancedCustom = &dto.AdvancedCustomConfig{
			Routes: []dto.AdvancedCustomRoute{{
				IncomingPath: "/v1/responses",
				UpstreamPath: "/v1/responses",
				Converter:    "none",
			}},
		}
	}
	channel.SetOtherSettings(otherSettings)
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{
		Group:     "default",
		Model:     "gpt-5.4",
		ChannelId: id,
		Enabled:   true,
		Priority:  common.GetPointer(int64(0)),
		Weight:    100,
	}).Error)
}
