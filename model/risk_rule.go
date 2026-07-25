package model

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type RiskRuleType string

const (
	RiskRuleKeyword      RiskRuleType = "keyword"
	RiskRulePhrase       RiskRuleType = "phrase"
	RiskRuleRegex        RiskRuleType = "regex"
	maxRiskPatternLength              = 4096
)

var ErrInvalidRiskRulePattern = errors.New("invalid risk rule pattern")

type RiskRule struct {
	Id        int          `json:"id" gorm:"primaryKey"`
	RuleType  RiskRuleType `json:"rule_type" gorm:"type:varchar(16);not null"`
	Pattern   string       `json:"pattern" gorm:"type:text;not null"`
	Enabled   bool         `json:"enabled" gorm:"not null"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type RiskRuleInput struct {
	RuleType RiskRuleType
	Pattern  string
	Enabled  bool
}

type RiskRuleUpdateInput struct {
	RuleType RiskRuleType
	Pattern  string
	Enabled  *bool
}

func (RiskRule) TableName() string {
	return "risk_rules"
}

func GetRiskRules() ([]*RiskRule, error) {
	var rules []*RiskRule
	if err := DB.Order("id asc").Find(&rules).Error; err != nil {
		return nil, fmt.Errorf("list risk rules: %w", err)
	}
	return rules, nil
}

func GetRiskRuleByID(id int) (*RiskRule, error) {
	var rule RiskRule
	if err := DB.First(&rule, id).Error; err != nil {
		return nil, fmt.Errorf("get risk rule %d: %w", id, err)
	}
	return &rule, nil
}

func CreateRiskRule(input RiskRuleInput) (*RiskRule, error) {
	if err := ValidateRiskRule(input.RuleType, input.Pattern); err != nil {
		return nil, err
	}
	rule := &RiskRule{RuleType: input.RuleType, Pattern: strings.TrimSpace(input.Pattern), Enabled: input.Enabled}
	if err := DB.Create(rule).Error; err != nil {
		return nil, fmt.Errorf("create risk rule: %w", err)
	}
	return rule, nil
}

func UpdateRiskRule(id int, input RiskRuleUpdateInput) (*RiskRule, error) {
	if err := ValidateRiskRule(input.RuleType, input.Pattern); err != nil {
		return nil, err
	}
	rule, err := GetRiskRuleByID(id)
	if err != nil {
		return nil, err
	}
	rule.RuleType = input.RuleType
	rule.Pattern = strings.TrimSpace(input.Pattern)
	if input.Enabled != nil {
		rule.Enabled = *input.Enabled
	}
	if err := DB.Save(rule).Error; err != nil {
		return nil, fmt.Errorf("update risk rule %d: %w", id, err)
	}
	return rule, nil
}

func DeleteRiskRule(id int) error {
	if err := DB.Delete(&RiskRule{}, id).Error; err != nil {
		return fmt.Errorf("delete risk rule %d: %w", id, err)
	}
	return nil
}

func ValidateRiskRule(ruleType RiskRuleType, pattern string) error {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || len(pattern) > maxRiskPatternLength {
		return ErrInvalidRiskRulePattern
	}
	switch ruleType {
	case RiskRuleKeyword, RiskRulePhrase:
		return nil
	case RiskRuleRegex:
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("compile risk rule regex: %w", ErrInvalidRiskRulePattern)
		}
		return nil
	default:
		return errors.New("unsupported risk rule type")
	}
}
