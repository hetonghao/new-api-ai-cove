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
	common.ApiSuccess(c, gin.H{
		"snapshot":    service.GetDeviMonSnapshot(),
		"collecting":  service.IsDeviMonCollecting(),
		"next_run_at": service.DeviMonNextRunAt(),
		"configured":  service.DeviMonKey() != "",
	})
}

// RunDeviMonCollection starts one collection synchronously. The page only
// uses it for the manual refresh button.
func RunDeviMonCollection(c *gin.Context) {
	if service.DeviMonKey() == "" {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Set CPA_MANAGEMENT_KEY before collecting"))
		return
	}
	snapshot, err := service.RunDeviMon(c.Request.Context())
	if err != nil && snapshot == nil {
		logger.LogError(c.Request.Context(), common.LogText("devin monitor collection failed: %v", err))
		common.ApiErrorStatus(c, http.StatusBadGateway, common.NewMessage("Could not collect the Devin monitor data"))
		return
	}
	common.ApiSuccess(c, gin.H{"snapshot": snapshot, "collecting": false, "next_run_at": service.DeviMonNextRunAt(), "configured": true})
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
