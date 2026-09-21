package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	ModelQualityScheduleTask       = "model_quality_schedule"
	ModelQualityExecuteTask        = "model_quality_execute"
	QualityMaxResponseBytes        = 2 << 20
	QualityMaxSVGBytes             = 512 << 10
	QualityMaxStreamBytes          = 64 << 20
	QualityMaxEventBytes           = 8 << 20
	QualityMaxOutputTokens         = 32768
	QualityTimelineBuckets         = 144
	QualityBucketMillis      int64 = 10 * 60 * 1000
)

var (
	ErrQualityConflict  = errors.New("model_quality_conflict")
	ErrQualityDisabled  = errors.New("model_quality_disabled")
	ErrQualityBudget    = errors.New("model_quality_budget_exhausted")
	ErrQualityOwnership = errors.New("model_quality_ownership_lost")
)

type QualityConfig struct {
	Model            string   `json:"model"`
	OutputType       string   `json:"output_type"`
	Mode             string   `json:"mode"`
	Protocol         string   `json:"protocol"`
	TokenID          int      `json:"token_id"`
	Group            string   `json:"group"`
	ChannelIDs       []int    `json:"channel_ids"`
	Prompt           string   `json:"prompt"`
	Instruction      string   `json:"instruction"`
	InstructionRole  string   `json:"instruction_role"`
	MaxOutputTokens  int      `json:"max_output_tokens"`
	ReasoningEffort  string   `json:"reasoning_effort"`
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	SamplesPerTarget int      `json:"samples_per_target"`
	TimeoutSeconds   int      `json:"timeout_seconds"`
	DailyLimit       int      `json:"daily_limit"`
}

type QualitySchedule struct {
	Enabled         bool   `json:"enabled"`
	Kind            string `json:"kind"`
	IntervalMinutes int    `json:"interval_minutes"`
	Time            string `json:"time"`
	Weekdays        []int  `json:"weekdays"`
	Timezone        string `json:"timezone"`
}

type QualitySettingsConfig struct {
	Enabled          bool  `json:"enabled"`
	TokenIDs         []int `json:"token_ids"`
	DailyLimit       int   `json:"daily_limit"`
	Concurrency      int   `json:"concurrency"`
	RetentionEnabled bool  `json:"retention_enabled"`
	ArtifactDays     int   `json:"artifact_days"`
	MetadataDays     int   `json:"metadata_days"`
}

func DefaultQualitySettings() QualitySettingsConfig {
	return QualitySettingsConfig{DailyLimit: 200, Concurrency: 2, ArtifactDays: 30, MetadataDays: 90, TokenIDs: []int{}}
}

type ModelQualitySettings struct {
	ID        int    `json:"id" gorm:"primaryKey"`
	Version   int64  `json:"version"`
	Config    string `json:"-" gorm:"type:text"`
	UpdatedAt int64  `json:"updated_at"`
}

type ModelQualityCase struct {
	ID                 int64  `json:"id" gorm:"primaryKey"`
	Name               string `json:"name" gorm:"size:128"`
	Description        string `json:"description" gorm:"type:text"`
	Enabled            bool   `json:"enabled"`
	Archived           bool   `json:"archived"`
	Version            int    `json:"version"`
	EditVersion        int64  `json:"edit_version"`
	ScheduleJSON       string `json:"-" gorm:"type:text"`
	ScheduleVersion    int64  `json:"schedule_version"`
	NextRunAt          int64  `json:"next_run_at" gorm:"index"`
	LastScheduleReason string `json:"last_schedule_reason" gorm:"size:64"`
	ActiveRunID        string `json:"active_run_id" gorm:"size:64"`
	CreatedBy          int    `json:"created_by"`
	UpdatedBy          int    `json:"updated_by"`
	CreatedAt          int64  `json:"created_at"`
	UpdatedAt          int64  `json:"updated_at"`
}

type ModelQualityRevision struct {
	ID          int64  `json:"id" gorm:"primaryKey"`
	CaseID      int64  `json:"case_id" gorm:"uniqueIndex:idx_quality_revision"`
	Version     int    `json:"version" gorm:"uniqueIndex:idx_quality_revision"`
	ConfigJSON  string `json:"-" gorm:"type:text"`
	Fingerprint string `json:"fingerprint" gorm:"size:64"`
	CreatedBy   int    `json:"created_by"`
	CreatedAt   int64  `json:"created_at"`
}

type QualityCaseView struct {
	ModelQualityCase
	Config   QualityConfig   `json:"config"`
	Schedule QualitySchedule `json:"schedule"`
}

type QualityCaseWrite struct {
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	Enabled             bool            `json:"enabled"`
	ExpectedEditVersion int64           `json:"expected_edit_version"`
	Config              QualityConfig   `json:"config"`
	Schedule            QualitySchedule `json:"schedule"`
}

type QualityRunRequest struct {
	Version          int   `json:"version"`
	ChannelIDs       []int `json:"channel_ids,omitempty"`
	SamplesPerTarget int   `json:"samples_per_target,omitempty"`
}

type ModelQualityRun struct {
	ID              string `json:"id" gorm:"size:64;primaryKey"`
	CaseID          int64  `json:"case_id" gorm:"index:idx_quality_run_case_time"`
	Version         int    `json:"version"`
	Source          string `json:"source" gorm:"size:16"`
	IdempotencyKey  string `json:"-" gorm:"size:64;uniqueIndex:idx_quality_idempotency"`
	RequestHash     string `json:"-" gorm:"size:64"`
	ConfigJSON      string `json:"-" gorm:"type:text"`
	Status          string `json:"status" gorm:"size:24;index"`
	CancelRequested bool   `json:"cancel_requested"`
	CreatedBy       int    `json:"created_by"`
	CreatedAt       int64  `json:"created_at" gorm:"index:idx_quality_run_case_time"`
	StartedAt       int64  `json:"started_at"`
	FinishedAt      int64  `json:"finished_at"`
	SampleCount     int    `json:"sample_count"`
	ExecutorID      string `json:"executor_id" gorm:"size:64"`
}

type ModelQualitySample struct {
	ID              int64  `json:"id" gorm:"primaryKey"`
	RunID           string `json:"run_id" gorm:"size:64;uniqueIndex:idx_quality_sample_ordinal;index"`
	Ordinal         int    `json:"ordinal" gorm:"uniqueIndex:idx_quality_sample_ordinal"`
	CaseID          int64  `json:"case_id" gorm:"index:idx_quality_sample_scope"`
	Version         int    `json:"version" gorm:"index:idx_quality_sample_scope"`
	TargetChannelID int    `json:"target_channel_id"`
	ChannelID       int    `json:"channel_id" gorm:"index:idx_quality_sample_scope"`
	ChannelName     string `json:"channel_name" gorm:"size:128"`
	Model           string `json:"model" gorm:"size:256"`
	ResponseModel   string `json:"response_model" gorm:"size:256"`
	OutputType      string `json:"output_type" gorm:"size:16"`
	Source          string `json:"source" gorm:"size:16"`
	Status          string `json:"status" gorm:"size:24;index"`
	RequestSuccess  bool   `json:"request_success"`
	RequestID       string `json:"request_id" gorm:"size:128;index"`
	StartedAt       int64  `json:"started_at" gorm:"index:idx_quality_sample_scope"`
	CreatedAt       int64  `json:"created_at"`
	FinishedAt      int64  `json:"finished_at"`
	DurationMs      *int64 `json:"duration_ms"`
	FirstTextMs     *int64 `json:"first_text_ms"`
	InputTokens     *int64 `json:"input_tokens"`
	OutputTokens    *int64 `json:"output_tokens"`
	ErrorCode       string `json:"error_code" gorm:"size:64"`
	Validation      string `json:"validation" gorm:"size:32"`
	FinishReason    string `json:"finish_reason" gorm:"size:64"`
	ExecutorID      string `json:"-" gorm:"size:64"`
	BudgetDay       string `json:"-" gorm:"size:10"`
	Annotation      string `json:"annotation" gorm:"size:32"`
	Note            string `json:"note" gorm:"type:text"`
	AnnotatedBy     int    `json:"annotated_by"`
	AnnotatedAt     int64  `json:"annotated_at"`
	ArtifactExpired bool   `json:"artifact_expired"`
	Pinned          bool   `json:"pinned"`
}

type ModelQualityArtifact struct {
	SampleID          int64  `json:"sample_id" gorm:"primaryKey"`
	Text              []byte `json:"-"`
	SVG               []byte `json:"-"`
	SHA256            string `json:"sha256" gorm:"size:64"`
	ValidationVersion string `json:"validation_version" gorm:"size:32"`
	ExpiresAt         int64  `json:"expires_at" gorm:"index"`
}

type QualityResult struct {
	Status         string `json:"status"`
	RequestSuccess bool   `json:"request_success"`
	RequestID      string `json:"request_id"`
	ChannelID      int    `json:"channel_id"`
	ChannelName    string `json:"channel_name"`
	ResponseModel  string `json:"response_model"`
	DurationMs     *int64 `json:"duration_ms"`
	FirstTextMs    *int64 `json:"first_text_ms"`
	InputTokens    *int64 `json:"input_tokens"`
	OutputTokens   *int64 `json:"output_tokens"`
	ErrorCode      string `json:"error_code"`
	Validation     string `json:"validation"`
	FinishReason   string `json:"finish_reason"`
	Text           []byte `json:"-"`
	SVG            []byte `json:"-"`
}

type ModelQualityBudget struct {
	ID       string `json:"-" gorm:"size:64;primaryKey"`
	Day      string `json:"day" gorm:"size:10;index"`
	Reserved int    `json:"reserved"`
	Started  int    `json:"started"`
}

func ValidateQualityConfig(c QualityConfig) error {
	if strings.TrimSpace(c.Model) == "" || len(c.Model) > 256 || strings.TrimSpace(c.Prompt) == "" || len(c.Prompt)+len(c.Instruction) > 32768 {
		return errors.New("invalid model or prompt")
	}
	if !slices.Contains([]string{"svg", "text"}, c.OutputType) || !slices.Contains([]string{"route", "channel"}, c.Mode) || !slices.Contains([]string{"responses", "chat"}, c.Protocol) {
		return errors.New("invalid output type, mode or protocol")
	}
	if c.TokenID < 1 || c.SamplesPerTarget < 1 || c.SamplesPerTarget > 10 || c.TimeoutSeconds < 10 || c.TimeoutSeconds > 600 || c.MaxOutputTokens < 1 || c.MaxOutputTokens > QualityMaxOutputTokens || c.DailyLimit < 1 || c.DailyLimit > 10000 {
		return errors.New("execution limits are out of range")
	}
	if c.Instruction != "" && c.InstructionRole != "system" && c.InstructionRole != "developer" {
		return errors.New("invalid instruction role")
	}
	if !slices.Contains([]string{"", "none", "minimal", "low", "medium", "high", "xhigh"}, c.ReasoningEffort) {
		return errors.New("invalid reasoning effort")
	}
	if c.Temperature != nil && (math.IsNaN(*c.Temperature) || math.IsInf(*c.Temperature, 0) || *c.Temperature < 0 || *c.Temperature > 2) {
		return errors.New("invalid temperature")
	}
	if c.TopP != nil && (math.IsNaN(*c.TopP) || math.IsInf(*c.TopP, 0) || *c.TopP < 0 || *c.TopP > 1) {
		return errors.New("invalid top_p")
	}
	if c.Mode == "route" && len(c.ChannelIDs) != 0 {
		return errors.New("normal routing cannot pin channels")
	}
	if c.Mode == "channel" && (len(c.ChannelIDs) == 0 || len(c.ChannelIDs)*c.SamplesPerTarget > 100) {
		return errors.New("invalid channel sample count")
	}
	seen := make(map[int]bool)
	for _, id := range c.ChannelIDs {
		if id <= 0 || seen[id] {
			return errors.New("invalid or duplicate channel")
		}
		seen[id] = true
	}
	return nil
}

func QualityConfigFingerprint(c QualityConfig) (string, string, error) {
	if err := ValidateQualityConfig(c); err != nil {
		return "", "", err
	}
	body, err := common.Marshal(c)
	if err != nil {
		return "", "", err
	}
	hash := sha256.Sum256(body)
	return string(body), hex.EncodeToString(hash[:]), nil
}

func ValidateQualitySettings(s QualitySettingsConfig) error {
	if s.DailyLimit < 1 || s.DailyLimit > 10000 || s.Concurrency < 1 || s.Concurrency > 2 || s.ArtifactDays < 1 || s.ArtifactDays > 90 || s.MetadataDays < s.ArtifactDays || s.MetadataDays > 365 || len(s.TokenIDs) > 20 {
		return errors.New("invalid quality settings")
	}
	seen := map[int]bool{}
	for _, id := range s.TokenIDs {
		if id < 1 || seen[id] {
			return errors.New("invalid execution token IDs")
		}
		seen[id] = true
	}
	if s.Enabled && len(s.TokenIDs) == 0 {
		return errors.New("an approved execution token is required")
	}
	return nil
}

func NextQualitySchedule(s QualitySchedule, after time.Time, anchor time.Time) (time.Time, error) {
	if !s.Enabled {
		return time.Time{}, nil
	}
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return time.Time{}, errors.New("invalid schedule timezone")
	}
	if s.Kind == "interval" {
		if s.IntervalMinutes < 1 || s.IntervalMinutes > 10080 {
			return time.Time{}, errors.New("schedule interval must be 1-10080 minutes")
		}
		d := time.Duration(s.IntervalMinutes) * time.Minute
		if anchor.IsZero() {
			anchor = after
		}
		if anchor.After(after) {
			return anchor, nil
		}
		return anchor.Add((after.Sub(anchor)/d + 1) * d), nil
	}
	if s.Kind != "daily" && s.Kind != "weekly" {
		return time.Time{}, errors.New("invalid schedule kind")
	}
	clock, err := time.Parse("15:04", s.Time)
	if err != nil {
		return time.Time{}, errors.New("schedule time must be HH:mm")
	}
	if s.Kind == "weekly" && len(s.Weekdays) == 0 {
		return time.Time{}, errors.New("weekly schedule needs a weekday")
	}
	for _, day := range s.Weekdays {
		if day < 1 || day > 7 {
			return time.Time{}, errors.New("weekday must be 1-7")
		}
	}
	local := after.In(loc)
	for offset := range 9 {
		date := time.Date(local.Year(), local.Month(), local.Day()+offset, 12, 0, 0, 0, loc)
		weekday := int(date.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		if s.Kind == "weekly" && !slices.Contains(s.Weekdays, weekday) {
			continue
		}
		candidate := time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), 0, 0, loc)
		if candidate.Hour() != clock.Hour() || candidate.Minute() != clock.Minute() || candidate.Day() != date.Day() {
			continue
		}
		for shift := -2 * time.Hour; shift <= 2*time.Hour; shift += 15 * time.Minute {
			other := candidate.Add(shift)
			view := other.In(loc)
			if other.Before(candidate) && view.Year() == date.Year() && view.Month() == date.Month() && view.Day() == date.Day() && view.Hour() == clock.Hour() && view.Minute() == clock.Minute() {
				candidate = other
			}
		}
		if candidate.After(after) {
			return candidate, nil
		}
	}
	return time.Time{}, errors.New("no schedule occurrence found")
}
