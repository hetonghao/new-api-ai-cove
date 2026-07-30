package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type riskObservationSinkFunc func(context.Context, RiskObservationEvent) error

var riskObservationTestDatabaseSequence atomic.Uint64

func (sink riskObservationSinkFunc) RecordRiskObservation(ctx context.Context, event RiskObservationEvent) error {
	return sink(ctx, event)
}

func TestProcessRiskObservation_reviews_selective_hit_and_records_usage(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	var reviewedContent string
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		require.NoError(t, common.DecodeJson(request.Body, &body))
		reviewedContent = body.Messages[0].Content
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":{"response":{"safe":false,"categories":["S1"]},"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4,"neurons":9.072817475858999}}}`))
	}))
	defer providerServer.Close()
	provider := createActiveRiskProvider(t, providerServer.URL)
	providerID := provider.Id
	_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
		ProviderIDs:     []int{providerID},
		EnabledChannels: []int{createRiskPolicyChannel(t)},
		ReviewMode:      model.RiskReviewSelective,
		ActionMode:      model.RiskActionObserve,
	})
	require.NoError(t, err)
	_, err = model.CreateRiskRule(model.RiskRuleInput{RuleType: model.RiskRuleKeyword, Pattern: "danger", Enabled: true})
	require.NoError(t, err)
	events := make(chan RiskObservationEvent, 1)
	SetRiskObservationSink(riskObservationSinkFunc(func(_ context.Context, event RiskObservationEvent) error {
		events <- event
		return nil
	}))

	// When
	processRiskObservation(context.Background(), RiskObservationJob{
		RequestID: "req-1", ChannelID: 7, UserID: 42,
		ChannelName: " CPA-Pro ",
		Text:        strings.Repeat("safe ", 600) + "danger" + strings.Repeat(" tail", 600),
		ProviderID:  providerID,
		ReviewMode:  model.RiskReviewSelective,
		ActionMode:  model.RiskActionObserve,
	})

	// Then
	event := <-events
	require.Equal(t, RiskObservationUnsafe, event.Result)
	require.Equal(t, []int{1}, event.RuleIDs)
	require.Equal(t, []string{"S1"}, event.Categories)
	require.Equal(t, 3, event.PromptTokens)
	require.InDelta(t, 9.072817475858999, event.Neurons, 1e-12)
	require.Equal(t, provider.Id, event.ProviderID)
	require.LessOrEqual(t, len([]rune(reviewedContent)), riskExcerptLimit)
}

func TestProcessRiskObservation_skips_selected_channel_not_enabled(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	providerCalled := false
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalled = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"result":{"response":{"safe":true},"usage":{}}}`))
	}))
	defer providerServer.Close()
	provider := createActiveRiskProvider(t, providerServer.URL)
	providerID := provider.Id
	_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
		ProviderIDs:     []int{providerID},
		EnabledChannels: []int{createRiskPolicyChannel(t)},
		ReviewMode:      model.RiskReviewFull,
		ActionMode:      model.RiskActionObserve,
	})
	require.NoError(t, err)
	recorded := false
	SetRiskObservationSink(riskObservationSinkFunc(func(_ context.Context, _ RiskObservationEvent) error {
		recorded = true
		return nil
	}))

	// When
	processRiskObservation(context.Background(), RiskObservationJob{
		RequestID: "req-other", ChannelID: 8, ChannelName: "CPA-core", UserID: 42, Text: "current",
	})

	// Then
	require.False(t, providerCalled)
	require.False(t, recorded)
}

func TestProcessRiskObservation_skips_disabled_policy(t *testing.T) {
	// Given
	setupRiskObservationTest(t)
	called := false
	SetRiskObservationSink(riskObservationSinkFunc(func(_ context.Context, _ RiskObservationEvent) error {
		called = true
		return nil
	}))

	// When
	processRiskObservation(context.Background(), RiskObservationJob{RequestID: "disabled", ChannelName: "CPA-pro", Text: "danger"})

	// Then
	require.False(t, called)
}

func TestProcessRiskObservation_records_safe_and_provider_error(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantResult RiskObservationResult
		wantError  string
	}{
		{
			name:       "safe",
			statusCode: http.StatusOK,
			body:       `{"success":true,"result":{"response":{"safe":true,"categories":[]},"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}}`,
			wantResult: RiskObservationSafe,
		},
		{
			name:       "provider error",
			statusCode: http.StatusTooManyRequests,
			body:       `{"error":"rate limited"}`,
			wantResult: RiskObservationError,
			wantError:  riskObservationProviderError,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			setupRiskObservationTest(t)
			providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(test.body))
			}))
			defer providerServer.Close()
			provider := createActiveRiskProvider(t, providerServer.URL)
			providerID := provider.Id
			_, err := model.SaveRiskPolicy(model.RiskPolicyInput{
				ProviderIDs:     []int{providerID},
				EnabledChannels: []int{createRiskPolicyChannel(t)},
				ReviewMode:      model.RiskReviewFull,
				ActionMode:      model.RiskActionObserve,
			})
			require.NoError(t, err)
			events := make(chan RiskObservationEvent, 1)
			SetRiskObservationSink(riskObservationSinkFunc(func(_ context.Context, event RiskObservationEvent) error {
				events <- event
				return nil
			}))

			// When
			processRiskObservation(context.Background(), RiskObservationJob{
				RequestID: "req", ChannelName: "CPA-pro", Text: "current",
				ProviderID: providerID, ReviewMode: model.RiskReviewFull, ActionMode: model.RiskActionObserve,
			})

			// Then
			event := <-events
			require.Equal(t, test.wantResult, event.Result)
			require.Equal(t, test.wantError, event.ErrorCode)
		})
	}
}

func setupRiskObservationTest(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalSecret := common.CryptoSecret
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	common.CryptoSecret = "risk-observation-test-key"
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf(
		"file:%s_%d?mode=memory&cache=shared",
		strings.ReplaceAll(t.Name(), "/", "_"),
		riskObservationTestDatabaseSequence.Add(1),
	)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.RiskProvider{}, &model.RiskPolicy{}, &model.RiskRule{}, &model.Channel{}))
	InitHttpClient()
	t.Cleanup(func() {
		SetRiskObservationSink(nil)
		model.DB = originalDB
		common.CryptoSecret = originalSecret
		common.SetDatabaseTypes(originalMainType, originalLogType)
	})
}

func createActiveRiskProvider(t *testing.T, baseURL string) *model.RiskProvider {
	t.Helper()
	serverURL, err := url.Parse(baseURL)
	require.NoError(t, err)
	if serverURL.Hostname() == "127.0.0.1" || serverURL.Hostname() == "localhost" {
		originalHTTPClient := httpClient
		httpClient = &http.Client{Transport: riskProviderRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			proxiedRequest := request.Clone(request.Context())
			proxiedURL := *request.URL
			proxiedURL.Scheme = serverURL.Scheme
			proxiedURL.Host = serverURL.Host
			proxiedRequest.URL = &proxiedURL
			return http.DefaultTransport.RoundTrip(proxiedRequest)
		})}
		t.Cleanup(func() { httpClient = originalHTTPClient })
	}
	credential, err := common.EncryptCredential("token")
	require.NoError(t, err)
	provider := &model.RiskProvider{
		Name: "provider", ProviderType: model.RiskProviderCloudflare, Model: "guard-" + strings.ReplaceAll(t.Name(), "/", "-"),
		AccountID: "0123456789abcdef0123456789abcdef", BaseURL: baseURL, CredentialEncrypted: credential, TimeoutMs: 800,
	}
	require.NoError(t, model.CreateRiskProvider(provider))
	require.NoError(t, model.MarkRiskProviderValidated(provider.Id))
	return provider
}

func createRiskPolicyChannel(t *testing.T) int {
	t.Helper()
	channel := &model.Channel{Name: "Risk channel", Key: "secret", Models: "gpt-test"}
	require.NoError(t, model.DB.Create(channel).Error)
	return channel.Id
}
