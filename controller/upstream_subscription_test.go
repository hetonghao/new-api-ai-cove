package controller

import (
	"context"
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
const subscriptionCommandCodeFixture = `{"credits":{"monthlyCredits":68.6,"purchasedCredits":0},"windowLimits":{"limited":true,"fiveHour":{"used":1.4,"cap":14,"exceeded":false,"resetAt":1794764000392},"weekly":{"used":31.5,"cap":35,"exceeded":false,"resetAt":1795350800392}}}`
const subscriptionCommandCodeKey = "opaque_test.key+123"

type subscriptionAPIFixture struct {
	status        int
	quotaBody     string
	fallback      bool
	loginStatus   string
	activationURL string
	quotaCalls    int
	summaryBody   string
	summaryStatus int
	workspaceType string
	workspaceID   string
	summaryCalls  int
	infoBody      string
	infoStatus    int
	infoCalls     int
	commandOrgID  string
}

func (f *subscriptionAPIFixture) respond(t *testing.T, request *http.Request) *http.Response {
	t.Helper()
	status, body := http.StatusOK, subscriptionOpenCodeFixture
	path := request.URL.Path
	assert.Equal(t, "AI-Cove/1.0", request.Header.Get("User-Agent"))
	switch {
	case path == "/alpha/whoami":
		assert.Equal(t, "api.commandcode.ai", request.URL.Host)
		assert.Equal(t, "1", request.URL.Query().Get("limits"))
		assert.Equal(t, "Bearer "+subscriptionCommandCodeKey, request.Header.Get("Authorization"))
		body = `{"success":true,"user":{"id":"person-1"},"org":null}`
		if f.commandOrgID != "" {
			body = fmt.Sprintf(`{"success":true,"user":{"id":"person-1"},"org":{"id":%q}}`, f.commandOrgID)
		}
	case path == "/alpha/billing/credits" || path == "/alpha/billing/subscriptions":
		assert.Equal(t, "api.commandcode.ai", request.URL.Host)
		assert.Equal(t, f.commandOrgID, request.URL.Query().Get("orgId"))
		assert.Equal(t, "Bearer "+subscriptionCommandCodeKey, request.Header.Get("Authorization"))
		assert.Empty(t, request.Header.Get("Cookie"))
		if path == "/alpha/billing/credits" {
			f.quotaCalls++
			body = subscriptionCommandCodeFixture
			if f.quotaBody != "" {
				body = f.quotaBody
			}
			if f.status != 0 {
				status = f.status
			}
		} else {
			f.infoCalls++
			body = `{"success":true,"data":{"planId":"individual-goat","quantity":1,"status":"active","currentPeriodEnd":"2026-12-15T12:24:45.000Z"}}`
			if f.infoBody != "" {
				body = f.infoBody
			}
			if f.infoStatus != 0 {
				status = f.infoStatus
			}
		}
	case path == "/console/api/go/status":
		f.infoCalls++
		assert.Equal(t, "", request.Header.Get("Cookie"))
		assert.Equal(t, "", request.Header.Get("x-org-id"))
		body = `{"access":{"startsAt":"2026-11-12T14:54:44Z","endsAt":"2026-12-12T14:54:44Z"}}`
		if f.infoBody != "" {
			body = f.infoBody
		}
		if f.infoStatus != 0 {
			status = f.infoStatus
		}
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
		workspaceType := f.workspaceType
		if workspaceType == "" {
			workspaceType = "personal"
		}
		workspaceID := f.workspaceID
		if workspaceID == "" {
			workspaceID = "workspace-1"
		}
		body = fmt.Sprintf(`{"workspaceId":%q,"workspaceName":"Quota workspace","workspaceType":%q}`, workspaceID, workspaceType)
	case path == "/api/v1/billing/summary":
		f.summaryCalls++
		body = `{"subscriptionCurrentPeriodEnd":"2027-11-11T05:50:51Z"}`
		if f.summaryBody != "" {
			body = f.summaryBody
		}
		if f.summaryStatus != 0 {
			status = f.summaryStatus
		}
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
	group.GET("/channels", ListUpstreamSubscriptionChannels)
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

func TestUpstreamSubscriptionCommandCodeAcceptsPersonalAndOrganizationKeys(t *testing.T) {
	for _, orgID := range []string{"", "org-test+one"} {
		t.Run("organization="+orgID, func(t *testing.T) {
			s := setupSubscriptionControllerTest(t)
			s.fixture.commandOrgID = orgID
			response := s.request(t, http.MethodPost, "", gin.H{"platform": "commandcode", "credential": subscriptionCommandCodeKey}, s.bundle.AccessToken)
			created := subscriptionResponse(t, response)
			assert.NotContains(t, response.Body.String(), subscriptionCommandCodeKey)
			assert.Equal(t, "CommandCode", created.Name)
			assert.Empty(t, created.ErrorCode)
			require.NotNil(t, created.Quota)
			require.Len(t, created.Quota.Windows, 3)
			assert.InDelta(t, 10.0, *created.Quota.Windows[0].UsedPercent, 0.000001)
			assert.InDelta(t, 90.0, *created.Quota.Windows[1].UsedPercent, 0.000001)
			assert.InDelta(t, 2.0, *created.Quota.Windows[2].UsedPercent, 0.000001)
			assert.Equal(t, created.Quota.SubscriptionPeriodEnd, created.Quota.Windows[2].ResetsAt)
			assert.Equal(t, int64(1794764000392), created.Quota.Windows[0].ResetsAt.UnixMilli())
			require.NotNil(t, created.Quota.SubscriptionPeriodEnd)
			assert.Equal(t, "2026-12-15T12:24:45Z", created.Quota.SubscriptionPeriodEnd.Format(time.RFC3339))
			stored, err := model.GetUpstreamSubscription(created.ID)
			require.NoError(t, err)
			assert.NotContains(t, stored.CredentialEncrypted, subscriptionCommandCodeKey)
			credential, err := common.DecryptCredential(stored.CredentialEncrypted)
			require.NoError(t, err)
			assert.Equal(t, subscriptionCommandCodeKey, credential)
		})
	}
}

func TestUpstreamSubscriptionCommandCodeHandlesUnusedAndUnavailableWindows(t *testing.T) {
	for _, test := range []struct {
		name, window string
		percent      *float64
		status       string
	}{
		{name: "not started", window: `{"used":0,"cap":14,"resetAt":0}`, percent: new(0.0), status: "ok"},
		{name: "no cap", window: `{"used":0,"cap":0,"resetAt":null}`, status: "unavailable"},
		{name: "provider exhausted", window: `{"used":13.99,"cap":14,"exceeded":true,"resetAt":null}`, percent: new(100.0), status: "ok"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := setupSubscriptionControllerTest(t)
			s.fixture.quotaBody = `{"windowLimits":{"fiveHour":` + test.window + `}}`
			created := s.create(t, "commandcode", subscriptionCommandCodeKey)
			assert.Empty(t, created.ErrorCode)
			require.NotNil(t, created.Quota)
			require.Len(t, created.Quota.Windows, 1)
			assert.Equal(t, test.percent, created.Quota.Windows[0].UsedPercent)
			assert.Equal(t, test.status, created.Quota.Windows[0].Status)
			assert.Nil(t, created.Quota.Windows[0].ResetsAt)
		})
	}
}

func TestUpstreamSubscriptionCommandCodeExpiryIsCachedAndRunsWithQuotaDisabled(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "commandcode", subscriptionCommandCodeKey)
	s.fixture.quotaBody = strings.Replace(subscriptionCommandCodeFixture, `68.6`, `35`, 1)
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Update("last_attempt_at", 0).Error)
	refreshed, err := service.RefreshUpstreamSubscription(context.Background(), created.ID, true)
	require.NoError(t, err)
	assert.Equal(t, 2, s.fixture.quotaCalls)
	assert.Equal(t, 1, s.fixture.infoCalls)
	assert.Equal(t, created.Quota.SubscriptionPeriodEnd, refreshed.Response().Quota.SubscriptionPeriodEnd)
	require.Len(t, refreshed.Response().Quota.Windows, 3)
	assert.Equal(t, 50.0, *refreshed.Response().Quota.Windows[2].UsedPercent)

	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Updates(map[string]any{
		"auto_refresh_disabled": true, "subscription_info_next_refresh_at": 0, "subscription_info_last_attempt_at": 0,
	}).Error)
	s.fixture.infoStatus = http.StatusServiceUnavailable
	refreshed, err = service.RefreshUpstreamSubscription(context.Background(), created.ID, false)
	require.NoError(t, err)
	assert.Equal(t, 2, s.fixture.quotaCalls)
	assert.Equal(t, created.Quota.SubscriptionPeriodEnd, refreshed.Response().Quota.SubscriptionPeriodEnd)
	assert.InDelta(t, time.Now().Add(15*time.Minute).Unix(), refreshed.SubscriptionInfoNextRefreshAt, 3)

	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Updates(map[string]any{
		"subscription_info_next_refresh_at": 0, "subscription_info_last_attempt_at": 0,
	}).Error)
	s.fixture.infoStatus = 0
	s.fixture.infoBody = `{"success":true,"data":null}`
	refreshed, err = service.RefreshUpstreamSubscription(context.Background(), created.ID, false)
	require.NoError(t, err)
	assert.Equal(t, 2, s.fixture.quotaCalls)
	assert.Nil(t, refreshed.Response().Quota.SubscriptionPeriodEnd)
	assert.Len(t, refreshed.Response().Quota.Windows, 2)
}

func TestUpstreamSubscriptionCommandCodeMonthlyWindowUsesStudioPlanAndGrantRules(t *testing.T) {
	for _, test := range []struct {
		name, plan, status, credits, end string
		quantity                         int
		percent                          *float64
	}{
		{name: "advertised 100 is not the goat base pool", plan: "individual-goat", credits: `{"monthlyCredits":35}`, percent: new(50.0)},
		{name: "dynamic grant exceeds base", plan: "individual-goat", credits: `{"monthlyCredits":99,"monthlyCreditsGranted":100}`, percent: new(1.0)},
		{name: "smaller grant retains plan base", plan: "individual-goat", credits: `{"monthlyCredits":35,"monthlyCreditsGranted":30}`, percent: new(50.0)},
		{name: "organization seats without grant", plan: "teams-pro", quantity: 3, credits: `{"monthlyCredits":60}`, percent: new(50.0)},
		{name: "grant already covers organization seats", plan: "teams-pro", quantity: 3, credits: `{"monthlyCredits":60,"monthlyCreditsGranted":100}`, percent: new(40.0)},
		{name: "legacy pro retains its own pool", plan: "individual-pro", credits: `{"monthlyCredits":15}`, percent: new(50.0)},
		{name: "current pro has a different pool", plan: "individual-pro-v1", credits: `{"monthlyCredits":40}`, percent: new(50.0)},
		{name: "unknown plan stays unknown despite grant", plan: "future-plan", credits: `{"monthlyCredits":60,"monthlyCreditsGranted":100}`},
		{name: "past due hides monthly allowance", plan: "individual-goat", status: "past_due", credits: `{"monthlyCredits":35}`},
		{name: "unknown reset does not hide known allowance", plan: "individual-goat", credits: `{"monthlyCredits":35}`, end: "null", percent: new(50.0)},
		{name: "negative remaining is exhausted", plan: "individual-goat", credits: `{"monthlyCredits":-1}`, percent: new(100.0)},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := setupSubscriptionControllerTest(t)
			status, end := test.status, test.end
			if status == "" {
				status = "active"
			}
			if end == "" {
				end = `"2026-12-15T12:24:45Z"`
			}
			s.fixture.infoBody = fmt.Sprintf(`{"success":true,"data":{"planId":%q,"status":%q,"quantity":%d,"currentPeriodEnd":%s}}`, test.plan, status, max(1, test.quantity), end)
			s.fixture.quotaBody = `{"credits":` + test.credits + `,"windowLimits":{"fiveHour":{"used":0,"cap":14,"resetAt":0}}}`
			created := s.create(t, "commandcode", subscriptionCommandCodeKey)
			assert.Empty(t, created.ErrorCode)
			require.NotNil(t, created.Quota)
			if test.percent == nil {
				assert.Len(t, created.Quota.Windows, 1)
				return
			}
			require.Len(t, created.Quota.Windows, 2)
			monthly := created.Quota.Windows[1]
			assert.Equal(t, "monthly", monthly.Key)
			require.NotNil(t, monthly.UsedPercent)
			assert.InDelta(t, *test.percent, *monthly.UsedPercent, 0.000001)
			assert.Equal(t, created.Quota.SubscriptionPeriodEnd, monthly.ResetsAt)
		})
	}
}

func TestUpstreamSubscriptionCommandCodeRejectsMalformedKeysBeforeContactingPlatform(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	for _, credential := range []string{"", "opaque invalid-key", "opaque\x01key", strings.Repeat("a", 4097)} {
		response := s.request(t, http.MethodPost, "", gin.H{"platform": "commandcode", "credential": credential}, s.bundle.AccessToken)
		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.Contains(t, response.Body.String(), `"message_key":"Use a valid CommandCode API key"`)
	}
	assert.Zero(t, s.fixture.quotaCalls)
	assert.Zero(t, s.fixture.infoCalls)
}

func TestUpstreamSubscriptionCommandCodeInflightQuotaCannotOverwriteNewPlanMetadata(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "commandcode", subscriptionCommandCodeKey)
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Updates(map[string]any{
		"last_attempt_at": 0, "subscription_info_next_refresh_at": 0, "subscription_info_last_attempt_at": 0,
	}).Error)
	old, claimed, err := model.ClaimUpstreamSubscriptionRefresh(created.ID, time.Now().Unix(), true)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Update("auto_refresh_disabled", true).Error)
	s.fixture.infoBody = `{"success":true,"data":{"planId":"individual-pro-v1","status":"active","currentPeriodEnd":"2026-12-15T12:24:45Z"}}`
	updated, err := service.RefreshUpstreamSubscription(context.Background(), created.ID, false)
	require.NoError(t, err)
	assert.Equal(t, "individual-pro-v1", updated.Response().Quota.Tier)
	s.fixture.quotaBody = strings.Replace(subscriptionCommandCodeFixture, `68.6`, `35`, 1)
	quota, err := service.FetchSubscriptionQuota(context.Background(), "commandcode", subscriptionCommandCodeKey)
	require.NoError(t, err)
	require.NoError(t, model.FinishUpstreamSubscriptionRefresh(old, quota, "", time.Now().Add(time.Hour).Unix()))
	stored, err := model.GetUpstreamSubscription(created.ID)
	require.NoError(t, err)
	assert.Equal(t, "individual-pro-v1", stored.Response().Quota.Tier)
	assert.InDelta(t, 14.25, *stored.Response().Quota.Windows[2].UsedPercent, 0.000001)
}

func TestUpstreamSubscriptionCommandCodeInvalidPeriodDoesNotDiscardCachedExpiry(t *testing.T) {
	for _, body := range []string{`{"success":false,"data":null}`, `{"success":true,"data":{}}`, `{"success":true,"data":{"currentPeriodEnd":"not-a-date"}}`} {
		t.Run(body, func(t *testing.T) {
			s := setupSubscriptionControllerTest(t)
			created := s.create(t, "commandcode", subscriptionCommandCodeKey)
			require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Updates(map[string]any{
				"auto_refresh_disabled": true, "subscription_info_next_refresh_at": 0, "subscription_info_last_attempt_at": 0,
			}).Error)
			s.fixture.infoBody = body
			updated, err := service.RefreshUpstreamSubscription(context.Background(), created.ID, false)
			require.NoError(t, err)
			assert.Equal(t, created.Quota.SubscriptionPeriodEnd, updated.Response().Quota.SubscriptionPeriodEnd)
			assert.Empty(t, updated.ErrorCode)
			assert.InDelta(t, time.Now().Add(15*time.Minute).Unix(), updated.SubscriptionInfoNextRefreshAt, 3)
		})
	}
}

func TestUpstreamSubscriptionCommandCodeScopeChangeClearsExpiryWhileQuotaIsDisabled(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "commandcode", subscriptionCommandCodeKey)
	require.NoError(t, s.db.Model(&model.UpstreamSubscription{}).Where("id = ?", created.ID).Updates(map[string]any{
		"auto_refresh_disabled": true, "subscription_info_next_refresh_at": 0, "subscription_info_last_attempt_at": 0,
	}).Error)
	s.fixture.commandOrgID = "org-other"
	updated, err := service.RefreshUpstreamSubscription(context.Background(), created.ID, false)
	require.NoError(t, err)
	assert.Nil(t, updated.Response().Quota.SubscriptionPeriodEnd)
	assert.Equal(t, created.Quota.WorkspaceID, updated.Response().Quota.WorkspaceID)
	assert.Equal(t, 1, s.fixture.quotaCalls)
	assert.Equal(t, 1, s.fixture.infoCalls, "a new billing scope must not supply the old scope's expiry")
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
	require.NotNil(t, created.Quota.SubscriptionPeriodEnd)
	assert.Equal(t, "2027-11-11T05:50:51Z", created.Quota.SubscriptionPeriodEnd.Format(time.RFC3339))
}

func TestUpstreamSubscriptionAccountIsOptionalAndPreservedWhenOmitted(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := subscriptionResponse(t, s.request(t, http.MethodPost, "", gin.H{"name": "Account label", "account": " team@example.com ", "platform": "opencode", "credential": "oc_sk_account-label"}, s.bundle.AccessToken))
	assert.Equal(t, "team@example.com", created.Account)
	updated := subscriptionResponse(t, s.request(t, http.MethodPut, "/"+strconv.Itoa(created.ID), gin.H{"name": "Renamed", "platform": "opencode"}, s.bundle.AccessToken))
	assert.Equal(t, created.Account, updated.Account)
	invalid := s.request(t, http.MethodPut, "/"+strconv.Itoa(created.ID), gin.H{"platform": "opencode", "account": strings.Repeat("x", 129)}, s.bundle.AccessToken)
	assert.Equal(t, http.StatusBadRequest, invalid.Code)
	cleared := subscriptionResponse(t, s.request(t, http.MethodPut, "/"+strconv.Itoa(created.ID), gin.H{"platform": "opencode", "account": ""}, s.bundle.AccessToken))
	assert.Empty(t, cleared.Account)
	assert.Equal(t, created.Quota, cleared.Quota)
}

func TestUpstreamSubscriptionOptionalExpiryFailurePreservesQuota(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "unavailable", status: http.StatusForbidden},
		{name: "missing", body: `{}`},
		{name: "invalid", body: `{"subscriptionCurrentPeriodEnd":"not-a-date"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := setupSubscriptionControllerTest(t)
			s.fixture.summaryBody, s.fixture.summaryStatus = test.body, test.status
			created := s.create(t, "opendesign", "vela_ctrl_optional-expiry")
			require.NotNil(t, created.Quota)
			assert.Len(t, created.Quota.Windows, 2)
			assert.Nil(t, created.Quota.SubscriptionPeriodEnd)
			assert.Empty(t, created.ErrorCode)
		})
	}
}

func TestUpstreamSubscriptionDoesNotUsePersonalExpiryForTeamWorkspace(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	s.fixture.workspaceType = "team"
	created := s.create(t, "opendesign", "vela_ctrl_team")
	require.NotNil(t, created.Quota)
	assert.Nil(t, created.Quota.SubscriptionPeriodEnd)
	assert.Zero(t, s.fixture.summaryCalls)
	assert.Len(t, created.Quota.Windows, 2)
}

func TestUpstreamSubscriptionAPIChannelBindingUsesLiveChannelName(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	require.NoError(t, s.db.AutoMigrate(&model.Channel{}))
	priority, weight := int64(7), uint(30)
	channel := model.Channel{Name: "Channel original", Key: "private-channel-key", Priority: &priority, Weight: &weight}
	require.NoError(t, s.db.Create(&channel).Error)
	response := s.request(t, http.MethodPost, "", gin.H{"name": "Must not rename channel", "platform": "opencode", "credential": "oc_sk_bound", "channel_id": channel.Id}, s.bundle.AccessToken)
	require.Equal(t, http.StatusOK, response.Code)
	var body struct {
		Data struct {
			ID        int    `json:"id"`
			Name      string `json:"name"`
			ChannelID int    `json:"channel_id"`
			Channel   struct {
				Name     string `json:"name"`
				Priority int64  `json:"priority"`
				Weight   int    `json:"weight"`
			} `json:"channel"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
	assert.Equal(t, "Channel original", body.Data.Name)
	assert.Equal(t, channel.Id, body.Data.ChannelID)
	assert.Equal(t, int64(7), body.Data.Channel.Priority)
	assert.Equal(t, 30, body.Data.Channel.Weight)
	assert.NotContains(t, response.Body.String(), "private-channel-key")
	require.NoError(t, s.db.Model(&channel).Updates(map[string]any{"name": "Channel renamed", "priority": 9, "weight": 20}).Error)
	listed := s.request(t, http.MethodGet, "", nil, s.bundle.AccessToken)
	require.Equal(t, http.StatusOK, listed.Code)
	var list struct {
		Data []struct {
			Name    string `json:"name"`
			Channel struct {
				Priority int64 `json:"priority"`
				Weight   int   `json:"weight"`
			} `json:"channel"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(listed.Body.Bytes(), &list))
	require.Len(t, list.Data, 1)
	assert.Equal(t, "Channel renamed", list.Data[0].Name)
	assert.Equal(t, int64(9), list.Data[0].Channel.Priority)
	assert.Equal(t, 20, list.Data[0].Channel.Weight)
	independent := subscriptionResponse(t, s.request(t, http.MethodPut, "/"+strconv.Itoa(body.Data.ID), gin.H{"name": "Independent again", "platform": "opencode", "channel_id": 0}, s.bundle.AccessToken))
	assert.Equal(t, "Independent again", independent.Name)
	require.NotNil(t, independent.Quota)
	assert.Equal(t, 4.0, *independent.Quota.Windows[1].UsedPercent)
	var unchanged model.Channel
	require.NoError(t, s.db.First(&unchanged, channel.Id).Error)
	assert.Equal(t, "Channel renamed", unchanged.Name)
	assert.Equal(t, "private-channel-key", unchanged.Key)
	rebound := s.request(t, http.MethodPut, "/"+strconv.Itoa(body.Data.ID), gin.H{"name": "Ignored", "platform": "opencode", "channel_id": channel.Id}, s.bundle.AccessToken)
	require.Equal(t, http.StatusOK, rebound.Code)
	assert.Equal(t, "Channel renamed", subscriptionResponse(t, rebound).Name)
	options := s.request(t, http.MethodGet, "/channels", nil, s.bundle.AccessToken)
	require.Equal(t, http.StatusOK, options.Code)
	assert.Contains(t, options.Body.String(), `"priority":9`)
	assert.NotContains(t, options.Body.String(), "private-channel-key")
	for _, invalidID := range []int{-1, channel.Id + 1000} {
		invalid := s.request(t, http.MethodPut, "/"+strconv.Itoa(body.Data.ID), gin.H{"platform": "opencode", "channel_id": invalidID}, s.bundle.AccessToken)
		assert.Equal(t, http.StatusBadRequest, invalid.Code)
		assert.Contains(t, invalid.Body.String(), `"message_key":"Selected channel was not found; select another channel or use an independent name"`)
	}
	require.NoError(t, s.db.Delete(&channel).Error)
	listed = s.request(t, http.MethodGet, "", nil, s.bundle.AccessToken)
	require.Equal(t, http.StatusOK, listed.Code)
	assert.Contains(t, listed.Body.String(), `"channel":null`)
	assert.Contains(t, listed.Body.String(), fmt.Sprintf(`"channel_id":%d`, channel.Id))
	independent = subscriptionResponse(t, s.request(t, http.MethodPut, "/"+strconv.Itoa(body.Data.ID), gin.H{"name": "Recovered", "platform": "opencode", "channel_id": 0}, s.bundle.AccessToken))
	assert.Equal(t, "Recovered", independent.Name)
}

func TestUpstreamSubscriptionChannelBindingMigrationPreservesExistingConnection(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	created := s.create(t, "opencode", "oc_sk_migration")
	before, err := model.GetUpstreamSubscription(created.ID)
	require.NoError(t, err)
	require.NoError(t, s.db.Migrator().DropColumn(&model.UpstreamSubscription{}, "channel_id"))
	require.NoError(t, s.db.Migrator().DropColumn(&model.UpstreamSubscription{}, "account"))
	reopened, err := gorm.Open(s.db.Dialector, &gorm.Config{})
	require.NoError(t, err)
	s.db, model.DB = reopened, reopened
	t.Cleanup(func() { pool, err := reopened.DB(); require.NoError(t, err); require.NoError(t, pool.Close()) })
	for range 2 {
		require.NoError(t, reopened.AutoMigrate(&model.UpstreamSubscription{}))
	}
	after, err := model.GetUpstreamSubscription(created.ID)
	require.NoError(t, err)
	assert.Zero(t, after.ChannelID)
	assert.Empty(t, after.Account)
	assert.Equal(t, before.CredentialEncrypted, after.CredentialEncrypted)
	assert.Equal(t, before.Name, after.Name)
	assert.Equal(t, before.QuotaJSON, after.QuotaJSON)
	assert.Equal(t, before.RefreshIntervalMinutes, after.RefreshIntervalMinutes)
}

func TestUpstreamSubscriptionLoginCarriesChannelBinding(t *testing.T) {
	s := setupSubscriptionControllerTest(t)
	require.NoError(t, s.db.AutoMigrate(&model.Channel{}))
	channel := model.Channel{Name: "Login channel", Key: "channel-private"}
	require.NoError(t, s.db.Create(&channel).Error)
	response := s.request(t, http.MethodPost, "/opendesign/login", gin.H{"name": "Ignored", "account": "login@example.com", "channel_id": channel.Id, "refresh_interval_minutes": 17, "auto_refresh_disabled": true}, s.bundle.AccessToken)
	require.Equal(t, http.StatusOK, response.Code)
	var challenge struct {
		Data struct {
			Token string `json:"flow_token"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &challenge))
	require.NotEmpty(t, challenge.Data.Token)
	s.allowPoll(t, challenge.Data.Token)
	s.fixture.loginStatus = "approved"
	approved := s.request(t, http.MethodPost, "/opendesign/login/poll", gin.H{"flow_token": challenge.Data.Token}, s.bundle.AccessToken)
	require.Equal(t, http.StatusOK, approved.Code)
	assert.Contains(t, approved.Body.String(), `"name":"Login channel"`)
	assert.Contains(t, approved.Body.String(), `"account":"login@example.com"`)
	assert.Contains(t, approved.Body.String(), `"auto_refresh_disabled":true`)
	assert.Contains(t, approved.Body.String(), fmt.Sprintf(`"channel_id":%d`, channel.Id))
	assert.NotContains(t, approved.Body.String(), "channel-private")
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
		{name: "commandcode rate limit", platform: "commandcode", status: 429, code: "rate_limited"},
		{name: "commandcode missing windows", platform: "commandcode", body: `{"credits":{}}`, code: "invalid_response"},
		{name: "commandcode missing usage", platform: "commandcode", body: `{"windowLimits":{"fiveHour":{"cap":14}}}`, code: "invalid_response"},
		{name: "commandcode negative cap", platform: "commandcode", body: `{"windowLimits":{"fiveHour":{"used":1,"cap":-1}}}`, code: "invalid_response"},
		{name: "commandcode invalid reset", platform: "commandcode", body: `{"windowLimits":{"fiveHour":{"used":1,"cap":14,"resetAt":-1}}}`, code: "invalid_response"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := setupSubscriptionControllerTest(t)
			credential := "oc_sk_subscription"
			if test.platform == "opendesign" {
				credential = "vela_ctrl_subscription"
			} else if test.platform == "commandcode" {
				credential = subscriptionCommandCodeKey
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
