package model

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRiskPolicyModelTest(t *testing.T) {
	t.Helper()
	originalDB := DB
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	require.NoError(t, db.AutoMigrate(&RiskProvider{}, &RiskPolicy{}, &RiskRule{}, &Channel{}, &User{}))
	t.Cleanup(func() {
		DB = originalDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestRiskRulePersistence_lists_created_rule(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	created, err := CreateRiskRule(RiskRuleInput{RuleType: RiskRuleKeyword, Pattern: "  Example  ", Enabled: true})
	require.NoError(t, err)

	// When
	rules, err := GetRiskRules()

	// Then
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Equal(t, created.Id, rules[0].Id)
	require.Equal(t, "Example", rules[0].Pattern)
	require.True(t, rules[0].Enabled)
	require.Equal(t, RiskRuleActionReview, rules[0].Action)
}

func TestRiskRulePersistence_reads_legacy_blank_action_as_review(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	require.NoError(t, DB.Exec(
		"INSERT INTO risk_rules (rule_type, pattern, enabled, action) VALUES (?, ?, ?, ?)",
		RiskRuleKeyword, "legacy", true, "",
	).Error)

	// When
	rules, err := GetRiskRules()

	// Then
	require.NoError(t, err)
	require.Len(t, rules, 1)
	require.Equal(t, RiskRuleActionReview, rules[0].Action)
	rule, err := GetRiskRuleByID(rules[0].Id)
	require.NoError(t, err)
	require.Equal(t, RiskRuleActionReview, rule.Action)
}

func TestRiskRulePersistence_updates_existing_rule(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)
	created, err := CreateRiskRule(RiskRuleInput{RuleType: RiskRuleKeyword, Pattern: "example", Enabled: true})
	require.NoError(t, err)
	enabled := false
	action := RiskRuleActionSkip

	// When
	updated, err := UpdateRiskRule(created.Id, RiskRuleUpdateInput{
		RuleType: RiskRulePhrase, Pattern: "example phrase", Enabled: &enabled, Action: &action,
	})

	// Then
	require.NoError(t, err)
	require.Equal(t, RiskRulePhrase, updated.RuleType)
	require.Equal(t, "example phrase", updated.Pattern)
	require.False(t, updated.Enabled)
	require.Equal(t, RiskRuleActionSkip, updated.Action)
}

func TestRiskRulePersistence_rejects_invalid_action(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)

	// When
	_, err := CreateRiskRule(RiskRuleInput{
		RuleType: RiskRuleKeyword, Pattern: "example", Enabled: true, Action: "block",
	})

	// Then
	require.ErrorIs(t, err, ErrInvalidRiskRuleAction)
}

func TestRiskRulePersistence_rejects_invalid_regex(t *testing.T) {
	// Given
	setupRiskPolicyModelTest(t)

	// When
	_, err := CreateRiskRule(RiskRuleInput{RuleType: RiskRuleRegex, Pattern: "[", Enabled: true})

	// Then
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidRiskRulePattern))
}
