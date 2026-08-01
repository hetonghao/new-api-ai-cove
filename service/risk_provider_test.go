// allow: SIZE_OK -- provider lifecycle tests share one database and HTTP fixture boundary.
package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type riskProviderRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn riskProviderRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func setupPlatformInternalRiskProviderTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Token{}, &model.RiskProvider{}))
	t.Cleanup(func() {
		common.RedisEnabled = originalRedisEnabled
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func createPlatformInternalRiskProviderFixture(t *testing.T, db *gorm.DB) (*model.RiskProvider, *model.Token) {
	t.Helper()
	root := &model.User{
		Username: "root", Password: "password", Role: common.RoleRootUser,
		Status: common.UserStatusEnabled, Group: "default", Quota: 1000000,
	}
	require.NoError(t, db.Create(root).Error)
	channel := &model.Channel{
		Name: "Internal review", Key: "upstream-key", Status: common.ChannelStatusEnabled,
		Models: "guard-model", Group: "default",
	}
	require.NoError(t, db.Create(channel).Error)
	provider := &model.RiskProvider{
		Name: "Platform review", ProviderType: model.RiskProviderPlatformInternal,
		ChannelID: channel.Id, Model: "guard-model",
	}
	require.NoError(t, model.CreateRiskProvider(provider))
	var token model.Token
	require.NoError(t, db.First(&token, provider.InternalTokenID).Error)
	return provider, &token
}

func TestReviewRiskContentMapsPlatformInternalChatCompletion(t *testing.T) {
	originalDB := model.DB
	t.Setenv("PORT", "34567")
	t.Cleanup(func() { model.DB = originalDB })
	db := setupPlatformInternalRiskProviderTestDB(t)
	provider, token := createPlatformInternalRiskProviderFixture(t, db)

	originalClient := platformInternalRiskHTTPClient
	platformInternalRiskHTTPClient = &http.Client{Transport: riskProviderRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, "http://127.0.0.1:34567/v1/chat/completions", request.URL.String())
		assert.Equal(t, "Bearer sk-"+token.Key+"-"+strconv.Itoa(provider.ChannelID), request.Header.Get("Authorization"))
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), `"stream":false`)
		assert.Contains(t, string(body), `"temperature":0`)
		assert.Contains(t, string(body), "connection test")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"choices":[{"message":{"content":"{\"verdict\":\"unsafe\",\"categories\":[\"S1\"]}"}}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`,
			)),
		}, nil
	})}
	t.Cleanup(func() { platformInternalRiskHTTPClient = originalClient })

	result, err := ReviewRiskContent(context.Background(), provider, "connection test")
	require.NoError(t, err)
	assert.Equal(t, RiskReviewUnsafe, result.Status)
	assert.Equal(t, []string{"S1"}, result.Categories)
	assert.Equal(t, RiskReviewUsage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18}, result.Usage)
}

func TestReviewRiskContentRejectsInvalidPlatformInternalVerdict(t *testing.T) {
	originalDB := model.DB
	t.Cleanup(func() { model.DB = originalDB })
	db := setupPlatformInternalRiskProviderTestDB(t)
	provider, _ := createPlatformInternalRiskProviderFixture(t, db)
	originalClient := platformInternalRiskHTTPClient
	calls := 0
	platformInternalRiskHTTPClient = &http.Client{Transport: riskProviderRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(
				`{"choices":[{"message":{"content":"{\"verdict\":\"maybe\",\"categories\":[]}"}}]}`,
			)),
		}, nil
	})}
	t.Cleanup(func() { platformInternalRiskHTTPClient = originalClient })

	_, err := ReviewRiskContent(context.Background(), provider, "connection test")
	require.Error(t, err)
	code, detail := RiskObservationErrorInfo(err)
	assert.Equal(t, riskObservationProviderError, code)
	assert.Equal(t, "Platform internal model returned an invalid moderation verdict", detail)
	assert.Equal(t, 1, calls)
}

func riskProviderTestProvider(t *testing.T) *model.RiskProvider {
	t.Helper()
	ciphertext, err := common.EncryptCredential("cf-token")
	require.NoError(t, err)
	return &model.RiskProvider{
		ProviderType:        model.RiskProviderCloudflare,
		AccountID:           "0123456789abcdef0123456789abcdef",
		Model:               "@cf/meta/llama-guard-3-8b",
		CredentialEncrypted: ciphertext,
		TimeoutMs:           800,
	}
}

func TestReviewRiskContentMapsCloudflareResponses(t *testing.T) {
	originalSecret := common.CryptoSecret
	common.CryptoSecret = "risk-provider-test-key"
	t.Cleanup(func() { common.CryptoSecret = originalSecret })

	tests := []struct {
		name        string
		response    string
		legacy      bool
		wantStatus  RiskReviewStatus
		categories  []string
		wantNeurons float64
	}{
		{name: "safe object", response: `{"success":true,"result":{"response":{"safe":true,"categories":[]},"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4,"neurons":12}}}`, wantStatus: RiskReviewSafe, categories: []string{}, wantNeurons: 12},
		{name: "safe object with fractional neurons", response: `{"success":true,"result":{"response":{"safe":true,"categories":[]},"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4,"neurons":9.072817475858999}}}`, wantStatus: RiskReviewSafe, categories: []string{}, wantNeurons: 9.072817475858999},
		{name: "unsafe text from legacy row", response: `{"success":true,"result":{"response":"unsafe\nS1,S9","usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}}`, legacy: true, wantStatus: RiskReviewUnsafe, categories: []string{"S1", "S9"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalHTTPClient := httpClient
			httpClient = &http.Client{Transport: riskProviderRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				assert.Equal(t, http.MethodPost, request.Method)
				assert.Equal(t, "Bearer cf-token", request.Header.Get("Authorization"))
				assert.Equal(t, "https://api.cloudflare.com/client/v4/accounts/0123456789abcdef0123456789abcdef/ai/run/@cf/meta/llama-guard-3-8b", request.URL.String())
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(tt.response)),
				}, nil
			})}
			t.Cleanup(func() { httpClient = originalHTTPClient })

			provider := riskProviderTestProvider(t)
			if tt.legacy {
				provider.AccountID = ""
				provider.BaseURL = "https://legacy.example/client/v4/accounts/0123456789abcdef0123456789abcdef/ai/run"
			}

			result, err := ReviewRiskContent(context.Background(), provider, "connection test")
			require.NoError(t, err)
			assert.Equal(t, tt.wantStatus, result.Status)
			assert.Equal(t, tt.categories, result.Categories)
			if tt.wantNeurons != 0 {
				assert.InDelta(t, tt.wantNeurons, result.Usage.Neurons, 1e-12)
			}
		})
	}
}

func TestCloudflareDailyNeuronsResponse_requires_explicit_daily_neurons_signal(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "daily neurons quota", body: `{"error":"daily neurons quota exceeded"}`, want: true},
		{name: "neurons daily limit", body: `{"error":"neurons daily limit exhausted"}`, want: true},
		{name: "generic quota", body: `{"error":"quota exceeded"}`},
		{name: "generic rate limit", body: `{"error":"daily request limit exceeded"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, cloudflareDailyNeuronsResponse([]byte(test.body)))
		})
	}
}

func TestReviewRiskContent_returnsSafeProviderErrorDetails(t *testing.T) {
	// Given
	originalSecret := common.CryptoSecret
	common.CryptoSecret = "risk-provider-error-test-key"
	t.Cleanup(func() { common.CryptoSecret = originalSecret })
	provider := riskProviderTestProvider(t)
	privateText := strings.Repeat("private user text ", 80)
	unsafeValues := []string{provider.AccountID, "cf-token", privateText, "https://api.cloudflare.com/client/v4/accounts/" + provider.AccountID}
	transportCause := errors.New("dial " + unsafeValues[3] + " Authorization Bearer cf-token body=" + privateText)
	tests := []struct {
		name       string
		transport  riskProviderRoundTripFunc
		wantCode   string
		wantDetail string
		wantCause  error
	}{
		{
			name: "network failure",
			transport: func(*http.Request) (*http.Response, error) {
				return nil, transportCause
			},
			wantCode: riskObservationProviderError, wantDetail: "Cloudflare network request failed", wantCause: transportCause,
		},
		{
			name: "HTTP failure",
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader(strings.Join(unsafeValues, " ")))}, nil
			},
			wantCode: riskObservationProviderError, wantDetail: "Cloudflare returned HTTP 429",
		},
		{
			name: "response parse failure",
			transport: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"success":true,"result":` + privateText))}, nil
			},
			wantCode: riskObservationProviderError, wantDetail: "Cloudflare response could not be decoded",
		},
		{
			name: "deadline",
			transport: func(*http.Request) (*http.Response, error) {
				return nil, context.DeadlineExceeded
			},
			wantCode: riskObservationTimeout, wantDetail: "Cloudflare request timed out", wantCause: context.DeadlineExceeded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			originalHTTPClient := httpClient
			httpClient = &http.Client{Transport: test.transport}
			t.Cleanup(func() { httpClient = originalHTTPClient })

			// When
			_, err := ReviewRiskContent(context.Background(), provider, privateText)
			code, detail := RiskObservationErrorInfo(err)

			// Then
			require.Error(t, err)
			if test.wantCause != nil {
				require.ErrorIs(t, err, test.wantCause)
			}
			assert.Equal(t, test.wantCode, code)
			assert.Equal(t, test.wantDetail, detail)
			assert.LessOrEqual(t, len([]rune(detail)), riskProviderErrorDetailMaxRunes)
			for _, unsafeValue := range unsafeValues {
				assert.NotContains(t, detail, unsafeValue)
			}
		})
	}
}
