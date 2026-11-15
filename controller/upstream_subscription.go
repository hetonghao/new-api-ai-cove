package controller

import (
	"errors"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
	"strconv"
	"strings"
)

func subscriptionRequestError(c *gin.Context, status int, message string) {
	body := common.NewMessage(message).Fields()
	body["success"] = false
	body["message"] = message
	c.JSON(status, body)
}

func upstreamSubscriptionID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id < 1 {
		subscriptionRequestError(c, http.StatusBadRequest, "Invalid subscription ID")
		return 0, false
	}
	return id, true
}

func writeUpstreamSubscription(c *gin.Context, subscription *model.UpstreamSubscription, err error) {
	if err != nil {
		if errors.Is(err, model.ErrSubscriptionChannelUnavailable) || errors.Is(err, model.ErrSubscriptionAccountTooLong) {
			subscriptionRequestError(c, http.StatusBadRequest, err.Error())
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			subscriptionRequestError(c, http.StatusNotFound, "Subscription was not found")
		} else {
			subscriptionRequestError(c, http.StatusConflict, "Subscription changed; reload and try again")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": subscription.Response()})
}

func ListUpstreamSubscriptions(c *gin.Context) {
	subscriptions, err := model.ListUpstreamSubscriptions()
	if err != nil {
		subscriptionRequestError(c, http.StatusInternalServerError, "Failed to load subscriptions")
		return
	}
	responses := make([]model.UpstreamSubscriptionResponse, 0, len(subscriptions))
	for _, subscription := range subscriptions {
		responses = append(responses, subscription.Response())
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": responses})
}

func ListUpstreamSubscriptionChannels(c *gin.Context) {
	channels, err := model.ListUpstreamSubscriptionChannels()
	if err != nil {
		subscriptionRequestError(c, http.StatusInternalServerError, "Failed to load channels")
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": channels})
}

func SaveUpstreamSubscription(c *gin.Context) {
	id := 0
	if c.Param("id") != "" {
		var valid bool
		id, valid = upstreamSubscriptionID(c)
		if !valid {
			return
		}
	}
	var input struct {
		Name                   string  `json:"name"`
		Account                *string `json:"account"`
		ChannelID              *int    `json:"channel_id"`
		Platform               string  `json:"platform"`
		Credential             string  `json:"credential"`
		RefreshIntervalMinutes int     `json:"refresh_interval_minutes"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	if c.ShouldBindJSON(&input) != nil {
		subscriptionRequestError(c, http.StatusBadRequest, "Invalid subscription configuration")
		return
	}
	if input.ChannelID != nil && *input.ChannelID != 0 {
		input.Name = ""
	}
	name, err := model.NormalizeUpstreamSubscription(input.Name, input.Platform)
	if err != nil {
		subscriptionRequestError(c, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := model.NormalizeSubscriptionRefreshMinutes(input.RefreshIntervalMinutes); err != nil {
		subscriptionRequestError(c, http.StatusBadRequest, err.Error())
		return
	}
	credential := strings.TrimSpace(input.Credential)
	encrypted := ""
	if id == 0 || credential != "" {
		if err := service.ValidateSubscriptionCredential(input.Platform, credential); err != nil {
			subscriptionRequestError(c, http.StatusBadRequest, err.Error())
			return
		}
		encrypted, err = common.EncryptCredential(credential)
		if err != nil {
			subscriptionRequestError(c, http.StatusInternalServerError, "Failed to protect subscription credential")
			return
		}
	}
	subscription, err := model.SaveUpstreamSubscriptionWithTx(model.DB, model.UpstreamSubscriptionConfig{ID: id, Name: name, Account: input.Account, ChannelID: input.ChannelID, Platform: input.Platform, CredentialEncrypted: encrypted, RefreshIntervalMinutes: input.RefreshIntervalMinutes})
	if err == nil && (id == 0 || encrypted != "") {
		if refreshed, refreshErr := service.RefreshUpstreamSubscription(c.Request.Context(), subscription.ID, true); refreshErr == nil {
			subscription = refreshed
		}
	}
	writeUpstreamSubscription(c, subscription, err)
}

func DeleteUpstreamSubscription(c *gin.Context) {
	id, valid := upstreamSubscriptionID(c)
	if !valid {
		return
	}
	if err := model.DeleteUpstreamSubscription(id); err != nil {
		writeUpstreamSubscription(c, nil, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func RefreshUpstreamSubscription(c *gin.Context) {
	id, valid := upstreamSubscriptionID(c)
	if !valid {
		return
	}
	subscription, err := service.RefreshUpstreamSubscription(c.Request.Context(), id, true)
	writeUpstreamSubscription(c, subscription, err)
}
