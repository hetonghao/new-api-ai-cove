package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const subscriptionOpenCodeFixture = `{"usage":{"rolling":{"status":"ok","percent":1,"resetsAt":"2026-12-15T02:30:17Z"},"weekly":{"status":"ok","percent":4,"resetsAt":"2026-12-16T00:00:00Z"},"monthly":{"status":"ok","percent":2,"resetsAt":"2027-01-12T14:54:44Z"}}}`
const subscriptionOpenDesignFixture = `{"workspaceId":"workspace-1","balanceUsd":"0.1548","codingPlan":{"workspaceId":"workspace-1","eligible":true,"tier":"go","windows":[{"durationSeconds":18000,"usedCredits":"9007199254740993","limitCredits":"18014398509481986","remainingCredits":"9007199254740990","resetsAt":"2026-12-15T01:44:39Z"},{"durationSeconds":604800,"usedCredits":"150002","limitCredits":"150000","remainingCredits":"7","resetsAt":"2026-12-17T11:44:39Z"}]}}`

type subscriptionAPIFixture struct {
	status        int
	quotaBody     string
	fallback      bool
	loginStatus   string
	activationURL string
	quotaCalls    int
}

func (f *subscriptionAPIFixture) respond(t *testing.T, request *http.Request) *http.Response {
	t.Helper()
	status, body := http.StatusOK, subscriptionOpenCodeFixture
	path := request.URL.Path
	switch {
	case strings.HasSuffix(path, "/token"):
		body = fmt.Sprintf(`{"status":%q}`, f.loginStatus)
		if f.loginStatus == "approved" {
			body = `{"status":"approved","controlKey":"vela_ctrl_authorized","runtimeKey":"sk-amr-never-store"}`
		}
	case path == "/api/v1/cli/device-authorizations":
		activation := f.activationURL
		if activation == "" {
			activation = "https://open-design.ai/cloud/cli/activate?deviceId=device-1&userCode=ABCD-EFGH"
		}
		encoded, err := common.Marshal(gin.H{"deviceId": "device-1", "deviceSecret": "private-device-proof", "userCode": "ABCD-EFGH", "activationUrl": activation, "expiresAt": time.Now().Add(10 * time.Minute), "pollIntervalSeconds": 2, "status": "pending"})
		require.NoError(t, err)
		body = string(encoded)
	case path == "/api/v1/workspaces/current":
		require.True(t, strings.HasPrefix(request.Header.Get("Authorization"), "Bearer vela_ctrl_"))
		body = `{"workspaceId":"workspace-1","workspaceName":"Quota workspace"}`
	case strings.HasSuffix(path, "/billing/preflight"):
		f.quotaCalls++
		body = subscriptionOpenDesignFixture
		if f.quotaBody != "" {
			body = f.quotaBody
		}
		if f.status != 0 {
			status = f.status
		}
	case strings.HasSuffix(path, "/go/v1/usage"):
		f.quotaCalls++
		require.True(t, strings.HasPrefix(request.Header.Get("Authorization"), "Bearer oc_sk_"))
		if f.quotaBody != "" {
			body = f.quotaBody
		}
		if f.status != 0 {
			status = f.status
		}
		if f.fallback && strings.HasPrefix(path, "/inference/") {
			status = http.StatusNotFound
		}
	default:
		t.Fatalf("unexpected outbound request: %s %s", request.Method, path)
	}
	return &http.Response{StatusCode: status, Header: http.Header{"Retry-After": []string{"600"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}
}

type subscriptionControllerTest struct {
	db      *gorm.DB
	router  *gin.Engine
	root    *model.User
	bundle  *service.AuthBundle
	fixture *subscriptionAPIFixture
}

func setupSubscriptionControllerTest(t *testing.T) *subscriptionControllerTest {
	t.Helper()
	t.Setenv("TEST_MANAGE_USER_DIALECT", os.Getenv("TEST_SUBSCRIPTION_DIALECT"))
	db := setupManageUserTestDB(t)
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "subscription-controller-test-secret"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })
	root := &model.User{Username: "quota-operator", Password: "existing-password-hash", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(root).Error)
	bundle, err := service.CreateLoginSession(root.Id, "password", "127.0.0.1", "subscription-test")
	require.NoError(t, err)
	// Adding the table to a populated pre-feature schema must be repeatable.
	require.NoError(t, db.AutoMigrate(&model.UpstreamSubscription{}))
	require.NoError(t, db.AutoMigrate(&model.UpstreamSubscription{}))
	fixture := &subscriptionAPIFixture{loginStatus: "pending"}
	service.InitHttpClient()
	shared := service.GetHttpClient()
	previousTransport := shared.Transport
	shared.Transport = riskProviderControllerRoundTripFunc(func(request *http.Request) (*http.Response, error) { return fixture.respond(t, request), nil })
	t.Cleanup(func() { shared.Transport = previousTransport })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/upstream-subscriptions", middleware.RootAuth())
	group.GET("", ListUpstreamSubscriptions)
	group.POST("", SaveUpstreamSubscription)
	group.PUT("/:id", SaveUpstreamSubscription)
	group.DELETE("/:id", DeleteUpstreamSubscription)
	group.POST("/:id/refresh", RefreshUpstreamSubscription)
	group.POST("/opendesign/login", StartOpenDesignSubscriptionLogin)
	group.POST("/opendesign/login/poll", PollOpenDesignSubscriptionLogin)
	group.POST("/opendesign/login/cancel", PollOpenDesignSubscriptionLogin)
	return &subscriptionControllerTest{db: db, router: router, root: root, bundle: bundle, fixture: fixture}
}

func (s *subscriptionControllerTest) request(t *testing.T, method, path string, payload any, token string) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := common.Marshal(payload)
	require.NoError(t, err)
	request := httptest.NewRequest(method, "/api/upstream-subscriptions"+path, strings.NewReader(string(encoded)))
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("New-Api-User", strconv.Itoa(s.root.Id))
	}
	recorder := httptest.NewRecorder()
	s.router.ServeHTTP(recorder, request)
	return recorder
}

func subscriptionResponse(t *testing.T, response *httptest.ResponseRecorder) model.UpstreamSubscriptionResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body struct {
		Success bool                               `json:"success"`
		Data    model.UpstreamSubscriptionResponse `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
	require.True(t, body.Success, response.Body.String())
	return body.Data
}

func (s *subscriptionControllerTest) create(t *testing.T, platform, credential string) model.UpstreamSubscriptionResponse {
	t.Helper()
	return subscriptionResponse(t, s.request(t, http.MethodPost, "", gin.H{"platform": platform, "credential": credential}, s.bundle.AccessToken))
}

func (s *subscriptionControllerTest) challenge(t *testing.T) string {
	t.Helper()
	response := s.request(t, http.MethodPost, "/opendesign/login", gin.H{"name": "Authorized account", "refresh_interval_minutes": 17}, s.bundle.AccessToken)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.NotContains(t, response.Body.String(), "private-device-proof")
	var body struct {
		Data struct {
			Token string `json:"flow_token"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
	return body.Data.Token
}

func (s *subscriptionControllerTest) allowPoll(t *testing.T, token string) {
	t.Helper()
	flow, err := model.GetAuthFlow(token, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeSubscription})
	require.NoError(t, err)
	decoded, err := common.DecryptCredential(flow.Payload)
	require.NoError(t, err)
	var payload subscriptionLoginPayload
	require.NoError(t, common.UnmarshalJsonStr(decoded, &payload))
	payload.NextPollAt = 0
	encoded, err := common.Marshal(payload)
	require.NoError(t, err)
	encrypted, err := common.EncryptCredential(string(encoded))
	require.NoError(t, err)
	require.NoError(t, s.db.Model(flow).Update("payload", encrypted).Error)
}

func TestUpstreamSubscriptionAPICreatesEncryptedCredentialsAndPreservesMigrationData(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	s.fixture.fallback = true
	created := s.create(t, "opencode", "oc_sk_subscription")
	require.NotNil(t, created.Quota)
	require.Len(t, created.Quota.Windows, 3)
	assert.Equal(t, []string{"rolling", "weekly", "monthly"}, []string{created.Quota.Windows[0].Key, created.Quota.Windows[1].Key, created.Quota.Windows[2].Key})
	assert.Equal(t, 4.0, *created.Quota.Windows[1].UsedPercent)
	assert.Equal(t, "OpenCode Go", created.Name)
	assert.Equal(t, 10, created.RefreshIntervalMinutes)
	assert.Equal(t, created.LastAttemptAt+600, created.NextRefreshAt)
	stored, err := model.GetUpstreamSubscription(created.ID)
	require.NoError(t, err)
	assert.NotContains(t, stored.CredentialEncrypted, "oc_sk_subscription")
	response := s.request(t, http.MethodGet, "", nil, s.bundle.AccessToken)
	assert.NotContains(t, response.Body.String(), "oc_sk_subscription")
	assert.NotContains(t, response.Body.String(), "credential_encrypted")
	for range 2 {
		require.NoError(t, s.db.AutoMigrate(&model.UpstreamSubscription{}))
	}
	updated := subscriptionResponse(t, s.request(t, http.MethodPut, "/"+strconv.Itoa(created.ID), gin.H{"name": "Renamed", "platform": "opencode", "credential": ""}, s.bundle.AccessToken))
	assert.Equal(t, "Renamed", updated.Name)
	assert.Equal(t, 4.0, *updated.Quota.Windows[1].UsedPercent)
	var root model.User
	require.NoError(t, s.db.First(&root, s.root.Id).Error)
	assert.Equal(t, "existing-password-hash", root.Password)
	assert.Equal(t, common.RoleRootUser, root.Role)
	assert.True(t, s.db.Migrator().HasIndex(&model.UpstreamSubscription{}, "NextRefreshAt"))
	assert.Equal(t, http.StatusOK, s.request(t, http.MethodDelete, "/"+strconv.Itoa(created.ID), nil, s.bundle.AccessToken).Code)
}

func TestUpstreamSubscriptionAPIKeepsExactCreditsAndProviderRemaining(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opendesign", "vela_ctrl_subscription")
	require.NotNil(t, created.Quota)
	require.Len(t, created.Quota.Windows, 2)
	assert.Equal(t, "9007199254740993", created.Quota.Windows[0].UsedCredits)
	assert.Equal(t, "9007199254740990", created.Quota.Windows[0].RemainingCredits)
	assert.Equal(t, 50.0, *created.Quota.Windows[0].UsedPercent)
	assert.Equal(t, "7", created.Quota.Windows[1].RemainingCredits)
	assert.InDelta(t, 100.0013333333, *created.Quota.Windows[1].UsedPercent, 0.00000001)
	assert.Equal(t, "0.1548", *created.Quota.BalanceUSD)
}

func TestUpstreamSubscriptionAPIDiscardsAnOldCredentialRefreshAfterRotation(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opencode", "oc_sk_previous-account")
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Update("last_attempt_at", 0).Error)
	old, claimed, err := model.ClaimUpstreamSubscriptionRefresh(created.ID, time.Now().Unix(), true)
	require.NoError(t, err)
	require.True(t, claimed)
	s.fixture.quotaBody = strings.Replace(subscriptionOpenCodeFixture, `"percent":1`, `"percent":9`, 1)
	updated := subscriptionResponse(t, s.request(t, http.MethodPut, "/"+strconv.Itoa(created.ID), gin.H{"name": "New account", "platform": "opencode", "credential": "oc_sk_new-account"}, s.bundle.AccessToken))
	require.NotNil(t, updated.Quota)
	assert.Equal(t, 9.0, *updated.Quota.Windows[0].UsedPercent)
	require.NoError(t, model.FinishUpstreamSubscriptionRefresh(old, created.Quota, "", time.Now().Add(time.Hour).Unix()))
	current, err := model.GetUpstreamSubscription(created.ID)
	require.NoError(t, err)
	assert.Equal(t, 9.0, *current.Response().Quota.Windows[0].UsedPercent)
}

func TestUpstreamSubscriptionAPIFailuresPreserveLastSnapshot(t *testing.T) {
	for _, test := range []struct {
		name, platform, code, body string
		status                     int
	}{
		{name: "expired credential", platform: "opencode", status: 401, code: "unauthorized"},
		{name: "missing entitlement", platform: "opencode", status: 403, code: "forbidden"},
		{name: "both routes removed", platform: "opencode", status: 404, code: "endpoint_changed"},
		{name: "rate limit", platform: "opencode", status: 429, code: "rate_limited"},
		{name: "missing percentage", platform: "opencode", body: `{"usage":{"rolling":{"status":"ok"}}}`, code: "invalid_response"},
		{name: "workspace mismatch", platform: "opendesign", body: strings.Replace(subscriptionOpenDesignFixture, `"workspaceId":"workspace-1"`, `"workspaceId":"wrong-workspace"`, 1), code: "invalid_response"},
		{name: "duplicate window", platform: "opendesign", body: strings.Replace(subscriptionOpenDesignFixture, `604800`, `18000`, 1), code: "invalid_response"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := setupSubscriptionControllerTest(t)
			credential := "oc_sk_subscription"
			if test.platform == "opendesign" {
				credential = "vela_ctrl_subscription"
			}
			created := s.create(t, test.platform, credential)
			require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Updates(map[string]any{"last_attempt_at": 0, "next_refresh_at": 0}).Error)
			s.fixture.status, s.fixture.quotaBody = test.status, test.body
			updated := subscriptionResponse(t, s.request(t, http.MethodPost, "/"+strconv.Itoa(created.ID)+"/refresh", nil, s.bundle.AccessToken))
			assert.Equal(t, test.code, updated.ErrorCode)
			assert.True(t, updated.Stale)
			assert.Equal(t, created.Quota, updated.Quota)
			if test.status == 429 {
				assert.Equal(t, updated.LastAttemptAt+600, updated.NextRefreshAt)
			}
			calls := s.fixture.quotaCalls
			require.Equal(t, http.StatusOK, s.request(t, http.MethodPost, "/"+strconv.Itoa(created.ID)+"/refresh", nil, s.bundle.AccessToken).Code)
			assert.Equal(t, calls, s.fixture.quotaCalls, "immediate retries must not contact the provider")
		})
	}
}

func TestUpstreamSubscriptionAPIRejectsInferenceKeysAndUnauthenticatedAccess(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	assert.Equal(t, http.StatusUnauthorized, s.request(t, http.MethodGet, "", nil, "").Code)
	for _, platform := range []string{"opencode", "opendesign", "unsupported"} {
		response := s.request(t, http.MethodPost, "", gin.H{"platform": platform, "credential": "sk-amr-inference"}, s.bundle.AccessToken)
		assert.Equal(t, http.StatusBadRequest, response.Code)
		var failure struct {
			MessageKey string `json:"message_key"`
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &failure))
		expectedKey := "Use the platform subscription credential, not an inference key"
		if platform == "unsupported" {
			expectedKey = "Unsupported subscription platform"
		}
		assert.Equal(t, expectedKey, failure.MessageKey)
	}
	assert.Zero(t, s.fixture.quotaCalls)
}

func TestUpstreamSubscriptionLoginIsBoundToSessionAndConsumedOnce(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	token := s.challenge(t)
	flow, err := model.GetAuthFlow(token, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeSubscription})
	require.NoError(t, err)
	assert.NotContains(t, flow.Payload, "private-device-proof")
	assert.NotEqual(t, token, flow.TokenHash)
	secondSession, err := service.CreateLoginSession(s.root.Id, "password", "127.0.0.1", "other-session")
	require.NoError(t, err)
	assert.Equal(t, http.StatusGone, s.request(t, http.MethodPost, "/opendesign/login/poll", gin.H{"flow_token": token}, secondSession.AccessToken).Code)
	s.allowPoll(t, token)
	s.fixture.loginStatus = "approved"
	approved := s.request(t, http.MethodPost, "/opendesign/login/poll", gin.H{"flow_token": token}, s.bundle.AccessToken)
	require.Equal(t, http.StatusOK, approved.Code, approved.Body.String())
	assert.Contains(t, approved.Body.String(), `"status":"approved"`)
	assert.NotContains(t, approved.Body.String(), "vela_ctrl_authorized")
	assert.NotContains(t, approved.Body.String(), "sk-amr-never-store")
	assert.Equal(t, http.StatusGone, s.request(t, http.MethodPost, "/opendesign/login/poll", gin.H{"flow_token": token}, s.bundle.AccessToken).Code)
	subscriptions, err := model.ListUpstreamSubscriptions()
	require.NoError(t, err)
	require.Len(t, subscriptions, 1)
	assert.Equal(t, "Authorized account", subscriptions[0].Name)
	assert.Equal(t, 17, subscriptions[0].RefreshIntervalMinutes)
}

func TestUpstreamSubscriptionRefreshIntervalCompletionUsesLatestCadence(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opencode", "oc_sk_subscription")
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Update("last_attempt_at", 0).Error)
	old, claimed, err := model.ClaimUpstreamSubscriptionRefresh(created.ID, time.Now().Unix(), true)
	require.NoError(t, err)
	require.True(t, claimed)
	updated := subscriptionResponse(t, s.request(t, http.MethodPut, "/"+strconv.Itoa(created.ID), gin.H{"name": "Configured", "platform": "opencode", "refresh_interval_minutes": 30}, s.bundle.AccessToken))
	assert.Equal(t, 30, updated.RefreshIntervalMinutes)
	assert.Equal(t, old.LastAttemptAt+1800, updated.NextRefreshAt)
	require.NoError(t, model.FinishUpstreamSubscriptionRefresh(old, created.Quota, "", old.LastAttemptAt+600))
	stored, err := model.GetUpstreamSubscription(created.ID)
	require.NoError(t, err)
	assert.Equal(t, old.LastAttemptAt+1800, stored.NextRefreshAt)
	stored.LastSuccessAt = time.Now().Add(-40 * time.Minute).Unix()
	assert.False(t, stored.Response().Stale, "staleness follows the configured cadence")
}

func TestUpstreamSubscriptionRefreshIntervalHonorsProviderRetryAndBounds(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	for _, minutes := range []int{-1, 1441} {
		response := s.request(t, http.MethodPost, "", gin.H{"platform": "opencode", "credential": "oc_sk_subscription", "refresh_interval_minutes": minutes}, s.bundle.AccessToken)
		assert.Equal(t, http.StatusBadRequest, response.Code)
	}
	assert.Zero(t, s.fixture.quotaCalls)
	created := s.create(t, "opencode", "oc_sk_subscription")
	retryAt := time.Now().Add(time.Hour).Unix()
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Updates(map[string]any{"error_code": "rate_limited", "next_refresh_at": retryAt}).Error)
	updated := subscriptionResponse(t, s.request(t, http.MethodPut, "/"+strconv.Itoa(created.ID), gin.H{"platform": "opencode", "refresh_interval_minutes": 1}, s.bundle.AccessToken))
	assert.Equal(t, 1, updated.RefreshIntervalMinutes)
	assert.Equal(t, retryAt, updated.NextRefreshAt)
}

func TestUpstreamSubscriptionRefreshIntervalMigrationPreservesExistingConnection(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opencode", "oc_sk_subscription")
	before, err := model.GetUpstreamSubscription(created.ID)
	require.NoError(t, err)
	require.NoError(t, s.db.Migrator().DropColumn(&model.UpstreamSubscription{}, "refresh_interval_minutes"))
	// Startup uses a fresh pool; PostgreSQL caches SELECT * result shapes per connection.
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
	assert.Equal(t, 10, after.RefreshIntervalMinutes)
	assert.Equal(t, before.CredentialEncrypted, after.CredentialEncrypted)
	assert.Equal(t, before.QuotaJSON, after.QuotaJSON)
	assert.Equal(t, before.Name, after.Name)
}

func TestUpstreamSubscriptionRefreshIntervalChangeKeepsInflightRateLimit(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opencode", "oc_sk_subscription")
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Update("last_attempt_at", 0).Error)
	old, claimed, err := model.ClaimUpstreamSubscriptionRefresh(created.ID, time.Now().Unix(), true)
	require.NoError(t, err)
	require.True(t, claimed)
	subscriptionResponse(t, s.request(t, http.MethodPut, "/"+strconv.Itoa(created.ID), gin.H{"platform": "opencode", "refresh_interval_minutes": 1}, s.bundle.AccessToken))
	retryAt := old.LastAttemptAt + 3600
	require.NoError(t, model.FinishUpstreamSubscriptionRefresh(old, nil, "rate_limited", retryAt))
	stored, err := model.GetUpstreamSubscription(created.ID)
	require.NoError(t, err)
	assert.Equal(t, "rate_limited", stored.ErrorCode)
	assert.Equal(t, retryAt, stored.NextRefreshAt)
	_, claimed, err = model.ClaimUpstreamSubscriptionRefresh(created.ID, old.LastAttemptAt+61, true)
	require.NoError(t, err)
	assert.False(t, claimed, "manual refresh cannot bypass an in-flight Retry-After")
}

func TestUpstreamSubscriptionLoginRejectsCancelledExpiredAndForeignActivation(t *testing.T) {
	for _, state := range []string{"cancelled", "expired", "foreign activation"} {
		t.Run(state, func(t *testing.T) {
			s := setupSubscriptionControllerTest(t)
			if state == "foreign activation" {
				s.fixture.activationURL = "https://untrusted.example/cloud/cli/activate?deviceId=device-1&userCode=ABCD-EFGH"
				assert.Equal(t, http.StatusBadGateway, s.request(t, http.MethodPost, "/opendesign/login", gin.H{}, s.bundle.AccessToken).Code)
				return
			}
			token := s.challenge(t)
			if state == "cancelled" {
				require.Equal(t, http.StatusOK, s.request(t, http.MethodPost, "/opendesign/login/cancel", gin.H{"flow_token": token}, s.bundle.AccessToken).Code)
			} else {
				flow, err := model.GetAuthFlow(token, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeSubscription})
				require.NoError(t, err)
				require.NoError(t, s.db.Model(flow).Update("expires_at", time.Now().Add(-time.Minute)).Error)
			}
			s.fixture.loginStatus = "approved"
			assert.Equal(t, http.StatusGone, s.request(t, http.MethodPost, "/opendesign/login/poll", gin.H{"flow_token": token}, s.bundle.AccessToken).Code)
			subscriptions, err := model.ListUpstreamSubscriptions()
			require.NoError(t, err)
			assert.Empty(t, subscriptions)
		})
	}
}
