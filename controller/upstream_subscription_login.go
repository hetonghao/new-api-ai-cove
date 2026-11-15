package controller

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type subscriptionLoginPayload struct {
	Identity               model.AuthSessionIdentity `json:"identity"`
	Name                   string                    `json:"name"`
	Account                *string                   `json:"account,omitempty"`
	ChannelID              *int                      `json:"channel_id,omitempty"`
	SubscriptionID         int                       `json:"subscription_id"`
	DeviceID               string                    `json:"device_id"`
	DeviceSecret           string                    `json:"device_secret"`
	NextPollAt             int64                     `json:"next_poll_at"`
	PollInterval           int                       `json:"poll_interval"`
	RefreshIntervalMinutes int                       `json:"refresh_interval_minutes"`
}

func subscriptionLoginIdentity(c *gin.Context) (model.AuthSessionIdentity, model.AuthFlowMatch, bool) {
	identity, valid := middleware.GetAuthIdentity(c)
	if !valid || identity.SessionID == "" {
		subscriptionRequestError(c, http.StatusUnauthorized, "A dashboard login session is required")
		return identity, model.AuthFlowMatch{}, false
	}
	return identity, model.AuthFlowMatch{Purpose: model.AuthFlowPurposeSubscription, Provider: model.SubscriptionPlatformOpenDesign, UserId: identity.UserID, SessionId: identity.SessionID}, true
}

func StartOpenDesignSubscriptionLogin(c *gin.Context) {
	identity, _, valid := subscriptionLoginIdentity(c)
	if !valid {
		return
	}
	var input struct {
		Name                   string  `json:"name"`
		Account                *string `json:"account"`
		ChannelID              *int    `json:"channel_id"`
		SubscriptionID         int     `json:"subscription_id"`
		RefreshIntervalMinutes int     `json:"refresh_interval_minutes"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	if c.ShouldBindJSON(&input) != nil || input.SubscriptionID < 0 {
		subscriptionRequestError(c, http.StatusBadRequest, "Invalid subscription configuration")
		return
	}
	if input.Account != nil && utf8.RuneCountInString(strings.TrimSpace(*input.Account)) > 128 {
		subscriptionRequestError(c, http.StatusBadRequest, model.ErrSubscriptionAccountTooLong.Error())
		return
	}
	if input.ChannelID != nil && *input.ChannelID != 0 {
		if *input.ChannelID < 0 {
			subscriptionRequestError(c, http.StatusBadRequest, model.ErrSubscriptionChannelUnavailable.Error())
			return
		}
		if _, err := model.GetUpstreamSubscriptionChannel(model.DB, *input.ChannelID); err != nil {
			subscriptionRequestError(c, http.StatusBadRequest, model.ErrSubscriptionChannelUnavailable.Error())
			return
		}
		input.Name = ""
	}
	name, err := model.NormalizeUpstreamSubscription(input.Name, model.SubscriptionPlatformOpenDesign)
	if err != nil {
		subscriptionRequestError(c, http.StatusBadRequest, err.Error())
		return
	}
	minutes, err := model.NormalizeSubscriptionRefreshMinutes(input.RefreshIntervalMinutes)
	if err != nil {
		subscriptionRequestError(c, http.StatusBadRequest, err.Error())
		return
	}
	if input.SubscriptionID > 0 {
		subscription, err := model.GetUpstreamSubscription(input.SubscriptionID)
		if err != nil || subscription.Platform != model.SubscriptionPlatformOpenDesign {
			subscriptionRequestError(c, http.StatusBadRequest, "OpenDesign subscription was not found")
			return
		}
	}
	challenge, err := service.StartOpenDesignSubscriptionLogin(c.Request.Context())
	if err != nil {
		subscriptionRequestError(c, http.StatusBadGateway, "Could not start OpenDesign login; try again or enter a controlKey")
		return
	}
	payload := subscriptionLoginPayload{Identity: identity, Name: name, Account: input.Account, ChannelID: input.ChannelID, SubscriptionID: input.SubscriptionID, DeviceID: challenge.DeviceID, DeviceSecret: challenge.DeviceSecret, PollInterval: max(challenge.PollInterval, 2), NextPollAt: time.Now().Unix() + int64(max(challenge.PollInterval, 2)), RefreshIntervalMinutes: minutes}
	encoded, err := common.Marshal(payload)
	if err != nil {
		subscriptionRequestError(c, http.StatusInternalServerError, "Could not save OpenDesign login state")
		return
	}
	encrypted, err := common.EncryptCredential(string(encoded))
	if err != nil {
		subscriptionRequestError(c, http.StatusInternalServerError, "Could not save OpenDesign login state")
		return
	}
	if model.ExpireSubscriptionLoginFlows(identity.UserID, identity.SessionID) != nil {
		subscriptionRequestError(c, http.StatusInternalServerError, "Could not save OpenDesign login state")
		return
	}
	token, flow, err := model.CreateAuthFlow(model.AuthFlowCreate{Purpose: model.AuthFlowPurposeSubscription, Provider: model.SubscriptionPlatformOpenDesign, UserId: identity.UserID, SessionId: identity.SessionID, Payload: encrypted, ExpiresAt: challenge.ExpiresAt})
	if err != nil {
		subscriptionRequestError(c, http.StatusInternalServerError, "Could not save OpenDesign login state")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"flow_token": token, "activation_url": challenge.Activation, "user_code": challenge.UserCode, "expires_at": flow.ExpiresAt, "poll_interval": payload.PollInterval}})
}

func PollOpenDesignSubscriptionLogin(c *gin.Context) {
	identity, match, valid := subscriptionLoginIdentity(c)
	if !valid {
		return
	}
	var input struct {
		Token string `json:"flow_token"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if c.ShouldBindJSON(&input) != nil {
		subscriptionRequestError(c, http.StatusBadRequest, "Invalid OpenDesign login state")
		return
	}
	flow, err := model.GetAuthFlow(input.Token, match)
	if err != nil {
		subscriptionRequestError(c, http.StatusGone, "OpenDesign login expired or was cancelled; start again")
		return
	}
	if strings.HasSuffix(c.FullPath(), "/cancel") {
		if _, err := model.ConsumeAuthFlow(input.Token, match); err != nil {
			subscriptionRequestError(c, http.StatusGone, "OpenDesign login expired or was cancelled; start again")
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	decoded, err := common.DecryptCredential(flow.Payload)
	var payload subscriptionLoginPayload
	if err != nil || common.UnmarshalJsonStr(decoded, &payload) != nil || payload.Identity != identity {
		subscriptionRequestError(c, http.StatusGone, "Invalid OpenDesign login state")
		return
	}
	if payload.NextPollAt > time.Now().Unix() {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "pending"}})
		return
	}
	payload.NextPollAt = time.Now().Unix() + int64(payload.PollInterval)
	encoded, err := common.Marshal(payload)
	if err != nil {
		subscriptionRequestError(c, http.StatusInternalServerError, "Invalid OpenDesign login state")
		return
	}
	encryptedPayload, err := common.EncryptCredential(string(encoded))
	if err != nil {
		subscriptionRequestError(c, http.StatusInternalServerError, "Invalid OpenDesign login state")
		return
	}
	claimed, err := model.ClaimSubscriptionLoginPoll(flow, encryptedPayload)
	if err != nil {
		subscriptionRequestError(c, http.StatusInternalServerError, "Could not save OpenDesign login state")
		return
	}
	if !claimed {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "pending"}})
		return
	}
	status, credential, err := service.PollOpenDesignSubscriptionLogin(c.Request.Context(), payload.DeviceID, payload.DeviceSecret)
	if err != nil {
		subscriptionRequestError(c, http.StatusBadGateway, "Could not check OpenDesign login; retry or start again")
		return
	}
	if status == "pending" {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "pending"}})
		return
	}
	if status != "approved" {
		if _, err := model.ConsumeAuthFlow(input.Token, match); err != nil {
			subscriptionRequestError(c, http.StatusGone, "OpenDesign login expired or was cancelled; start again")
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "expired"}})
		return
	}
	encryptedCredential, err := common.EncryptCredential(credential)
	if err != nil {
		subscriptionRequestError(c, http.StatusInternalServerError, "Failed to protect subscription credential")
		return
	}
	var subscription *model.UpstreamSubscription
	_, err = model.ConsumeAuthFlowWithAction(input.Token, match, func(tx *gorm.DB, flow *model.AuthFlow) error {
		if err := model.ValidateAuthSessionWithTx(tx, identity); err != nil {
			return err
		}
		var saveErr error
		subscription, saveErr = model.SaveUpstreamSubscriptionWithTx(tx, model.UpstreamSubscriptionConfig{ID: payload.SubscriptionID, Name: payload.Name, Account: payload.Account, ChannelID: payload.ChannelID, Platform: model.SubscriptionPlatformOpenDesign, CredentialEncrypted: encryptedCredential, RefreshIntervalMinutes: payload.RefreshIntervalMinutes})
		return saveErr
	})
	if err != nil {
		if errors.Is(err, model.ErrSubscriptionChannelUnavailable) || errors.Is(err, model.ErrSubscriptionAccountTooLong) {
			subscriptionRequestError(c, http.StatusBadRequest, err.Error())
			return
		}
		subscriptionRequestError(c, http.StatusGone, "OpenDesign login expired or was cancelled; start again")
		return
	}
	if refreshed, refreshErr := service.RefreshUpstreamSubscription(c.Request.Context(), subscription.ID, true); refreshErr == nil {
		subscription = refreshed
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "approved", "subscription": subscription.Response()}})
}
