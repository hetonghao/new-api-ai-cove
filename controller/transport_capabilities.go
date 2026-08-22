package controller

import (
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	transportCapabilityMaxModels = 100
	transportCapabilityTTL       = 30 * time.Second
)

type transportCapabilityItem struct {
	Model              string `json:"model"`
	Allowed            bool   `json:"allowed"`
	HTTP               bool   `json:"http"`
	ResponsesWebSocket bool   `json:"responses_websocket"`
	ReasonCode         string `json:"reason_code"`
}

func TransportCapabilities(c *gin.Context) {
	models, ok := parseTransportCapabilityModels(c.Request.URL.Query()["models"])
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    "invalid_models",
			"message": "models must contain one or more model slugs",
		})
		return
	}
	if len(models) > transportCapabilityMaxModels {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"code":    "too_many_models",
			"message": "models must contain at most 100 unique slugs",
		})
		return
	}

	groups, err := getModelListGroups(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "group_lookup_failed"})
		return
	}
	tokenLimited := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
	tokenModelLimit := map[string]bool{}
	if tokenLimited {
		if value, exists := common.GetContextKey(c, constant.ContextKeyTokenModelLimit); exists {
			tokenModelLimit, _ = value.(map[string]bool)
		}
	}

	now := time.Now().UTC()
	items := make([]transportCapabilityItem, 0, len(models))
	for _, modelName := range models {
		item := transportCapabilityItem{Model: modelName, ReasonCode: "model_not_allowed"}
		if tokenLimited {
			if !tokenModelLimit[modelName] {
				items = append(items, item)
				continue
			}
		}
		capability, capabilityErr := model.GetTransportCapability(groups.ownerGroups, modelName)
		if capabilityErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "code": "capability_lookup_failed"})
			return
		}
		item.HTTP = capability.HTTP
		item.ResponsesWebSocket = capability.ResponsesWebSocket
		item.Allowed = capability.HTTP
		if item.Allowed {
			item.ReasonCode = "http_only"
			if item.ResponsesWebSocket {
				item.ReasonCode = "ok"
			}
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"object":       "transport_capabilities",
		"generated_at": now,
		"expires_at":   now.Add(transportCapabilityTTL),
		"data":         items,
	})
}

func parseTransportCapabilityModels(values []string) ([]string, bool) {
	seen := make(map[string]struct{})
	models := make([]string, 0)
	for _, value := range values {
		for _, rawModel := range strings.Split(value, ",") {
			modelName := strings.TrimSpace(rawModel)
			if modelName == "" {
				continue
			}
			if _, exists := seen[modelName]; exists {
				continue
			}
			seen[modelName] = struct{}{}
			models = append(models, modelName)
		}
	}
	return models, len(models) > 0
}
