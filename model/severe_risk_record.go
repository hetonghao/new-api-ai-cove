package model

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type SevereRiskChannelScope string
type SevereRiskActionStatus string

const (
	SevereRiskChannelScopeAll SevereRiskChannelScope = "all"
	SevereRiskChannelScopeKey SevereRiskChannelScope = "key"

	SevereRiskActionPending SevereRiskActionStatus = "pending"
	SevereRiskActionSuccess SevereRiskActionStatus = "success"
	SevereRiskActionFailed  SevereRiskActionStatus = "failed"
)

var ErrInvalidSevereRiskRecord = errors.New("invalid severe risk record")

type SevereRiskRecord struct {
	Id                    int                    `json:"id" gorm:"primaryKey"`
	RequestID             string                 `json:"request_id" gorm:"type:varchar(256);not null;uniqueIndex"`
	ChannelID             int                    `json:"channel_id" gorm:"not null;index"`
	ChannelName           string                 `json:"channel_name" gorm:"type:varchar(128);not null"`
	UserID                int                    `json:"user_id" gorm:"not null;index"`
	Username              string                 `json:"username" gorm:"type:varchar(128);not null"`
	TokenID               int                    `json:"token_id" gorm:"not null"`
	TokenName             string                 `json:"token_name" gorm:"type:varchar(128);not null"`
	Model                 string                 `json:"model" gorm:"type:varchar(256);not null"`
	Path                  string                 `json:"path" gorm:"type:varchar(512);not null"`
	ErrorCode             string                 `json:"error_code" gorm:"type:varchar(128);not null"`
	ErrorDetail           string                 `json:"error_detail" gorm:"type:text;not null"`
	ContextHash           string                 `json:"context_hash" gorm:"type:varchar(64);not null"`
	ContextEncrypted      string                 `json:"-" gorm:"type:text;not null"`
	ChannelScope          SevereRiskChannelScope `json:"channel_scope" gorm:"type:varchar(16);not null"`
	ChannelKeyFingerprint string                 `json:"channel_key_fingerprint,omitempty" gorm:"type:varchar(64);not null"`
	UserActionStatus      SevereRiskActionStatus `json:"user_action_status" gorm:"type:varchar(16);not null"`
	ChannelActionStatus   SevereRiskActionStatus `json:"channel_action_status" gorm:"type:varchar(16);not null"`
	TriggeredAt           time.Time              `json:"triggered_at" gorm:"not null;index"`
	ActionClaimedAt       *time.Time             `json:"-"`
}

func (SevereRiskRecord) TableName() string { return "severe_risk_records" }

type SevereRiskRecordInput struct {
	RequestID             string
	ChannelID             int
	ChannelName           string
	UserID                int
	Username              string
	TokenID               int
	TokenName             string
	Model                 string
	Path                  string
	ErrorCode             string
	ErrorDetail           string
	ContextHash           string
	ContextEncrypted      string
	ChannelScope          SevereRiskChannelScope
	ChannelKeyFingerprint string
	UserActionStatus      SevereRiskActionStatus
	ChannelActionStatus   SevereRiskActionStatus
	TriggeredAt           time.Time
}

type SevereRiskRecordQuery struct {
	Offset         int
	Limit          int
	StartTimestamp int64
	EndTimestamp   int64
	ChannelID      int
	UserID         int
	Model          string
	RequestID      string
	ActionStatus   SevereRiskActionStatus
}

func RecordSevereRiskEvent(ctx context.Context, input SevereRiskRecordInput) error {
	record, err := newSevereRiskRecord(input)
	if err != nil {
		return err
	}
	var existing SevereRiskRecord
	result := DB.WithContext(ctx).Where("request_id = ?", record.RequestID).First(&existing)
	if result.Error == nil || !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		if result.Error != nil {
			return fmt.Errorf("check severe risk record: %w", result.Error)
		}
		return nil
	}
	if err := DB.WithContext(ctx).Create(&record).Error; err != nil {
		if duplicate := DB.WithContext(ctx).Where("request_id = ?", record.RequestID).First(&existing); duplicate.Error == nil {
			return nil
		}
		return fmt.Errorf("record severe risk event %q: %w", record.RequestID, err)
	}
	return nil
}

func QuerySevereRiskRecords(ctx context.Context, filter SevereRiskRecordQuery) ([]*SevereRiskRecord, int64, error) {
	if filter.Offset < 0 || filter.Limit < 1 || filter.Limit > 100 || filter.StartTimestamp < 0 || filter.EndTimestamp < 0 ||
		(filter.StartTimestamp > 0 && filter.EndTimestamp > 0 && filter.StartTimestamp > filter.EndTimestamp) || filter.ChannelID < 0 || filter.UserID < 0 {
		return nil, 0, ErrInvalidSevereRiskRecord
	}
	query := DB.WithContext(ctx).Model(&SevereRiskRecord{})
	if filter.StartTimestamp > 0 {
		query = query.Where("triggered_at >= ?", time.Unix(filter.StartTimestamp, 0).UTC())
	}
	if filter.EndTimestamp > 0 {
		query = query.Where("triggered_at <= ?", time.Unix(filter.EndTimestamp, 0).UTC())
	}
	if filter.ChannelID > 0 {
		query = query.Where("channel_id = ?", filter.ChannelID)
	}
	if filter.UserID > 0 {
		query = query.Where("user_id = ?", filter.UserID)
	}
	if filter.Model != "" {
		query = query.Where("model = ?", strings.TrimSpace(filter.Model))
	}
	if filter.RequestID != "" {
		query = query.Where("request_id = ?", strings.TrimSpace(filter.RequestID))
	}
	if filter.ActionStatus != "" {
		if filter.ActionStatus != SevereRiskActionPending && filter.ActionStatus != SevereRiskActionSuccess && filter.ActionStatus != SevereRiskActionFailed {
			return nil, 0, ErrInvalidSevereRiskRecord
		}
		query = query.Where("(user_action_status = ? OR channel_action_status = ?)", filter.ActionStatus, filter.ActionStatus)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count severe risk records: %w", err)
	}
	records := make([]*SevereRiskRecord, 0)
	if err := query.Order("triggered_at desc, id desc").Offset(filter.Offset).Limit(filter.Limit).Find(&records).Error; err != nil {
		return nil, 0, fmt.Errorf("query severe risk records: %w", err)
	}
	return records, total, nil
}

func GetSevereRiskRecord(ctx context.Context, id int) (*SevereRiskRecord, error) {
	if id <= 0 {
		return nil, ErrInvalidSevereRiskRecord
	}
	var record SevereRiskRecord
	if err := DB.WithContext(ctx).First(&record, id).Error; err != nil {
		return nil, fmt.Errorf("get severe risk record: %w", err)
	}
	return &record, nil
}

func GetSevereRiskRecordByRequestID(ctx context.Context, requestID string) (*SevereRiskRecord, error) {
	if strings.TrimSpace(requestID) == "" {
		return nil, ErrInvalidSevereRiskRecord
	}
	var record SevereRiskRecord
	if err := DB.WithContext(ctx).Where("request_id = ?", requestID).First(&record).Error; err != nil {
		return nil, fmt.Errorf("get severe risk record by request id: %w", err)
	}
	return &record, nil
}

func ClaimSevereRiskAction(ctx context.Context, requestID string) (bool, error) {
	if strings.TrimSpace(requestID) == "" {
		return false, ErrInvalidSevereRiskRecord
	}
	claimed := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var record SevereRiskRecord
		if err := lockForUpdate(tx).Where("request_id = ?", requestID).First(&record).Error; err != nil {
			return fmt.Errorf("load severe risk action claim: %w", err)
		}
		if record.ActionClaimedAt != nil {
			return nil
		}
		now := time.Now().UTC()
		if err := tx.Model(&record).Update("action_claimed_at", now).Error; err != nil {
			return fmt.Errorf("claim severe risk action: %w", err)
		}
		claimed = true
		return nil
	})
	return claimed, err
}

func UpdateSevereRiskActionStatus(ctx context.Context, requestID string, userStatus, channelStatus SevereRiskActionStatus) error {
	if strings.TrimSpace(requestID) == "" {
		return ErrInvalidSevereRiskRecord
	}
	updates := map[string]interface{}{}
	if userStatus != "" {
		if !validSevereRiskActionStatus(userStatus) {
			return ErrInvalidSevereRiskRecord
		}
		updates["user_action_status"] = userStatus
	}
	if channelStatus != "" {
		if !validSevereRiskActionStatus(channelStatus) {
			return ErrInvalidSevereRiskRecord
		}
		updates["channel_action_status"] = channelStatus
	}
	if len(updates) == 0 {
		return ErrInvalidSevereRiskRecord
	}
	return DB.WithContext(ctx).Model(&SevereRiskRecord{}).Where("request_id = ?", requestID).Updates(updates).Error
}

func validSevereRiskActionStatus(status SevereRiskActionStatus) bool {
	return status == SevereRiskActionPending || status == SevereRiskActionSuccess || status == SevereRiskActionFailed
}

func newSevereRiskRecord(input SevereRiskRecordInput) (SevereRiskRecord, error) {
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.ChannelName = strings.TrimSpace(input.ChannelName)
	input.Username = strings.TrimSpace(input.Username)
	input.TokenName = strings.TrimSpace(input.TokenName)
	input.Model = strings.TrimSpace(input.Model)
	input.Path = strings.TrimSpace(input.Path)
	input.ErrorCode = strings.TrimSpace(input.ErrorCode)
	input.ErrorDetail = strings.TrimSpace(input.ErrorDetail)
	if runes := []rune(input.ErrorDetail); len(runes) > 512 {
		input.ErrorDetail = string(runes[:512])
	}
	if input.RequestID == "" || len(input.RequestID) > 256 || input.ChannelID <= 0 || input.UserID <= 0 || input.TokenID <= 0 || input.Model == "" || input.Path == "" || input.ErrorCode != "invalid_prompt" || input.ContextHash == "" || input.ContextEncrypted == "" || input.TriggeredAt.IsZero() {
		return SevereRiskRecord{}, ErrInvalidSevereRiskRecord
	}
	if input.ChannelScope != SevereRiskChannelScopeAll && input.ChannelScope != SevereRiskChannelScopeKey {
		return SevereRiskRecord{}, ErrInvalidSevereRiskRecord
	}
	if input.UserActionStatus == "" {
		input.UserActionStatus = SevereRiskActionPending
	}
	if input.ChannelActionStatus == "" {
		input.ChannelActionStatus = SevereRiskActionPending
	}
	return SevereRiskRecord{
		RequestID: input.RequestID, ChannelID: input.ChannelID, ChannelName: input.ChannelName,
		UserID: input.UserID, Username: input.Username, TokenID: input.TokenID, TokenName: input.TokenName,
		Model: input.Model, Path: input.Path, ErrorCode: input.ErrorCode, ErrorDetail: input.ErrorDetail,
		ContextHash: input.ContextHash, ContextEncrypted: input.ContextEncrypted, ChannelScope: input.ChannelScope,
		ChannelKeyFingerprint: input.ChannelKeyFingerprint, UserActionStatus: input.UserActionStatus,
		ChannelActionStatus: input.ChannelActionStatus, TriggeredAt: input.TriggeredAt.UTC(),
	}, nil
}
