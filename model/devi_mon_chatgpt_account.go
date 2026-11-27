package model

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/QuantumNous/new-api/common"
)

// A Devin monitor account reads its subscription data from one of two channels:
// the Codex credential inventory inside CPA, or a ChatGPT sign-in stored here.
// The row is also the switch: deleting it returns the account to the CPA channel.
const (
	DeviMonChannelCPA     = "cpa"
	DeviMonChannelChatGPT = "chatgpt"
)

var ErrDeviMonChatGPTAccountMissing = errors.New("the ChatGPT sign-in is not stored")

// DeviMonChatGPTAccount holds one ChatGPT OAuth sign-in. Tokens are stored
// encrypted and never leave the gateway.
type DeviMonChatGPTAccount struct {
	Email        string `gorm:"type:varchar(254);primaryKey"`
	AccountID    string `gorm:"type:varchar(64);not null;default:''"`
	Channel      string `gorm:"type:varchar(16);not null;default:'chatgpt'"`
	AccessToken  string `gorm:"type:text"`
	RefreshToken string `gorm:"type:text"`
	IDToken      string `gorm:"type:text"`
	ExpiresAt    int64  `gorm:"not null;default:0"`
	CreatedAt    int64  `gorm:"not null;default:0"`
	UpdatedAt    int64  `gorm:"not null;default:0"`
}

func DeviMonChatGPTEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func GetDeviMonChatGPTAccounts(ctx context.Context) (map[string]DeviMonChatGPTAccount, error) {
	var rows []DeviMonChatGPTAccount
	if err := DB.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	accounts := make(map[string]DeviMonChatGPTAccount, len(rows))
	for _, row := range rows {
		accounts[DeviMonChatGPTEmail(row.Email)] = row
	}
	return accounts, nil
}

func GetDeviMonChatGPTAccount(ctx context.Context, email string) (DeviMonChatGPTAccount, error) {
	var row DeviMonChatGPTAccount
	err := DB.WithContext(ctx).Where("email = ?", DeviMonChatGPTEmail(email)).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DeviMonChatGPTAccount{}, ErrDeviMonChatGPTAccountMissing
	}
	return row, err
}

func SaveDeviMonChatGPTAccount(ctx context.Context, row *DeviMonChatGPTAccount) error {
	row.Email = DeviMonChatGPTEmail(row.Email)
	return DB.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "email"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"account_id", "channel", "access_token", "refresh_token", "id_token",
			"expires_at", "updated_at",
		}),
	}).Create(row).Error
}

func UpdateDeviMonChatGPTTokens(ctx context.Context, email, accessToken, refreshToken, idToken string, expiresAt int64) error {
	return DB.WithContext(ctx).Model(&DeviMonChatGPTAccount{}).
		Where("email = ?", DeviMonChatGPTEmail(email)).
		Updates(map[string]any{
			"access_token": accessToken, "refresh_token": refreshToken, "id_token": idToken,
			"expires_at": expiresAt, "updated_at": common.GetTimestamp(),
		}).Error
}

// SetDeviMonChatGPTChannel only moves the switch. Tokens stay, so switching
// back does not require signing in again.
func SetDeviMonChatGPTChannel(ctx context.Context, email, channel string) (bool, error) {
	result := DB.WithContext(ctx).Model(&DeviMonChatGPTAccount{}).
		Where("email = ?", DeviMonChatGPTEmail(email)).
		Updates(map[string]any{"channel": channel, "updated_at": common.GetTimestamp()})
	return result.RowsAffected == 1, result.Error
}

func DeleteDeviMonChatGPTAccount(ctx context.Context, email string) error {
	return DB.WithContext(ctx).Where("email = ?", DeviMonChatGPTEmail(email)).
		Delete(&DeviMonChatGPTAccount{}).Error
}
