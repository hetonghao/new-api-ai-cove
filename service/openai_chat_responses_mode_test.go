package service

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/assert"
)

func TestShouldPassThroughNewAPIChatCompletionsBody(t *testing.T) {
	assert.True(t, ShouldPassThroughNewAPIChatCompletionsBody(constant.ChannelTypeNewAPI, 54, "deepseek-v4.1-flash"))
	assert.True(t, ShouldPassThroughNewAPIChatCompletionsBody(constant.ChannelTypeNewAPI, 54, "kimi-k3"))
	assert.False(t, ShouldPassThroughNewAPIChatCompletionsBody(constant.ChannelTypeDeepSeek, 54, "deepseek-v4.1-flash"))
	assert.False(t, ShouldPassThroughNewAPIChatCompletionsBody(constant.ChannelTypeOpenAI, 1, "gpt-4o"))
	assert.False(t, ShouldPassThroughNewAPIChatCompletionsBody(constant.ChannelTypeAdvancedCustom, 58, "deepseek-v4.1-flash"))
}

func TestShouldPassThroughNewAPIChatCompletionsBodyYieldsToResponsesUpgrade(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	original := settings.ChatCompletionsToResponsesPolicy
	t.Cleanup(func() { settings.ChatCompletionsToResponsesPolicy = original })
	settings.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{
		Enabled:       true,
		ChannelTypes:  []int{constant.ChannelTypeNewAPI},
		ModelPatterns: []string{"deepseek-.*"},
	}

	assert.False(t, ShouldPassThroughNewAPIChatCompletionsBody(constant.ChannelTypeNewAPI, 54, "deepseek-v4.1-flash"))
	assert.True(t, ShouldPassThroughNewAPIChatCompletionsBody(constant.ChannelTypeNewAPI, 54, "kimi-k3"))
}
