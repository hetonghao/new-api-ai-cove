package controller

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpstreamSubscriptionOpenCodeExpiryUsesConsoleStatus(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opencode", "oc_sk_subscription")
	require.NotNil(t, created.Quota)
	require.NotNil(t, created.Quota.SubscriptionPeriodEnd)
	assert.Equal(t, "2026-12-12T14:54:44Z", created.Quota.SubscriptionPeriodEnd.Format(time.RFC3339))
	assert.Equal(t, "2027-01-12T14:54:44Z", created.Quota.Windows[2].ResetsAt.Format(time.RFC3339), "the quota reset is separate from the subscription period")
}

func TestUpstreamSubscriptionCanDisableAutomaticQuotaAndStillRefreshManually(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opencode", "oc_sk_subscription")
	path := "/" + strconv.Itoa(created.ID)
	created = subscriptionResponse(t, s.request(t, http.MethodPut, path, gin.H{"platform": "opencode", "name": "Preserved name", "account": "quota@example.com", "refresh_interval_minutes": 17}, s.bundle.AccessToken))
	updated := s.request(t, http.MethodPut, path, gin.H{"platform": "opencode", "auto_refresh_disabled": true}, s.bundle.AccessToken)
	require.Equal(t, http.StatusOK, updated.Code)
	assert.Contains(t, updated.Body.String(), `"auto_refresh_disabled":true`)
	disabled := subscriptionResponse(t, updated)
	assert.Equal(t, created.Name, disabled.Name)
	assert.Equal(t, created.Account, disabled.Account)
	assert.Equal(t, created.RefreshIntervalMinutes, disabled.RefreshIntervalMinutes)
	_, claimed, err := model.ClaimUpstreamSubscriptionRefresh(created.ID, time.Now().Add(time.Hour).Unix(), false)
	require.NoError(t, err)
	assert.False(t, claimed, "automatic pulls must stop when disabled")
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Update("last_attempt_at", 0).Error)
	before := s.fixture.quotaCalls
	refreshed := subscriptionResponse(t, s.request(t, http.MethodPost, path+"/refresh", nil, s.bundle.AccessToken))
	require.NotNil(t, refreshed.Quota)
	assert.Greater(t, s.fixture.quotaCalls, before)
	updated = s.request(t, http.MethodPut, path, gin.H{"platform": "opencode", "auto_refresh_disabled": false}, s.bundle.AccessToken)
	assert.Contains(t, updated.Body.String(), `"auto_refresh_disabled":false`)
}

func TestUpstreamSubscriptionQuotaPullKeepsCachedExpiry(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opencode", "oc_sk_cached")
	require.Equal(t, 1, s.fixture.infoCalls)
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Update("last_attempt_at", 0).Error)
	updated := subscriptionResponse(t, s.request(t, http.MethodPost, "/"+strconv.Itoa(created.ID)+"/refresh", nil, s.bundle.AccessToken))
	assert.Equal(t, 2, s.fixture.quotaCalls)
	assert.Equal(t, 1, s.fixture.infoCalls, "quota refresh must not poll slow-changing subscription information")
	assert.Equal(t, created.Quota.SubscriptionPeriodEnd, updated.Quota.SubscriptionPeriodEnd)
}

func TestUpstreamSubscriptionExpiryRefreshRunsWithAutomaticQuotaDisabled(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opencode", "oc_sk_independent")
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Updates(map[string]any{
		"auto_refresh_disabled": true, "next_refresh_at": time.Now().Add(24 * time.Hour).Unix(),
		"subscription_info_next_refresh_at": 0, "subscription_info_last_attempt_at": 0,
	}).Error)
	ids, err := model.DueUpstreamSubscriptionIDs(time.Now().Unix())
	require.NoError(t, err)
	assert.Contains(t, ids, created.ID)
	before := s.fixture.quotaCalls
	updated, err := service.RefreshUpstreamSubscription(context.Background(), created.ID, false)
	require.NoError(t, err)
	assert.Equal(t, before, s.fixture.quotaCalls)
	assert.Equal(t, 2, s.fixture.infoCalls)
	assert.Equal(t, created.LastSuccessAt, updated.LastSuccessAt)
	assert.InDelta(t, time.Now().Add(6*time.Hour).Unix(), updated.SubscriptionInfoNextRefreshAt, 3)
	ids, err = model.DueUpstreamSubscriptionIDs(time.Now().Unix())
	require.NoError(t, err)
	assert.NotContains(t, ids, created.ID)
}

func TestUpstreamSubscriptionExpiryFailurePreservesDateAndBacksOff(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opencode", "oc_sk_transient")
	s.fixture.infoStatus = http.StatusForbidden
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Updates(map[string]any{
		"last_attempt_at": 0, "subscription_info_next_refresh_at": 0, "subscription_info_last_attempt_at": 0,
	}).Error)
	updated, err := service.RefreshUpstreamSubscription(context.Background(), created.ID, true)
	require.NoError(t, err)
	assert.Empty(t, updated.ErrorCode)
	assert.Equal(t, created.Quota.SubscriptionPeriodEnd, updated.Response().Quota.SubscriptionPeriodEnd)
	assert.InDelta(t, time.Now().Add(15*time.Minute).Unix(), updated.SubscriptionInfoNextRefreshAt, 3)
	_, err = service.RefreshUpstreamSubscription(context.Background(), created.ID, true)
	require.NoError(t, err)
	assert.Equal(t, 2, s.fixture.infoCalls, "a failed optional check must not be retried on every quota request")
}

func TestUpstreamSubscriptionRenewalRechecksNearPeriodEndAndClearsEndedAccess(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	end := time.Now().UTC().Add(30 * time.Minute).Truncate(time.Second)
	s.fixture.infoBody = `{"access":{"endsAt":"` + end.Format(time.RFC3339) + `"}}`
	created := s.create(t, "opencode", "oc_sk_renewal")
	stored, err := model.GetUpstreamSubscription(created.ID)
	require.NoError(t, err)
	assert.InDelta(t, time.Now().Add(5*time.Minute).Unix(), stored.SubscriptionInfoNextRefreshAt, 3)
	s.fixture.infoBody = `{"access":null}`
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Updates(map[string]any{
		"subscription_info_next_refresh_at": 0, "subscription_info_last_attempt_at": 0,
	}).Error)
	updated, err := service.RefreshUpstreamSubscription(context.Background(), created.ID, false)
	require.NoError(t, err)
	assert.Nil(t, updated.Response().Quota.SubscriptionPeriodEnd)
	assert.Empty(t, updated.ErrorCode)
}

func TestUpstreamSubscriptionExpiryLeaseAndCredentialRotationRejectOldCompletion(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opencode", "oc_sk_before")
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Updates(map[string]any{
		"subscription_info_next_refresh_at": 0, "subscription_info_last_attempt_at": 0,
	}).Error)
	old, claimed, err := model.ClaimUpstreamSubscriptionInfoRefresh(created.ID, time.Now().Unix())
	require.NoError(t, err)
	require.True(t, claimed)
	_, claimed, err = model.ClaimUpstreamSubscriptionInfoRefresh(created.ID, time.Now().Unix())
	require.NoError(t, err)
	assert.False(t, claimed, "another node must not duplicate an in-flight information request")
	s.fixture.infoBody = `{"access":{"endsAt":"2027-12-12T14:54:44Z"}}`
	rotated := subscriptionResponse(t, s.request(t, http.MethodPut, "/"+strconv.Itoa(created.ID), gin.H{"platform": "opencode", "credential": "oc_sk_after"}, s.bundle.AccessToken))
	oldEnd := time.Now().Add(-time.Hour)
	require.NoError(t, model.FinishUpstreamSubscriptionInfoRefresh(old, model.UpstreamSubscriptionInfo{PeriodEnd: &oldEnd}, true, time.Now().Add(24*time.Hour).Unix()))
	stored, err := model.GetUpstreamSubscription(created.ID)
	require.NoError(t, err)
	assert.Equal(t, rotated.Quota.SubscriptionPeriodEnd, stored.Response().Quota.SubscriptionPeriodEnd)
}

func TestUpstreamSubscriptionScheduleMigrationPreservesExistingConfiguration(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opendesign", "vela_ctrl_upgrade")
	legacySnapshot, err := common.Marshal(created.Quota)
	require.NoError(t, err)
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Update("quota_json", string(legacySnapshot)).Error)
	before, err := model.GetUpstreamSubscription(created.ID)
	require.NoError(t, err)
	for _, column := range []string{"auto_refresh_disabled", "subscription_period_end_at", "subscription_info_last_attempt_at", "subscription_info_next_refresh_at"} {
		require.NoError(t, s.db.Migrator().DropColumn(&model.UpstreamSubscription{}, column))
	}
	reopened, err := gorm.Open(s.db.Dialector, &gorm.Config{})
	require.NoError(t, err)
	s.db, model.DB = reopened, reopened
	t.Cleanup(func() {
		pool, err := reopened.DB()
		require.NoError(t, err)
		require.NoError(t, pool.Close())
	})
	for range 2 {
		require.NoError(t, s.db.AutoMigrate(&model.UpstreamSubscription{}))
	}
	after, err := model.GetUpstreamSubscription(created.ID)
	require.NoError(t, err)
	assert.False(t, after.AutoRefreshDisabled)
	assert.Zero(t, after.SubscriptionInfoNextRefreshAt)
	assert.Equal(t, before.CredentialEncrypted, after.CredentialEncrypted)
	assert.Equal(t, before.QuotaJSON, after.QuotaJSON)
	assert.Equal(t, before.RefreshIntervalMinutes, after.RefreshIntervalMinutes)
	assert.Equal(t, before.LastSuccessAt, after.LastSuccessAt)
	assert.Equal(t, created.Quota.SubscriptionPeriodEnd, after.Response().Quota.SubscriptionPeriodEnd)
	s.fixture.summaryStatus = http.StatusForbidden
	after, err = service.RefreshUpstreamSubscription(context.Background(), created.ID, false)
	require.NoError(t, err)
	assert.Equal(t, created.Quota.SubscriptionPeriodEnd, after.Response().Quota.SubscriptionPeriodEnd, "an unavailable optional check must retain the pre-upgrade expiry")
}

func TestUpstreamSubscriptionWorkspaceChangeDiscardsPersonalExpiryAndOldCompletion(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opendesign", "vela_ctrl_workspace")
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Updates(map[string]any{"subscription_info_next_refresh_at": 0, "subscription_info_last_attempt_at": 0, "last_attempt_at": 0}).Error)
	old, claimed, err := model.ClaimUpstreamSubscriptionInfoRefresh(created.ID, time.Now().Unix())
	require.NoError(t, err)
	require.True(t, claimed)
	s.fixture.workspaceID, s.fixture.workspaceType = "workspace-2", "team"
	s.fixture.quotaBody = strings.ReplaceAll(subscriptionOpenDesignFixture, "workspace-1", "workspace-2")
	updated, err := service.RefreshUpstreamSubscription(context.Background(), created.ID, true)
	require.NoError(t, err)
	assert.Equal(t, "workspace-2", updated.Response().Quota.WorkspaceID)
	assert.Nil(t, updated.Response().Quota.SubscriptionPeriodEnd)
	require.NoError(t, model.FinishUpstreamSubscriptionInfoRefresh(old, model.UpstreamSubscriptionInfo{PeriodEnd: created.Quota.SubscriptionPeriodEnd}, true, time.Now().Add(6*time.Hour).Unix()))
	updated, err = model.GetUpstreamSubscription(created.ID)
	require.NoError(t, err)
	assert.Nil(t, updated.Response().Quota.SubscriptionPeriodEnd)
	assert.Equal(t, 1, s.fixture.summaryCalls, "team quota must never inherit a personal subscription's expiry")
}

func TestUpstreamSubscriptionWorkspaceChangeClearsExpiryWhileAutomaticQuotaIsOff(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opendesign", "vela_ctrl_paused-workspace")
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Updates(map[string]any{
		"auto_refresh_disabled": true, "subscription_info_next_refresh_at": 0, "subscription_info_last_attempt_at": 0,
	}).Error)
	s.fixture.workspaceID, s.fixture.workspaceType = "workspace-2", "team"
	quotaCalls := s.fixture.quotaCalls
	updated, err := service.RefreshUpstreamSubscription(context.Background(), created.ID, false)
	require.NoError(t, err)
	assert.Nil(t, updated.Response().Quota.SubscriptionPeriodEnd)
	assert.Equal(t, quotaCalls, s.fixture.quotaCalls, "independent information refresh must not pull quota while it is disabled")
	assert.Equal(t, created.LastSuccessAt, updated.LastSuccessAt)
	assert.Equal(t, created.Quota.WorkspaceID, updated.Response().Quota.WorkspaceID, "do not relabel the old quota snapshot as another workspace")
}
