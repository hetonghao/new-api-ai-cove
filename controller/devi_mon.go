package controller

import (
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
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

func RefreshDeviMonAccount(c *gin.Context) {
	var body struct {
		Email string `json:"email"`
	}
	if err := common.DecodeJsonBody(http.MaxBytesReader(c.Writer, c.Request.Body, deviMonMaxRequestSize), &body); err != nil || strings.TrimSpace(body.Email) == "" || len(body.Email) > 254 {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid email address"))
		return
	}
	if service.DeviMonKey() == "" {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Set CPA_MANAGEMENT_KEY before collecting"))
		return
	}
	if err := service.RefreshDeviMonAccount(c.Request.Context(), body.Email); err != nil {
		if errors.Is(err, service.ErrDeviMonAccountMissing) {
			common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Account no longer in the monitor inventory"))
			return
		}
		logger.LogError(c.Request.Context(), common.LogText("devin monitor: account refresh failed: %v", err))
		common.ApiErrorStatus(c, http.StatusBadGateway, common.NewMessage("Refresh failed. Showing the last successful snapshot."))
		return
	}
	common.ApiSuccess(c, nil)
}

func SetDeviMonAccountAutoReset(c *gin.Context) {
	var body struct {
		Email            string `json:"email"`
		ResetKey         string `json:"reset_key"`
		Enabled          *bool  `json:"enabled"`
		EnableAfterReset *bool  `json:"enable_after_reset"`
	}
	if err := common.DecodeJsonBody(http.MaxBytesReader(c.Writer, c.Request.Body, deviMonMaxRequestSize), &body); err != nil ||
		strings.TrimSpace(body.Email) == "" || len(body.Email) > 254 || (body.Enabled == nil) == (body.EnableAfterReset == nil) || len(body.ResetKey) != 64 {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid monitor configuration"))
		return
	}
	if _, err := hex.DecodeString(body.ResetKey); err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid monitor configuration"))
		return
	}
	if err := service.SetDeviMonAccountPolicy(c.Request.Context(), body.Email, body.ResetKey, body.Enabled, body.EnableAfterReset); err != nil {
		if errors.Is(err, service.ErrDeviMonAccountMissing) {
			common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Account no longer in the monitor inventory"))
			return
		}
		logger.LogError(c.Request.Context(), common.LogText("devin monitor: could not save auto reset: %v", err))
		common.ApiErrorStatus(c, http.StatusBadGateway, common.NewMessage("Could not save monitor configuration"))
		return
	}
	common.ApiSuccess(c, nil)
}

func ConfirmDeviMonAccountReset(c *gin.Context) {
	var body struct {
		Email   string `json:"email"`
		OfferID string `json:"offer_id"`
	}
	if err := common.DecodeJsonBody(http.MaxBytesReader(c.Writer, c.Request.Body, deviMonMaxRequestSize), &body); err != nil ||
		strings.TrimSpace(body.Email) == "" || len(body.Email) > 254 || uuid.Validate(body.OfferID) != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid reset card confirmation"))
		return
	}
	account, err := service.ConfirmDeviMonAccountReset(c.Request.Context(), body.Email, body.OfferID)
	if err != nil {
		if errors.Is(err, service.ErrDeviMonResetOfferChanged) {
			common.ApiErrorStatus(c, http.StatusConflict, common.NewMessage("The reset offer has changed. Refresh the account before trying again."))
			return
		}
		logger.LogError(c.Request.Context(), common.LogText("devin monitor: reset card confirmation failed: %v", err))
		common.ApiErrorStatus(c, http.StatusBadGateway, common.NewMessage("Could not confirm the reset. Refresh the account to check its usage."))
		return
	}
	common.ApiSuccess(c, account)
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

// SetDeviMonDevinStatus mirrors cpa-pro-1's enable/disable toggle. It changes
// the Devin credential itself, independently of the reading channel.
func SetDeviMonDevinStatus(c *gin.Context) {
	var body struct {
		Email    string `json:"email"`
		Disabled *bool  `json:"disabled"`
	}
	if err := common.DecodeJsonBody(http.MaxBytesReader(c.Writer, c.Request.Body, deviMonMaxRequestSize), &body); err != nil ||
		strings.TrimSpace(body.Email) == "" || len(body.Email) > 254 || body.Disabled == nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid account"))
		return
	}
	if err := service.SetDeviMonDevinDisabled(c.Request.Context(), body.Email, *body.Disabled); err != nil {
		if errors.Is(err, service.ErrDeviMonDevinAccountMissing) {
			common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Account no longer in the Devin inventory"))
			return
		}
		logger.LogError(c.Request.Context(), common.LogText("devin monitor: could not change the Devin status: %v", err))
		common.ApiErrorStatus(c, http.StatusBadGateway, common.NewMessage("Could not change the Devin status"))
		return
	}
	common.ApiSuccess(c, nil)
}

// Shared between monitors on this node; reading CPA's inventory never probes quota.
func GetDeviMonDevinInventory(c *gin.Context) {
	data, err := service.GetDeviMonLiveInventory(c.Request.Context())
	if err != nil {
		common.ApiErrorStatus(c, http.StatusBadGateway, common.NewMessage("Could not load Devin request history"))
		return
	}
	common.ApiSuccess(c, data)
}

func SetDeviMonRouting(c *gin.Context) {
	var body struct {
		Email string `json:"email"`
		Field string `json:"field"`
		Value *int64 `json:"value"`
	}
	if err := common.DecodeJsonBody(http.MaxBytesReader(c.Writer, c.Request.Body, deviMonMaxRequestSize), &body); err != nil || strings.TrimSpace(body.Email) == "" || len(body.Email) > 254 || body.Value == nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid Devin routing value"))
		return
	}
	err := service.SetDeviMonRouting(c.Request.Context(), body.Email, body.Field, *body.Value)
	switch {
	case errors.Is(err, service.ErrDeviMonInvalidRouting):
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid Devin routing value"))
	case errors.Is(err, service.ErrDeviMonDevinAccountMissing):
		common.ApiErrorStatus(c, http.StatusNotFound, common.NewMessage("Account no longer in the Devin inventory"))
	case err != nil:
		common.ApiErrorStatus(c, http.StatusBadGateway, common.NewMessage("Could not update Devin routing"))
	default:
		common.ApiSuccess(c, nil)
	}
}

// --- ChatGPT session channel ---------------------------------------------

// The pasted /api/auth/session payload is a credential, so it is capped and
// never echoed back in an error message.
const deviMonSessionMaxSize = 256 << 10

// SetDeviMonAccountProxy stores one account's own egress proxy.
func SetDeviMonAccountProxy(c *gin.Context) {
	var body struct {
		Email    string `json:"email"`
		ProxyURL string `json:"proxy_url"`
	}
	if err := common.DecodeJsonBody(http.MaxBytesReader(c.Writer, c.Request.Body, deviMonMaxRequestSize), &body); err != nil ||
		strings.TrimSpace(body.Email) == "" || len(body.Email) > 254 || len(body.ProxyURL) > 512 {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid account"))
		return
	}
	if err := service.SetDeviMonAccountProxy(c.Request.Context(), body.Email, body.ProxyURL); err != nil {
		if errors.Is(err, service.ErrDeviMonChatGPTAccountMissing) {
			common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Save a ChatGPT session before setting a proxy"))
			return
		}
		if errors.Is(err, service.ErrDeviMonInvalidProxy) {
			common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage(err.Error()))
			return
		}
		logger.LogError(c.Request.Context(), common.LogText("devin monitor: could not save the account proxy: %v", err))
		common.ApiErrorStatus(c, http.StatusBadGateway, common.NewMessage("Could not save the proxy address"))
		return
	}
	common.ApiSuccess(c, nil)
}

// SaveDeviMonChatGPTSession stores a session pasted from an already signed-in
// browser. The token is verified against ChatGPT before it is written.
func SaveDeviMonChatGPTSession(c *gin.Context) {
	raw, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, deviMonSessionMaxSize))
	if err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Could not read the session JSON"))
		return
	}
	account, err := service.DeviMonSaveSession(c.Request.Context(), raw)
	if err != nil {
		if errors.Is(err, service.ErrDeviMonInvalidSession) {
			common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage(err.Error()))
			return
		}
		logger.LogError(c.Request.Context(), common.LogText("devin monitor: could not store the ChatGPT session: %v", err))
		common.ApiErrorStatus(c, http.StatusBadGateway, common.NewMessage("Could not verify the ChatGPT session"))
		return
	}
	common.ApiSuccess(c, gin.H{"email": account.Email, "account_id": account.AccountID})
}

func GetDeviMonAccounts(c *gin.Context) {
	accounts, err := service.GetDeviMonChatGPTAccounts(c.Request.Context())
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("devin monitor: could not list sign-ins: %v", err))
		common.ApiErrorStatus(c, http.StatusInternalServerError, common.NewMessage("Could not load the account channels"))
		return
	}
	common.ApiSuccess(c, accounts)
}

// SetDeviMonAccountChannel switches one account between the CPA inventory and
// the gateway-stored ChatGPT sign-in.
func SetDeviMonAccountChannel(c *gin.Context) {
	var body struct {
		Email   string `json:"email"`
		Channel string `json:"channel"`
	}
	if err := common.DecodeJsonBody(http.MaxBytesReader(c.Writer, c.Request.Body, deviMonMaxRequestSize), &body); err != nil ||
		strings.TrimSpace(body.Email) == "" || len(body.Email) > 254 {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid account"))
		return
	}
	if err := service.SetDeviMonAccountChannel(c.Request.Context(), body.Email, body.Channel); err != nil {
		if errors.Is(err, service.ErrDeviMonChatGPTAccountMissing) {
			common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Sign in to ChatGPT before selecting that channel"))
			return
		}
		logger.LogError(c.Request.Context(), common.LogText("devin monitor: could not switch account channel: %v", err))
		common.ApiErrorStatus(c, http.StatusBadGateway, common.NewMessage("Could not change the subscription channel"))
		return
	}
	common.ApiSuccess(c, nil)
}

// ForgetDeviMonChatGPTAccount removes the stored sign-in.
func ForgetDeviMonChatGPTAccount(c *gin.Context) {
	var body struct {
		Email string `json:"email"`
	}
	if err := common.DecodeJsonBody(http.MaxBytesReader(c.Writer, c.Request.Body, deviMonMaxRequestSize), &body); err != nil ||
		strings.TrimSpace(body.Email) == "" || len(body.Email) > 254 {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid account"))
		return
	}
	if err := service.ForgetDeviMonChatGPTAccount(c.Request.Context(), body.Email); err != nil {
		logger.LogError(c.Request.Context(), common.LogText("devin monitor: could not forget sign-in: %v", err))
		common.ApiErrorStatus(c, http.StatusBadGateway, common.NewMessage("Could not remove the ChatGPT sign-in"))
		return
	}
	common.ApiSuccess(c, nil)
}
