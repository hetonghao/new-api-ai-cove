package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRiskInternalAuthTest(t *testing.T) *gin.Engine {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousRedis := common.RedisEnabled
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.RedisEnabled = previousRedis
	})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.RiskProvider{}))
	root := &model.User{
		Username: "root", Role: common.RoleRootUser, Status: common.UserStatusEnabled,
		Group: "default", Quota: 1_000_000,
	}
	require.NoError(t, db.Create(root).Error)
	allowIPs := "127.0.0.1/32\n::1/128"
	token := &model.Token{
		UserId: root.Id, Key: "riskinternalkey", Name: "AI 风控内部审核 #1",
		Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true,
		ModelLimitsEnabled: true, ModelLimits: "gpt-5.4-mini", AllowIps: &allowIPs,
		SystemManaged: true,
	}
	require.NoError(t, db.Create(token).Error)
	provider := &model.RiskProvider{
		Name: "internal", ProviderType: model.RiskProviderPlatformInternal,
		ChannelID: 1, Model: "gpt-5.4-mini", InternalTokenID: token.Id,
	}
	require.NoError(t, db.Create(provider).Error)

	router := gin.New()
	require.NoError(t, router.SetTrustedProxies([]string{"0.0.0.0/0", "::/0"}))
	router.POST("/v1/chat/completions", TokenAuth(), func(c *gin.Context) {
		if !common.GetContextKeyBool(c, constant.ContextKeyRiskInternalReview) {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusNoContent)
	})
	return router
}

func performRiskInternalAuthRequest(router *gin.Engine, remoteAddr, forwardedFor, channelID string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	request.RemoteAddr = remoteAddr
	request.Header.Set("Authorization", "Bearer sk-riskinternalkey-"+channelID)
	request.Header.Set("X-Forwarded-For", forwardedFor)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func TestTokenAuthSystemManagedTokenUsesSocketIP(t *testing.T) {
	// Given
	router := setupRiskInternalAuthTest(t)

	// When
	externalResponse := performRiskInternalAuthRequest(router, "203.0.113.10:4567", "127.0.0.1", "1")
	loopbackResponse := performRiskInternalAuthRequest(router, "127.0.0.1:4567", "203.0.113.10", "1")

	// Then
	assert.Equal(t, http.StatusForbidden, externalResponse.Code)
	assert.Equal(t, http.StatusNoContent, loopbackResponse.Code)
}

func TestTokenAuthSystemManagedTokenRejectsDifferentChannel(t *testing.T) {
	// Given
	router := setupRiskInternalAuthTest(t)

	// When
	response := performRiskInternalAuthRequest(router, "127.0.0.1:4567", "127.0.0.1", "2")

	// Then
	assert.Equal(t, http.StatusForbidden, response.Code)
}

func TestTokenAuthPreservesAuthorizationForPrivateWebSocketSubprotocol(t *testing.T) {
	// Given
	router := setupRiskInternalAuthTest(t)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	request.RemoteAddr = "127.0.0.1:4567"
	request.Header.Set("Authorization", "Bearer sk-riskinternalkey-1")
	request.Header.Set("Sec-WebSocket-Protocol", "ai-cove-zstd.v1")
	request.Header.Set("X-Forwarded-For", "127.0.0.1")
	response := httptest.NewRecorder()

	// When
	router.ServeHTTP(response, request)

	// Then
	assert.Equal(t, http.StatusNoContent, response.Code)
}
