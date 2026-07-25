package model

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type RiskRecordResult string

const (
	RiskRecordResultSafe   RiskRecordResult = "safe"
	RiskRecordResultUnsafe RiskRecordResult = "unsafe"
	RiskRecordResultError  RiskRecordResult = "error"
)

var (
	ErrInvalidRiskRecord     = errors.New("invalid risk record")
	ErrInvalidRiskRecordPage = errors.New("invalid risk record pagination")
	riskRecordCode           = regexp.MustCompile(`^[A-Za-z0-9._:/-]+$`)
)

type RiskRecord struct {
	Id               int              `json:"id" gorm:"primaryKey"`
	RequestID        string           `json:"request_id" gorm:"type:varchar(256);not null;index"`
	ChannelID        int              `json:"channel_id" gorm:"not null"`
	UserID           int              `json:"user_id" gorm:"not null"`
	RuleIDs          []int            `json:"rule_ids" gorm:"serializer:json;type:text;not null"`
	ProviderID       int              `json:"provider_id" gorm:"not null"`
	ProviderName     string           `json:"provider_name" gorm:"type:varchar(128);not null"`
	Result           RiskRecordResult `json:"result" gorm:"type:varchar(16);not null"`
	Categories       []string         `json:"categories" gorm:"serializer:json;type:text;not null"`
	LatencyMS        int64            `json:"latency_ms" gorm:"not null"`
	PromptTokens     int              `json:"prompt_tokens" gorm:"not null"`
	CompletionTokens int              `json:"completion_tokens" gorm:"not null"`
	TotalTokens      int              `json:"total_tokens" gorm:"not null"`
	Neurons          int64            `json:"neurons" gorm:"not null"`
	ErrorCode        string           `json:"error_code" gorm:"type:varchar(128);not null"`
	ObservedAt       time.Time        `json:"observed_at" gorm:"not null;index"`
}

type RiskRecordInput struct {
	RequestID        string
	ChannelID        int
	UserID           int
	RuleIDs          []int
	ProviderID       int
	ProviderName     string
	Result           RiskRecordResult
	Categories       []string
	LatencyMS        int64
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Neurons          int64
	ErrorCode        string
	ObservedAt       time.Time
}

func (RiskRecord) TableName() string {
	return "risk_records"
}

func RecordRiskObservation(ctx context.Context, input RiskRecordInput) error {
	record, err := newRiskRecord(input)
	if err != nil {
		return fmt.Errorf("normalize risk observation: %w", err)
	}
	if err := DB.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("record risk observation %q: %w", record.RequestID, err)
	}
	return nil
}

func ListRiskRecords(ctx context.Context, offset int, limit int) ([]*RiskRecord, int64, error) {
	if offset < 0 || limit < 1 || limit > 100 {
		return nil, 0, ErrInvalidRiskRecordPage
	}
	records := make([]*RiskRecord, 0)
	var total int64
	query := DB.WithContext(ctx).Model(&RiskRecord{})
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count risk records: %w", err)
	}
	if err := query.Order("observed_at desc, id desc").Offset(offset).Limit(limit).Find(&records).Error; err != nil {
		return nil, 0, fmt.Errorf("list risk records: %w", err)
	}
	return records, total, nil
}

func newRiskRecord(input RiskRecordInput) (RiskRecord, error) {
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.ProviderName = strings.TrimSpace(input.ProviderName)
	input.ErrorCode = strings.TrimSpace(input.ErrorCode)
	if input.RequestID == "" || len(input.RequestID) > 256 || input.ChannelID < 1 || input.UserID < 1 {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if len(input.ProviderName) > 128 || input.ObservedAt.IsZero() {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if input.LatencyMS < 0 || input.PromptTokens < 0 || input.CompletionTokens < 0 || input.TotalTokens < 0 || input.Neurons < 0 {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	providerOptional := false
	switch input.Result {
	case RiskRecordResultSafe, RiskRecordResultUnsafe:
		if input.ErrorCode != "" {
			return RiskRecord{}, ErrInvalidRiskRecord
		}
	case RiskRecordResultError:
		if input.ErrorCode == "" || len(input.ErrorCode) > 128 || !riskRecordCode.MatchString(input.ErrorCode) {
			return RiskRecord{}, ErrInvalidRiskRecord
		}
		switch input.ErrorCode {
		case "queue_full", "service_shutdown", "policy_error", "rules_error":
			providerOptional = true
		}
	default:
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	providerPresent := input.ProviderID > 0 && input.ProviderName != ""
	providerMissing := input.ProviderID == 0 && input.ProviderName == ""
	if !providerPresent && (!providerOptional || !providerMissing) {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if len(input.RuleIDs) > 64 || len(input.Categories) > 64 {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	seenRuleIDs := make(map[int]struct{}, len(input.RuleIDs))
	ruleIDs := make([]int, 0, len(input.RuleIDs))
	for _, ruleID := range input.RuleIDs {
		if ruleID < 1 {
			return RiskRecord{}, ErrInvalidRiskRecord
		}
		if _, exists := seenRuleIDs[ruleID]; exists {
			continue
		}
		seenRuleIDs[ruleID] = struct{}{}
		ruleIDs = append(ruleIDs, ruleID)
	}
	categories := make([]string, 0, len(input.Categories))
	for _, category := range input.Categories {
		category = strings.TrimSpace(category)
		if category == "" || len(category) > 128 {
			return RiskRecord{}, ErrInvalidRiskRecord
		}
		categories = append(categories, category)
	}
	return RiskRecord{
		RequestID: input.RequestID, ChannelID: input.ChannelID, UserID: input.UserID, RuleIDs: ruleIDs,
		ProviderID: input.ProviderID, ProviderName: input.ProviderName, Result: input.Result, Categories: categories,
		LatencyMS: input.LatencyMS, PromptTokens: input.PromptTokens, CompletionTokens: input.CompletionTokens,
		TotalTokens: input.TotalTokens, Neurons: input.Neurons, ErrorCode: input.ErrorCode, ObservedAt: input.ObservedAt.UTC(),
	}, nil
}
