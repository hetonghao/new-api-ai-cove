package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var (
	ErrInvalidSevereRiskUser = errors.New("invalid severe risk user")
	ErrSevereRiskRootUser    = errors.New("root user cannot be disabled by severe risk automation")
)

func DisableUserForSevereRisk(ctx context.Context, userID int) error {
	if userID <= 0 {
		return ErrInvalidSevereRiskUser
	}
	if err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx.Unscoped()).Where("id = ?", userID).First(&user).Error; err != nil {
			return fmt.Errorf("load severe risk user: %w", err)
		}
		if user.Role == common.RoleRootUser {
			return ErrSevereRiskRootUser
		}
		if user.Status == common.UserStatusDisabled {
			return nil
		}
		if _, err := IncrementUserAuthVersionWithTx(tx, userID); err != nil {
			return fmt.Errorf("advance severe risk user auth version: %w", err)
		}
		if err := tx.Model(&User{}).Where("id = ?", userID).Update("status", common.UserStatusDisabled).Error; err != nil {
			return fmt.Errorf("disable severe risk user: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := PublishUserAuthCache(userID); err != nil {
		return fmt.Errorf("publish severe risk user cache: %w", err)
	}
	if _, err := RevokeAllUserSessions(userID, "severe_risk"); err != nil {
		return fmt.Errorf("revoke severe risk user sessions: %w", err)
	}
	if err := InvalidateUserTokensCache(userID); err != nil {
		return fmt.Errorf("invalidate severe risk user tokens: %w", err)
	}
	return nil
}

func QuarantineChannel(channelID int, usingKey string, reason string) bool {
	if channelID <= 0 {
		return false
	}
	channel, err := GetChannelById(channelID, true)
	if err != nil {
		return false
	}
	if channel.ChannelInfo.IsMultiKey && usingKey != "" {
		for index, key := range channel.GetKeys() {
			if key == usingKey && channel.ChannelInfo.MultiKeyStatusList[index] == common.ChannelStatusSevereDisabled {
				return true
			}
		}
	} else if channel.Status == common.ChannelStatusSevereDisabled {
		return true
	}
	return UpdateChannelStatus(channelID, usingKey, common.ChannelStatusSevereDisabled, reason)
}
