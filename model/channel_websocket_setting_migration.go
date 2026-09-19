package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"gorm.io/gorm"
)

const legacySupportsWebSocketsKey = "supports_websockets"

// migrateChannelResponsesWebSocketSetting moves the AI Cove channel toggle
// other_settings.supports_websockets onto the upstream field
// setting.responses_websocket_enabled and drops the legacy key. Rows without the
// legacy key are untouched, so the migration is idempotent.
func migrateChannelResponsesWebSocketSetting(db *gorm.DB) error {
	var channels []Channel
	if err := db.Where("settings LIKE ?", "%"+legacySupportsWebSocketsKey+"%").Find(&channels).Error; err != nil {
		return fmt.Errorf("load channels with legacy websocket setting: %w", err)
	}
	for i := range channels {
		channel := &channels[i]
		otherSettings := map[string]any{}
		if channel.OtherSettings != "" {
			if err := common.UnmarshalJsonStr(channel.OtherSettings, &otherSettings); err != nil {
				return fmt.Errorf("parse other settings of channel %d: %w", channel.Id, err)
			}
		}
		legacy, found := otherSettings[legacySupportsWebSocketsKey]
		if !found {
			continue
		}
		delete(otherSettings, legacySupportsWebSocketsKey)
		otherBytes, err := common.Marshal(otherSettings)
		if err != nil {
			return fmt.Errorf("encode other settings of channel %d: %w", channel.Id, err)
		}
		setting := dto.ChannelSettings{}
		if channel.Setting != nil && *channel.Setting != "" {
			if err := common.UnmarshalJsonStr(*channel.Setting, &setting); err != nil {
				return fmt.Errorf("parse setting of channel %d: %w", channel.Id, err)
			}
		}
		enabled, _ := legacy.(bool)
		setting.ResponsesWebSocketEnabled = enabled && isResponsesWebSocketChannelType(channel.Type)
		settingBytes, err := common.Marshal(setting)
		if err != nil {
			return fmt.Errorf("encode setting of channel %d: %w", channel.Id, err)
		}
		if err := db.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
			"settings": string(otherBytes),
			"setting":  string(settingBytes),
		}).Error; err != nil {
			return fmt.Errorf("migrate websocket setting of channel %d: %w", channel.Id, err)
		}
		common.SysLog(fmt.Sprintf("migrated channel %d responses websocket toggle: enabled=%t", channel.Id, setting.ResponsesWebSocketEnabled))
	}
	return nil
}
