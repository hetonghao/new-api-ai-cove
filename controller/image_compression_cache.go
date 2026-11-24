package controller

import (
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/requestplugin"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

const (
	imageCacheMaxSelection   = 100
	imageCacheMaxRequestSize = 16 << 10
)

func GetImageCompressionCacheConfig(c *gin.Context) {
	common.ApiSuccess(c, operation_setting.GetImageCompressionCacheSettings())
}

func SetImageCompressionCacheConfig(c *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, imageCacheMaxRequestSize))
	if err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid image compression cache configuration"))
		return
	}
	settings, err := operation_setting.ParseImageCompressionCacheSettings(string(body))
	if err != nil {
		common.ApiErrorStatus(c, http.StatusBadRequest, err)
		return
	}
	if err := model.UpdateImageCompressionCacheSettings(settings); err != nil {
		logger.LogError(c.Request.Context(), common.LogText("failed to save image compression cache configuration: %v", err))
		common.ApiErrorStatus(c, http.StatusInternalServerError, common.NewMessage("Could not save image compression cache configuration"))
		return
	}
	requestplugin.ApplyImageCompressionCacheSettings(c.Request.Context())
	common.ApiSuccess(c, settings)
}

func GetImageCompressionCacheStats(c *gin.Context) {
	page, pageErr := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, sizeErr := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	if pageErr != nil || sizeErr != nil || !validImageCachePage(page, pageSize) {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid image compression cache page"))
		return
	}
	common.ApiSuccess(c, requestplugin.GetImageCompressionCacheSnapshot(c.Request.Context(), page, pageSize))
}

func ClearImageCompressionCache(c *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, imageCacheMaxRequestSize))
	var request imageCacheClearRequest
	if err != nil || common.Unmarshal(body, &request) != nil || !request.valid() {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid image compression cache removal"))
		return
	}
	deleted, err := requestplugin.RemoveImageCompressionCacheEntries(c.Request.Context(), request.CacheID, request.IDs, request.All)
	switch {
	case errors.Is(err, requestplugin.ErrImageCacheChanged):
		common.ApiErrorStatus(c, http.StatusConflict, common.NewMessage("Image compression cache changed. Refresh the statistics and try again"))
	case err != nil:
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Shared image cache Redis is unavailable"))
	default:
		common.ApiSuccess(c, gin.H{"deleted": deleted})
	}
}

func validImageCachePage(page, pageSize int) bool {
	return page > 0 && pageSize > 0 && pageSize <= 100 && page <= math.MaxInt/pageSize
}

type imageCacheClearRequest struct {
	CacheID string   `json:"cache_id"`
	IDs     []string `json:"ids"`
	All     bool     `json:"all"`
}

func (request imageCacheClearRequest) valid() bool {
	if request.CacheID == "" || len(request.CacheID) > 128 || request.All == (len(request.IDs) > 0) || len(request.IDs) > imageCacheMaxSelection {
		return false
	}
	seen := make(map[string]struct{}, len(request.IDs))
	for _, id := range request.IDs {
		if strings.TrimSpace(id) == "" || len(id) > 128 {
			return false
		}
		if _, duplicate := seen[id]; duplicate {
			return false
		}
		seen[id] = struct{}{}
	}
	return true
}
