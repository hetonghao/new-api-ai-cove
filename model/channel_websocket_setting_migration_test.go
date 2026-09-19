package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateChannelResponsesWebSocketSettingMovesLegacyToggle(t *testing.T) {
	resetPricingEndpointTestTables(t)

	insert := func(id int, channelType int, setting string, otherSettings string) {
		channel := &Channel{
			Id:            id,
			Type:          channelType,
			Key:           "key",
			Name:          "ws-migration",
			Models:        "gpt-5.4",
			Group:         "default",
			Status:        common.ChannelStatusEnabled,
			OtherSettings: otherSettings,
		}
		if setting != "" {
			channel.Setting = common.GetPointer(setting)
		}
		require.NoError(t, DB.Create(channel).Error)
	}
	insert(901, constant.ChannelTypeOpenAI, `{"proxy":"http://proxy.local"}`, `{"supports_websockets":true,"claude_beta_query":true}`)
	insert(902, constant.ChannelTypeAnthropic, "", `{"supports_websockets":true}`)
	insert(903, constant.ChannelTypeOpenAI, "", `{"supports_websockets":false}`)
	insert(904, constant.ChannelTypeOpenAI, `{"responses_websocket_enabled":true}`, `{"claude_beta_query":true}`)

	for range 2 {
		require.NoError(t, migrateChannelResponsesWebSocketSetting(DB))
	}

	load := func(id int) *Channel {
		channel, err := GetChannelById(id, true)
		require.NoError(t, err)
		return channel
	}
	migrated := load(901)
	assert.True(t, migrated.GetSetting().ResponsesWebSocketEnabled)
	assert.Equal(t, "http://proxy.local", migrated.GetSetting().Proxy)
	assert.True(t, migrated.GetOtherSettings().ClaudeBetaQuery)
	assert.NotContains(t, migrated.OtherSettings, legacySupportsWebSocketsKey)

	unsupported := load(902)
	assert.False(t, unsupported.GetSetting().ResponsesWebSocketEnabled)
	assert.NotContains(t, unsupported.OtherSettings, legacySupportsWebSocketsKey)

	disabled := load(903)
	assert.False(t, disabled.GetSetting().ResponsesWebSocketEnabled)
	assert.NotContains(t, disabled.OtherSettings, legacySupportsWebSocketsKey)

	untouched := load(904)
	assert.True(t, untouched.GetSetting().ResponsesWebSocketEnabled)
	assert.Equal(t, `{"claude_beta_query":true}`, untouched.OtherSettings)
}
