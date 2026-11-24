package controller

import (
	"context"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
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
	common.ApiSuccess(c, settings)
}

type imageCacheNode struct {
	Name   string `json:"node_name"`
	Status string `json:"status"`
}

func ListImageCompressionCacheNodes(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), imageCacheNodeTimeout)
	defer cancel()
	instances, err := model.ListSystemInstancesContext(ctx)
	if err != nil {
		logger.LogError(c.Request.Context(), common.LogText("failed to list image compression cache nodes: %v", err))
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Could not list image compression cache nodes"))
		return
	}
	nodes := []imageCacheNode{{Name: common.NodeName, Status: model.SystemInstanceStatusOnline}}
	now := common.GetTimestamp()
	var remoteChannels []string
	for _, instance := range instances {
		if instance.NodeName == common.NodeName {
			continue
		}
		status := model.SystemInstanceStatusOnline
		if now-instance.LastSeenAt > model.SystemInstanceStaleAfterSeconds {
			status = model.SystemInstanceStatusStale
		} else {
			status = "unavailable"
			if common.RedisEnabled && common.RDB != nil {
				remoteChannels = append(remoteChannels, imageCacheNodeChannel(common.RDB, instance.NodeName))
			}
		}
		nodes = append(nodes, imageCacheNode{Name: instance.NodeName, Status: status})
	}
	if len(remoteChannels) > 0 {
		counts, err := common.RDB.PubSubNumSub(ctx, remoteChannels...).Result()
		if err != nil {
			logger.LogWarn(ctx, common.LogText("failed to check image compression cache node availability: %v", err))
		} else {
			for i := 1; i < len(nodes); i++ {
				node := &nodes[i]
				if node.Status == "unavailable" && counts[imageCacheNodeChannel(common.RDB, node.Name)] == 1 {
					node.Status = model.SystemInstanceStatusOnline
				}
			}
		}
	}
	common.ApiSuccess(c, gin.H{"local_node_name": common.NodeName, "nodes": nodes})
}

func GetImageCompressionCacheStats(c *gin.Context) {
	page, pageErr := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, sizeErr := strconv.Atoi(c.DefaultQuery("page_size", "25"))
	if pageErr != nil || sizeErr != nil || !validImageCachePage(page, pageSize) {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid image compression cache page"))
		return
	}
	request := imageCacheNodeRequest{Page: page, PageSize: pageSize}
	response := callImageCacheNode(c.Request.Context(), c.Query("node_name"), request)
	writeImageCacheResponse(c, response, response.Stats)
}

func ClearImageCompressionCache(c *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, imageCacheMaxRequestSize))
	var request imageCacheClearRequest
	if err != nil || common.Unmarshal(body, &request) != nil || !request.valid() {
		common.ApiErrorStatus(c, http.StatusBadRequest, common.NewMessage("Invalid image compression cache removal"))
		return
	}
	response := callImageCacheNode(c.Request.Context(), request.NodeName, imageCacheNodeRequest{Clear: &request})
	writeImageCacheResponse(c, response, gin.H{"deleted": response.Deleted})
}

func validImageCachePage(page, pageSize int) bool {
	return page > 0 && pageSize > 0 && pageSize <= 100 && page <= math.MaxInt/pageSize
}

type imageCacheClearRequest struct {
	NodeName   string   `json:"node_name"`
	InstanceID string   `json:"instance_id"`
	IDs        []string `json:"ids"`
	All        bool     `json:"all"`
}

func (request imageCacheClearRequest) valid() bool {
	if request.InstanceID == "" || len(request.InstanceID) > 128 || request.All == (len(request.IDs) > 0) || len(request.IDs) > imageCacheMaxSelection {
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

func writeImageCacheResponse(c *gin.Context, response imageCacheNodeResponse, data any) {
	switch response.Status {
	case http.StatusBadRequest:
		common.ApiErrorStatus(c, response.Status, common.NewMessage("Invalid image compression cache removal"))
	case http.StatusOK:
		common.ApiSuccess(c, data)
	case http.StatusConflict:
		common.ApiErrorStatus(c, response.Status, common.NewMessage("Image compression cache instance changed. Refresh the statistics and try again"))
	case http.StatusNotFound:
		common.ApiErrorStatus(c, response.Status, common.NewMessage("Image compression cache node was not found"))
	default:
		common.ApiErrorStatus(c, http.StatusServiceUnavailable, common.NewMessage("Image compression cache node is unavailable"))
	}
}
