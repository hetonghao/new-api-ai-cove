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
type RiskRecordSource string

const (
	RiskRecordResultNotReviewed RiskRecordResult = "not_reviewed"
	RiskRecordResultSafe        RiskRecordResult = "safe"
	RiskRecordResultUnsafe      RiskRecordResult = "unsafe"
	RiskRecordResultError       RiskRecordResult = "error"

	RiskRecordSourceLocal    RiskRecordSource = "local"
	RiskRecordSourceProvider RiskRecordSource = "provider"
	RiskRecordSourceCache    RiskRecordSource = "cache"
	RiskRecordSourceInflight RiskRecordSource = "inflight"

	riskProviderValidationPath = "/api/risk/providers/validate"
)

var (
	ErrInvalidRiskRecord     = errors.New("invalid risk record")
	ErrInvalidRiskRecordPage = errors.New("invalid risk record pagination")
	riskRecordCode           = regexp.MustCompile(`^[A-Za-z0-9._:/-]+$`)
	riskRecordContentHash    = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

const riskRecordErrorDetailMaxRunes = 512

type RiskRecord struct {
	Id                 int               `json:"id" gorm:"primaryKey"`
	RequestID          string            `json:"request_id" gorm:"type:varchar(256);not null;index"`
	ChannelID          int               `json:"channel_id" gorm:"not null;index"`
	ChannelName        string            `json:"channel_name" gorm:"->;-:migration"`
	UserID             int               `json:"user_id" gorm:"not null;index"`
	Username           string            `json:"username" gorm:"->;-:migration"`
	TokenID            int               `json:"token_id" gorm:"not null"`
	TokenName          string            `json:"token_name" gorm:"->;-:migration"`
	Model              string            `json:"model" gorm:"type:varchar(256);not null"`
	Path               string            `json:"path" gorm:"type:varchar(512);not null"`
	Preview            string            `json:"preview" gorm:"type:text;not null"`
	ContentHash        string            `json:"content_hash" gorm:"type:varchar(64);not null"`
	RuleIDs            []int             `json:"rule_ids" gorm:"serializer:json;type:text;not null"`
	ProviderID         int               `json:"provider_id" gorm:"not null;index"`
	ProviderName       string            `json:"provider_name" gorm:"type:varchar(128);not null"`
	ProviderType       RiskProviderType  `json:"provider_type" gorm:"type:varchar(32);not null;default:'';index"`
	Result             RiskRecordResult  `json:"result" gorm:"type:varchar(16);not null;index"`
	Categories         []string          `json:"categories" gorm:"serializer:json;type:text;not null"`
	LatencyMS          int64             `json:"latency_ms" gorm:"not null"`
	PromptTokens       int               `json:"prompt_tokens" gorm:"not null"`
	CompletionTokens   int               `json:"completion_tokens" gorm:"not null"`
	TotalTokens        int               `json:"total_tokens" gorm:"not null"`
	Neurons            int64             `json:"neurons" gorm:"not null"`
	Chunks             []RiskRecordChunk `json:"chunks" gorm:"serializer:json;type:text"`
	ErrorCode          string            `json:"error_code" gorm:"type:varchar(128);not null"`
	ErrorDetail        string            `json:"error_detail,omitempty" gorm:"type:varchar(512)"`
	Source             RiskRecordSource  `json:"source" gorm:"type:varchar(16);not null;index"`
	CacheHit           bool              `json:"cache_hit" gorm:"not null"`
	ProviderCalled     bool              `json:"provider_called" gorm:"not null"`
	Blocked            bool              `json:"blocked" gorm:"not null"`
	NonBlockingMatched bool              `json:"non_blocking_matched" gorm:"not null;default:false"`
	ObservedAt         time.Time         `json:"observed_at" gorm:"not null;index"`
}

type RiskRecordInput struct {
	RequestID          string
	ChannelID          int
	UserID             int
	TokenID            int
	Model              string
	Path               string
	Preview            string
	ContentHash        string
	RuleIDs            []int
	ProviderID         int
	ProviderName       string
	ProviderType       RiskProviderType
	Result             RiskRecordResult
	Categories         []string
	LatencyMS          int64
	PromptTokens       int
	CompletionTokens   int
	TotalTokens        int
	Neurons            int64
	Chunks             []RiskRecordChunk
	ErrorCode          string
	ErrorDetail        string
	Source             RiskRecordSource
	CacheHit           bool
	ProviderCalled     bool
	Blocked            bool
	NonBlockingMatched bool
	ObservedAt         time.Time
}

func (RiskRecord) TableName() string {
	return "risk_records"
}

func RecordRiskObservation(ctx context.Context, input RiskRecordInput) error {
	record, err := newRiskRecord(input)
	if err != nil {
		return fmt.Errorf("normalize risk observation: %w", err)
	}
	governance, err := GetRiskRecordGovernance(ctx)
	if err != nil {
		return fmt.Errorf("load risk record governance: %w", err)
	}
	applyRiskRecordContentGovernance(&record, governance)
	if record.Result != RiskRecordResultError {
		switch governance.SaveScope {
		case RiskRecordSaveSuspicious:
			if record.Source == RiskRecordSourceLocal {
				return nil
			}
		case RiskRecordSaveUnsafe:
			if !record.Blocked {
				return nil
			}
		}
	}
	if err := DB.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("record risk observation %q: %w", record.RequestID, err)
	}
	return nil
}

func RecordRiskProviderValidation(ctx context.Context, input RiskRecordInput) error {
	input.Path = riskProviderValidationPath
	record, err := newRiskRecord(input)
	if err != nil {
		return fmt.Errorf("normalize risk provider validation: %w", err)
	}
	governance, err := GetRiskRecordGovernance(ctx)
	if err != nil {
		return fmt.Errorf("load risk record governance: %w", err)
	}
	applyRiskRecordContentGovernance(&record, governance)
	if err := DB.WithContext(ctx).Create(&record).Error; err != nil {
		return fmt.Errorf("record risk provider validation %q: %w", record.RequestID, err)
	}
	return nil
}

func applyRiskRecordContentGovernance(record *RiskRecord, governance RiskRecordGovernance) {
	if governance.ContentSaveScope == RiskContentSaveAll ||
		(governance.ContentSaveScope == RiskContentSaveUnsafe && record.Result == RiskRecordResultUnsafe) {
		previewChars := governance.PreviewCharsForResult(record.Result)
		previewRunes := []rune(record.Preview)
		if len(previewRunes) > previewChars {
			record.Preview = string(previewRunes[:previewChars])
		}
		for index := range record.Chunks {
			summaryRunes := []rune(record.Chunks[index].Summary)
			if len(summaryRunes) > previewChars {
				record.Chunks[index].Summary = string(summaryRunes[:previewChars])
			}
		}
		return
	}
	record.Preview = ""
	record.ContentHash = ""
	for index := range record.Chunks {
		record.Chunks[index].Summary = ""
	}
}

func ListRiskRecords(ctx context.Context, offset int, limit int) ([]*RiskRecord, int64, error) {
	return QueryRiskRecords(ctx, RiskRecordQuery{Offset: offset, Limit: limit})
}

func newRiskRecord(input RiskRecordInput) (RiskRecord, error) {
	input.RequestID = strings.TrimSpace(input.RequestID)
	input.Model = strings.TrimSpace(input.Model)
	input.Path = strings.TrimSpace(input.Path)
	input.Preview = strings.TrimSpace(input.Preview)
	input.ContentHash = strings.TrimSpace(input.ContentHash)
	input.ProviderName = strings.TrimSpace(input.ProviderName)
	input.ErrorCode = strings.TrimSpace(input.ErrorCode)
	input.ErrorDetail = strings.TrimSpace(input.ErrorDetail)
	errorDetailRunes := []rune(input.ErrorDetail)
	if len(errorDetailRunes) > riskRecordErrorDetailMaxRunes {
		input.ErrorDetail = string(errorDetailRunes[:riskRecordErrorDetailMaxRunes])
	}
	if input.RequestID == "" || len(input.RequestID) > 256 || input.ChannelID < 0 || input.UserID < 1 {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if input.ChannelID == 0 && input.Path != riskProviderValidationPath {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if len(input.ProviderName) > 128 || input.ObservedAt.IsZero() {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	switch input.ProviderType {
	case "", RiskProviderCloudflare, RiskProviderPlatformInternal:
	default:
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if input.TokenID < 0 || len(input.Model) > 256 || len(input.Path) > 512 {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if input.Path != "" && (!strings.HasPrefix(input.Path, "/") || strings.ContainsAny(input.Path, "?#")) {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if input.ContentHash != "" && !riskRecordContentHash.MatchString(input.ContentHash) {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if input.LatencyMS < 0 || input.PromptTokens < 0 || input.CompletionTokens < 0 || input.TotalTokens < 0 || input.Neurons < 0 {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	providerOptional := false
	switch input.Result {
	case RiskRecordResultNotReviewed:
		providerOptional = true
		if input.ErrorCode != "" || input.ErrorDetail != "" {
			return RiskRecord{}, ErrInvalidRiskRecord
		}
	case RiskRecordResultSafe, RiskRecordResultUnsafe:
		if input.ErrorCode != "" || input.ErrorDetail != "" {
			return RiskRecord{}, ErrInvalidRiskRecord
		}
	case RiskRecordResultError:
		if input.ErrorCode == "" || len(input.ErrorCode) > 128 || !riskRecordCode.MatchString(input.ErrorCode) {
			return RiskRecord{}, ErrInvalidRiskRecord
		}
		switch input.ErrorCode {
		case "queue_full", "service_shutdown", "policy_error", "rules_error", "provider_config_error", "daily_neurons_exhausted", "budget_unavailable", "provider_unavailable":
			providerOptional = true
		}
	default:
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if input.Source == RiskRecordSourceCache || input.Source == RiskRecordSourceInflight {
		providerOptional = true
	}
	providerPresent := input.ProviderID > 0 && input.ProviderName != ""
	providerMissing := input.ProviderID == 0 && input.ProviderName == ""
	if providerMissing && input.ProviderType != "" {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if !providerPresent && (!providerOptional || !providerMissing) {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if input.Source == "" && providerPresent {
		input.Source = RiskRecordSourceProvider
	}
	if input.Source == "" && providerMissing && (providerOptional || input.Result == RiskRecordResultNotReviewed) {
		input.Source = RiskRecordSourceLocal
	}
	switch input.Source {
	case RiskRecordSourceLocal:
		if input.CacheHit || input.ProviderCalled || !providerMissing {
			return RiskRecord{}, ErrInvalidRiskRecord
		}
	case RiskRecordSourceProvider:
		if input.CacheHit || !providerPresent {
			return RiskRecord{}, ErrInvalidRiskRecord
		}
	case RiskRecordSourceInflight:
		if input.CacheHit || input.ProviderCalled || !providerMissing {
			return RiskRecord{}, ErrInvalidRiskRecord
		}
	case RiskRecordSourceCache:
		if !input.CacheHit || input.ProviderCalled || !providerMissing {
			return RiskRecord{}, ErrInvalidRiskRecord
		}
	default:
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if input.Blocked && input.Result != RiskRecordResultUnsafe {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if input.NonBlockingMatched && (input.Result != RiskRecordResultUnsafe || input.Blocked) {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	if input.Result == RiskRecordResultNotReviewed && input.Source != RiskRecordSourceLocal {
		return RiskRecord{}, ErrInvalidRiskRecord
	}
	chunks, err := normalizeRiskRecordChunks(input.Chunks, input.Source, input.ProviderCalled)
	if err != nil {
		return RiskRecord{}, err
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
		RequestID: input.RequestID, ChannelID: input.ChannelID, UserID: input.UserID, TokenID: input.TokenID,
		Model: input.Model, Path: input.Path, Preview: input.Preview, ContentHash: input.ContentHash, RuleIDs: ruleIDs,
		ProviderID: input.ProviderID, ProviderName: input.ProviderName, ProviderType: input.ProviderType,
		Result: input.Result, Categories: categories,
		LatencyMS: input.LatencyMS, PromptTokens: input.PromptTokens, CompletionTokens: input.CompletionTokens,
		TotalTokens: input.TotalTokens, Neurons: input.Neurons, Chunks: chunks, ErrorCode: input.ErrorCode, ErrorDetail: input.ErrorDetail, Source: input.Source,
		CacheHit: input.CacheHit, ProviderCalled: input.ProviderCalled, Blocked: input.Blocked, NonBlockingMatched: input.NonBlockingMatched, ObservedAt: input.ObservedAt.UTC(),
	}, nil
}
