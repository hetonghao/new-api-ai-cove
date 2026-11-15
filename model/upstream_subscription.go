package model

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	SubscriptionPlatformOpenCode      = "opencode"
	SubscriptionPlatformOpenDesign    = "opendesign"
	AuthFlowPurposeSubscription       = "subscription_device"
	DefaultSubscriptionRefreshMinutes = 10
)

var ErrSubscriptionChanged = errors.New("Subscription changed; reload and try again")
var ErrSubscriptionChannelUnavailable = errors.New("Selected channel was not found; select another channel or use an independent name")
var ErrSubscriptionAccountTooLong = errors.New("Subscription account must be at most 128 characters")

type UpstreamSubscriptionChannel struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Priority int64  `json:"priority"`
	Weight   uint   `json:"weight"`
}

type SubscriptionQuotaWindow struct {
	Key              string     `json:"key"`
	DurationSeconds  int64      `json:"duration_seconds"`
	UsedPercent      *float64   `json:"used_percent"`
	Status           string     `json:"status"`
	UsedCredits      string     `json:"used_credits,omitempty"`
	LimitCredits     string     `json:"limit_credits,omitempty"`
	RemainingCredits string     `json:"remaining_credits,omitempty"`
	ResetsAt         *time.Time `json:"resets_at"`
}

type SubscriptionQuota struct {
	Windows               []SubscriptionQuotaWindow `json:"windows"`
	WorkspaceID           string                    `json:"workspace_id,omitempty"`
	WorkspaceName         string                    `json:"workspace_name,omitempty"`
	Tier                  string                    `json:"tier,omitempty"`
	Eligible              *bool                     `json:"eligible,omitempty"`
	BalanceUSD            *string                   `json:"balance_usd,omitempty"`
	SubscriptionPeriodEnd *time.Time                `json:"subscription_period_end,omitempty"`
}

type UpstreamSubscription struct {
	ID                            int                          `json:"id" gorm:"primaryKey"`
	Name                          string                       `json:"name" gorm:"type:varchar(128);not null"`
	Account                       string                       `json:"account" gorm:"type:varchar(128);not null;default:''"`
	ChannelID                     int                          `json:"channel_id" gorm:"not null;default:0;index"`
	Channel                       *UpstreamSubscriptionChannel `json:"channel" gorm:"-"`
	Platform                      string                       `json:"platform" gorm:"type:varchar(32);not null"`
	CredentialEncrypted           string                       `json:"-" gorm:"type:text;not null"`
	RefreshIntervalMinutes        int                          `json:"refresh_interval_minutes" gorm:"not null;default:10"`
	AutoRefreshDisabled           bool                         `json:"auto_refresh_disabled" gorm:"not null;default:false"`
	SubscriptionPeriodEndAt       int64                        `json:"-" gorm:"not null;default:0"`
	SubscriptionInfoLastAttemptAt int64                        `json:"-" gorm:"not null;default:0"`
	SubscriptionInfoNextRefreshAt int64                        `json:"-" gorm:"not null;default:0;index"`
	QuotaJSON                     string                       `json:"-" gorm:"type:text"`
	LastSuccessAt                 int64                        `json:"last_success_at"`
	LastAttemptAt                 int64                        `json:"last_attempt_at"`
	NextRefreshAt                 int64                        `json:"next_refresh_at" gorm:"index"`
	FailureCount                  int                          `json:"-"`
	ErrorCode                     string                       `json:"error_code" gorm:"type:varchar(32)"`
	CreatedAt                     time.Time                    `json:"created_at"`
	UpdatedAt                     time.Time                    `json:"updated_at"`
}

type UpstreamSubscriptionResponse struct {
	UpstreamSubscription
	Quota *SubscriptionQuota `json:"quota"`
	Stale bool               `json:"stale"`
}

type UpstreamSubscriptionConfig struct {
	ID                     int
	Name                   *string
	Account                *string
	ChannelID              *int
	Platform               string
	CredentialEncrypted    string
	RefreshIntervalMinutes int
	AutoRefreshDisabled    *bool
}

func NormalizeSubscriptionRefreshMinutes(minutes int) (int, error) {
	if minutes == 0 {
		return DefaultSubscriptionRefreshMinutes, nil
	}
	if minutes < 1 || minutes > 1440 {
		return 0, errors.New("Refresh interval must be between 1 and 1440 minutes")
	}
	return minutes, nil
}

func (s UpstreamSubscription) RefreshInterval() time.Duration {
	minutes := s.RefreshIntervalMinutes
	if minutes < 1 {
		minutes = DefaultSubscriptionRefreshMinutes
	}
	return time.Duration(minutes) * time.Minute
}

func (s UpstreamSubscription) Response() UpstreamSubscriptionResponse {
	response := UpstreamSubscriptionResponse{UpstreamSubscription: s}
	if s.Channel != nil {
		response.Name = s.Channel.Name
	}
	if s.QuotaJSON != "" {
		var quota SubscriptionQuota
		if common.UnmarshalJsonStr(s.QuotaJSON, &quota) == nil {
			response.Quota = &quota
		} else {
			response.ErrorCode = "invalid_response"
		}
	}
	if s.SubscriptionPeriodEndAt > 0 {
		if response.Quota == nil {
			response.Quota = &SubscriptionQuota{Windows: []SubscriptionQuotaWindow{}}
		}
		periodEnd := time.Unix(s.SubscriptionPeriodEndAt, 0).UTC()
		response.Quota.SubscriptionPeriodEnd = &periodEnd
	} else if s.SubscriptionInfoLastAttemptAt > 0 && response.Quota != nil {
		response.Quota.SubscriptionPeriodEnd = nil
	}
	response.Stale = response.Quota != nil && (response.ErrorCode != "" || s.LastSuccessAt < time.Now().Add(-2*s.RefreshInterval()).Unix())
	return response
}

func NormalizeUpstreamSubscription(name, platform string) (string, error) {
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > 128 {
		return "", errors.New("Subscription name must be at most 128 characters")
	}
	switch platform {
	case SubscriptionPlatformOpenCode:
		if name == "" {
			name = "OpenCode Go"
		}
	case SubscriptionPlatformOpenDesign:
		if name == "" {
			name = "OpenDesign Go"
		}
	default:
		return "", errors.New("Unsupported subscription platform")
	}
	return name, nil
}

func ListUpstreamSubscriptions() ([]UpstreamSubscription, error) {
	subscriptions := make([]UpstreamSubscription, 0)
	err := DB.Order("id desc").Find(&subscriptions).Error
	if err != nil {
		return nil, err
	}
	ids := make([]int, 0)
	for _, subscription := range subscriptions {
		if subscription.ChannelID > 0 {
			ids = append(ids, subscription.ChannelID)
		}
	}
	if len(ids) > 0 {
		channels := make([]UpstreamSubscriptionChannel, 0)
		if err := DB.Model(&Channel{}).Select("id", "name", "priority", "weight").Where("id IN ?", ids).Find(&channels).Error; err != nil {
			return nil, err
		}
		byID := make(map[int]*UpstreamSubscriptionChannel, len(channels))
		for i := range channels {
			byID[channels[i].ID] = &channels[i]
		}
		for i := range subscriptions {
			subscriptions[i].Channel = byID[subscriptions[i].ChannelID]
		}
	}
	return subscriptions, err
}

func ListUpstreamSubscriptionChannels() ([]UpstreamSubscriptionChannel, error) {
	channels := make([]UpstreamSubscriptionChannel, 0)
	err := DB.Model(&Channel{}).Select("id", "name", "priority", "weight").Order("name").Order("id").Find(&channels).Error
	return channels, err
}

func GetUpstreamSubscriptionChannel(tx *gorm.DB, id int) (*UpstreamSubscriptionChannel, error) {
	var channel UpstreamSubscriptionChannel
	err := tx.Model(&Channel{}).Select("id", "name", "priority", "weight").Where("id = ?", id).Take(&channel).Error
	return &channel, err
}

func GetUpstreamSubscription(id int) (*UpstreamSubscription, error) {
	var subscription UpstreamSubscription
	err := DB.First(&subscription, id).Error
	if err == nil && subscription.ChannelID > 0 {
		channel, channelErr := GetUpstreamSubscriptionChannel(DB, subscription.ChannelID)
		if channelErr == nil {
			subscription.Channel = channel
		} else if !errors.Is(channelErr, gorm.ErrRecordNotFound) {
			return nil, channelErr
		}
	}
	return &subscription, err
}

func SaveUpstreamSubscriptionWithTx(tx *gorm.DB, config UpstreamSubscriptionConfig) (*UpstreamSubscription, error) {
	var subscription UpstreamSubscription
	if config.ID > 0 {
		if err := tx.First(&subscription, config.ID).Error; err != nil {
			return nil, err
		}
		if subscription.Platform != config.Platform {
			return nil, errors.New("Subscription platform cannot be changed")
		}
	}
	account := subscription.Account
	autoRefreshDisabled := subscription.AutoRefreshDisabled
	if config.AutoRefreshDisabled != nil {
		autoRefreshDisabled = *config.AutoRefreshDisabled
	}
	if config.Account != nil {
		account = strings.TrimSpace(*config.Account)
	}
	if utf8.RuneCountInString(account) > 128 {
		return nil, ErrSubscriptionAccountTooLong
	}
	channelID := subscription.ChannelID
	if config.ChannelID != nil {
		channelID = *config.ChannelID
	}
	if channelID < 0 {
		return nil, ErrSubscriptionChannelUnavailable
	}
	nameInput := subscription.Name
	if config.Name != nil {
		nameInput = *config.Name
	}
	if channelID > 0 {
		channel, err := GetUpstreamSubscriptionChannel(tx, channelID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSubscriptionChannelUnavailable
		}
		if err != nil {
			return nil, err
		}
		subscription.Channel = channel
		nameInput = subscription.Name
	}
	name, err := NormalizeUpstreamSubscription(nameInput, config.Platform)
	if err != nil {
		return nil, err
	}
	if config.ID == 0 {
		if config.CredentialEncrypted == "" {
			return nil, errors.New("Subscription credential is required")
		}
		minutes, err := NormalizeSubscriptionRefreshMinutes(config.RefreshIntervalMinutes)
		if err != nil {
			return nil, err
		}
		subscription.Name = name
		subscription.Account = account
		subscription.Platform = config.Platform
		subscription.ChannelID = channelID
		subscription.CredentialEncrypted = config.CredentialEncrypted
		subscription.RefreshIntervalMinutes = minutes
		subscription.AutoRefreshDisabled = autoRefreshDisabled
		return &subscription, tx.Create(&subscription).Error
	}
	minutes := config.RefreshIntervalMinutes
	if minutes == 0 {
		minutes = subscription.RefreshIntervalMinutes
	}
	minutes, err = NormalizeSubscriptionRefreshMinutes(minutes)
	if err != nil {
		return nil, err
	}
	if config.CredentialEncrypted == "" && name == subscription.Name && account == subscription.Account && channelID == subscription.ChannelID && minutes == subscription.RefreshIntervalMinutes && autoRefreshDisabled == subscription.AutoRefreshDisabled {
		return &subscription, nil
	}
	updates := map[string]any{"name": name, "account": account, "channel_id": channelID, "refresh_interval_minutes": minutes, "auto_refresh_disabled": autoRefreshDisabled}
	if subscription.AutoRefreshDisabled && !autoRefreshDisabled && subscription.ErrorCode != "rate_limited" {
		updates["next_refresh_at"] = 0
	}
	if minutes != subscription.RefreshIntervalMinutes && subscription.LastAttemptAt > 0 {
		next := subscription.LastAttemptAt + int64(minutes)*60
		// The provider may have installed a retry hold after the initial read.
		updates["next_refresh_at"] = gorm.Expr("CASE WHEN error_code = ? AND next_refresh_at > ? THEN next_refresh_at ELSE ? END", "rate_limited", next, next)
	}
	if config.CredentialEncrypted != "" {
		updates["credential_encrypted"] = config.CredentialEncrypted
		updates["quota_json"] = ""
		updates["last_success_at"] = 0
		updates["last_attempt_at"] = 0
		updates["next_refresh_at"] = 0
		updates["failure_count"] = 0
		updates["error_code"] = ""
		updates["subscription_period_end_at"] = 0
		updates["subscription_info_last_attempt_at"] = 0
		updates["subscription_info_next_refresh_at"] = 0
	}
	result := tx.Model(&subscription).Where("credential_encrypted = ? AND last_attempt_at = ? AND refresh_interval_minutes = ? AND channel_id = ? AND auto_refresh_disabled = ?", subscription.CredentialEncrypted, subscription.LastAttemptAt, subscription.RefreshIntervalMinutes, subscription.ChannelID, subscription.AutoRefreshDisabled).Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected != 1 {
		return nil, ErrSubscriptionChanged
	}
	if err := tx.First(&subscription, config.ID).Error; err != nil {
		return nil, err
	}
	return &subscription, nil
}

func DeleteUpstreamSubscription(id int) error {
	result := DB.Delete(&UpstreamSubscription{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// The compare-and-swap lease also covers manual refreshes on another node.
func ClaimUpstreamSubscriptionRefresh(id int, now int64, manual bool) (*UpstreamSubscription, bool, error) {
	subscription, err := GetUpstreamSubscription(id)
	if err != nil {
		return nil, false, err
	}
	query := DB.Model(&UpstreamSubscription{}).Where("id = ? AND credential_encrypted = ? AND last_attempt_at = ? AND last_attempt_at <= ?", id, subscription.CredentialEncrypted, subscription.LastAttemptAt, now-60)
	if manual {
		query = query.Where("error_code <> ? OR next_refresh_at <= ?", "rate_limited", now)
	} else {
		query = query.Where("auto_refresh_disabled = ? AND next_refresh_at <= ?", false, now)
	}
	result := query.Updates(map[string]any{"last_attempt_at": now, "next_refresh_at": now + 60})
	if result.Error != nil || result.RowsAffected == 0 {
		return subscription, false, result.Error
	}
	subscription.LastAttemptAt = now
	return subscription, true, nil
}

func FinishUpstreamSubscriptionRefresh(subscription *UpstreamSubscription, quota *SubscriptionQuota, code string, retryAt int64) error {
	updates := map[string]any{"error_code": code, "failure_count": subscription.FailureCount + 1}
	seconds := int64(60)
	if quota == nil {
		seconds *= 1 << min(subscription.FailureCount, 3)
	}
	// Use the current cadence without discarding an in-flight provider retry hold.
	autoNext := gorm.Expr("? + CASE WHEN refresh_interval_minutes * ? > 86400 THEN 86400 ELSE refresh_interval_minutes * ? END", subscription.LastAttemptAt, seconds, seconds)
	next := autoNext
	if code == "rate_limited" {
		next = gorm.Expr("CASE WHEN (?) > ? THEN (?) ELSE ? END", autoNext, retryAt, autoNext, retryAt)
	}
	updates["next_refresh_at"] = next
	if quota != nil {
		if quota.WorkspaceID != "" {
			var previous SubscriptionQuota
			if common.UnmarshalJsonStr(subscription.QuotaJSON, &previous) != nil || previous.WorkspaceID != quota.WorkspaceID {
				updates["subscription_period_end_at"] = 0
				updates["subscription_info_last_attempt_at"] = 0
				updates["subscription_info_next_refresh_at"] = 0
			}
		}
		encoded, err := common.Marshal(quota)
		if err != nil {
			return err
		}
		updates["quota_json"] = string(encoded)
		updates["last_success_at"] = time.Now().Unix()
		updates["failure_count"] = 0
	}
	return DB.Model(&UpstreamSubscription{}).
		Where("id = ? AND credential_encrypted = ? AND last_attempt_at = ?", subscription.ID, subscription.CredentialEncrypted, subscription.LastAttemptAt).
		Updates(updates).Error
}

func DueUpstreamSubscriptionIDs(now int64) ([]int, error) {
	var ids []int
	err := DB.Model(&UpstreamSubscription{}).Where("(auto_refresh_disabled = ? AND next_refresh_at <= ? AND last_attempt_at <= ?) OR (subscription_info_next_refresh_at <= ? AND subscription_info_last_attempt_at <= ?)", false, now, now-60, now, now-60).
		Order("next_refresh_at asc, id asc").Limit(25).Pluck("id", &ids).Error
	return ids, err
}

func ClaimUpstreamSubscriptionInfoRefresh(id int, now int64) (*UpstreamSubscription, bool, error) {
	subscription, err := GetUpstreamSubscription(id)
	if err != nil {
		return nil, false, err
	}
	periodEnd := subscription.SubscriptionPeriodEndAt
	// Carry forward the old OpenDesign snapshot until its first independent check succeeds.
	if subscription.SubscriptionInfoLastAttemptAt == 0 && periodEnd == 0 {
		if quota := subscription.Response().Quota; quota != nil && quota.SubscriptionPeriodEnd != nil {
			periodEnd = quota.SubscriptionPeriodEnd.Unix()
		}
	}
	result := DB.Model(&UpstreamSubscription{}).
		Where("id = ? AND credential_encrypted = ? AND subscription_info_last_attempt_at = ? AND subscription_info_last_attempt_at <= ? AND subscription_info_next_refresh_at <= ?", id, subscription.CredentialEncrypted, subscription.SubscriptionInfoLastAttemptAt, now-60, now).
		Updates(map[string]any{"subscription_period_end_at": periodEnd, "subscription_info_last_attempt_at": now, "subscription_info_next_refresh_at": now + 60})
	if result.Error != nil || result.RowsAffected == 0 {
		return subscription, false, result.Error
	}
	subscription.SubscriptionInfoLastAttemptAt = now
	return subscription, true, nil
}

func FinishUpstreamSubscriptionInfoRefresh(subscription *UpstreamSubscription, periodEnd *time.Time, succeeded bool, nextAt int64) error {
	updates := map[string]any{"subscription_info_next_refresh_at": nextAt}
	if succeeded {
		updates["subscription_period_end_at"] = int64(0)
		if periodEnd != nil {
			updates["subscription_period_end_at"] = periodEnd.Unix()
		}
	}
	query := DB.Model(&UpstreamSubscription{}).
		Where("id = ? AND credential_encrypted = ? AND subscription_info_last_attempt_at = ?", subscription.ID, subscription.CredentialEncrypted, subscription.SubscriptionInfoLastAttemptAt)
	if subscription.Platform == SubscriptionPlatformOpenDesign {
		// A controlKey can change workspaces while its information request is in flight.
		query = query.Where("COALESCE(quota_json, '') = ?", subscription.QuotaJSON)
	}
	return query.Updates(updates).Error
}

func ClaimSubscriptionLoginPoll(flow *AuthFlow, encryptedPayload string) (bool, error) {
	result := DB.Model(&AuthFlow{}).Where("id = ? AND payload = ? AND consumed_at IS NULL AND expires_at > ?", flow.Id, flow.Payload, time.Now()).Update("payload", encryptedPayload)
	return result.RowsAffected == 1, result.Error
}

func ExpireSubscriptionLoginFlows(userID int, sessionID string) error {
	return DB.Model(&AuthFlow{}).Where("purpose = ? AND user_id = ? AND session_id = ? AND consumed_at IS NULL", AuthFlowPurposeSubscription, userID, sessionID).
		Update("expires_at", time.Now()).Error
}
