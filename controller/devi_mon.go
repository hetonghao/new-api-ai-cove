package controller

import (
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const deviMonMaxRequestSize = 4 << 10

// GetDeviMonSnapshot returns the last Devin monitor collection. It never
// triggers an upstream probe, so opening the page cannot consume quota.
func GetDeviMonSnapshot(c *gin.Context) {
	state, err := service.GetDeviMonState()
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("failed to load devin monitor state: %v", err))
		common.ApiErrorStatus(c, http.StatusInternalServerError, common.NewMessage("Failed to load the Devin monitor"))
		return
	}
	common.ApiSuccess(c, state)
}

// A manual request only queues work. Closing the page cannot cancel the job.
func RunDeviMonCollection(c *gin.Context) {
	if service.DeviMonKey() == "" {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Set CPA_MANAGEMENT_KEY before collecting"))
		return
	}
	if _, _, err := service.EnqueueSystemTask(model.SystemTaskTypeDevinMonitor, nil); err != nil {
		logger.LogError(c.Request.Context(), common.LogText("devin monitor collection failed: %v", err))
		common.ApiErrorStatus(c, http.StatusInternalServerError, common.NewMessage("Could not collect the Devin monitor data"))
		return
	}
	GetDeviMonSnapshot(c)
}

func GetDeviMonSettingsHandler(c *gin.Context) {
	common.ApiSuccess(c, operation_setting.GetDeviMonSettings())
}

func SetDeviMonSettingsHandler(c *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, deviMonMaxRequestSize))
	if err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid monitor configuration"))
		return
	}
	settings, err := operation_setting.ParseDeviMonSettings(string(body))
	if err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, err)
		return
	}
	encoded, err := common.Marshal(settings)
	if err != nil {
		common.ApiErrorStatus(c, http.StatusInternalServerError, common.NewMessage("Could not save monitor configuration"))
		return
	}
	if err := model.UpdateOption(operation_setting.DeviMonOptionKey, string(encoded)); err != nil {
		logger.LogError(c.Request.Context(), common.LogText("failed to save devin monitor configuration: %v", err))
		common.ApiErrorStatus(c, http.StatusInternalServerError, common.NewMessage("Could not save monitor configuration"))
		return
	}
	common.ApiSuccess(c, settings)
}
