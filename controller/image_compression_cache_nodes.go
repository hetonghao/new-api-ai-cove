package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/requestplugin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const imageCacheNodeTimeout = 2 * time.Second

var imageCacheInstanceID = uuid.NewString()

// Check the named subscription and publish as one Redis operation so an
// overlapping process cannot receive a request we report as unavailable.
var dispatchImageCacheNodeRequest = redis.NewScript(`
local subscribers = redis.call("PUBSUB", "NUMSUB", KEYS[1])[2]
if subscribers ~= 1 then
    return 0
end
redis.call("PUBLISH", KEYS[1], ARGV[1])
return 1
`)

type imageCacheNodeStats struct {
	NodeName   string `json:"node_name"`
	InstanceID string `json:"instance_id"`
	requestplugin.ImageCompressionCacheStats
}

type imageCacheNodeRequest struct {
	ReplyID   uuid.UUID
	ExpiresAt int64
	Page      int
	PageSize  int
	Clear     *imageCacheClearRequest
}

type imageCacheNodeResponse struct {
	Status  int
	Stats   *imageCacheNodeStats `json:",omitempty"`
	Deleted int
}

func imageCacheNodeChannel(client *redis.Client, nodeName string) string {
	return "image-compression-cache:v1:" + strconv.Itoa(client.Options().DB) + ":node:" + nodeName
}

func imageCacheReplyChannel(client *redis.Client, id uuid.UUID) string {
	return "image-compression-cache:v1:" + strconv.Itoa(client.Options().DB) + ":reply:" + id.String()
}

func executeImageCacheNodeRequest(node string, request imageCacheNodeRequest) imageCacheNodeResponse {
	if request.Clear != nil {
		if !request.Clear.valid() || request.Clear.NodeName != "" && request.Clear.NodeName != node {
			return imageCacheNodeResponse{Status: http.StatusBadRequest}
		}
		if request.Clear.InstanceID != imageCacheInstanceID {
			return imageCacheNodeResponse{Status: http.StatusConflict}
		}
		return imageCacheNodeResponse{Status: http.StatusOK, Deleted: requestplugin.RemoveImageCompressionCacheEntries(request.Clear.IDs, request.Clear.All)}
	}
	if !validImageCachePage(request.Page, request.PageSize) {
		return imageCacheNodeResponse{Status: http.StatusBadRequest}
	}
	return imageCacheNodeResponse{Status: http.StatusOK, Stats: &imageCacheNodeStats{
		NodeName: node, InstanceID: imageCacheInstanceID,
		ImageCompressionCacheStats: requestplugin.GetImageCompressionCacheSnapshot(request.Page, request.PageSize),
	}}
}

func callImageCacheNode(ctx context.Context, target string, request imageCacheNodeRequest) imageCacheNodeResponse {
	if target == "" || target == common.NodeName {
		return executeImageCacheNodeRequest(common.NodeName, request)
	}
	unavailable := imageCacheNodeResponse{Status: http.StatusServiceUnavailable}
	if len(target) > 128 {
		return imageCacheNodeResponse{Status: http.StatusNotFound}
	}
	ctx, cancel := context.WithTimeout(ctx, imageCacheNodeTimeout)
	defer cancel()
	instance, err := model.GetSystemInstance(ctx, target)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return imageCacheNodeResponse{Status: http.StatusNotFound}
	}
	if err != nil {
		logger.LogError(ctx, common.LogText("failed to locate image compression cache node: %v", err))
		return unavailable
	}
	if instance.NodeName != target {
		return imageCacheNodeResponse{Status: http.StatusNotFound}
	}
	if common.GetTimestamp()-instance.LastSeenAt > model.SystemInstanceStaleAfterSeconds {
		return unavailable
	}
	if !common.RedisEnabled || common.RDB == nil {
		return unavailable
	}
	response, err := requestImageCacheNode(ctx, common.RDB, target, request)
	if err != nil {
		logger.LogWarn(ctx, common.LogText("image compression cache node did not respond: %v", err))
		return unavailable
	}
	if request.Clear == nil && response.Status == http.StatusOK && (response.Stats == nil || response.Stats.NodeName != target || response.Stats.InstanceID == "") {
		return unavailable
	}
	return response
}

func requestImageCacheNode(ctx context.Context, client *redis.Client, target string, request imageCacheNodeRequest) (imageCacheNodeResponse, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return imageCacheNodeResponse{Status: http.StatusServiceUnavailable}, nil
	}
	request.ReplyID = uuid.New()
	request.ExpiresAt = deadline.UnixMilli()
	payload, err := common.Marshal(request)
	if err != nil {
		return imageCacheNodeResponse{}, err
	}
	subscription := client.Subscribe(ctx, imageCacheReplyChannel(client, request.ReplyID))
	defer subscription.Close()
	// Subscribe must be acknowledged before publishing a request that can reply immediately.
	if _, err := subscription.Receive(ctx); err != nil {
		return imageCacheNodeResponse{}, err
	}
	dispatched, err := dispatchImageCacheNodeRequest.Run(ctx, client, []string{imageCacheNodeChannel(client, target)}, payload).Int()
	if err != nil {
		return imageCacheNodeResponse{}, err
	}
	if dispatched != 1 {
		return imageCacheNodeResponse{Status: http.StatusServiceUnavailable}, nil
	}
	result, err := subscription.ReceiveMessage(ctx)
	if err != nil {
		return imageCacheNodeResponse{}, err
	}
	var response imageCacheNodeResponse
	if err := common.UnmarshalJsonStr(result.Payload, &response); err != nil {
		return imageCacheNodeResponse{}, err
	}
	return response, nil
}

// ServeImageCompressionCacheNodes handles metadata and removal for this process.
func ServeImageCompressionCacheNodes(ctx context.Context) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	serveImageCacheNode(ctx, common.RDB, common.NodeName)
}

func serveImageCacheNode(ctx context.Context, client *redis.Client, node string) {
	subscription := client.Subscribe(ctx, imageCacheNodeChannel(client, node))
	defer subscription.Close()
	messages := subscription.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case message, open := <-messages:
			if !open {
				return
			}
			if len(message.Payload) > 2*imageCacheMaxRequestSize {
				continue
			}
			var request imageCacheNodeRequest
			if err := common.UnmarshalJsonStr(message.Payload, &request); err != nil || request.ReplyID == uuid.Nil {
				continue
			}
			now := time.Now()
			deadline := time.UnixMilli(request.ExpiresAt)
			if !deadline.After(now) || deadline.After(now.Add(2*imageCacheNodeTimeout)) {
				continue
			}
			response := executeImageCacheNodeRequest(node, request)
			payload, err := common.Marshal(response)
			if err != nil {
				logger.LogError(ctx, common.LogText("failed to encode image compression cache node response: %v", err))
				continue
			}
			replyCtx, cancel := context.WithDeadline(ctx, deadline)
			if err := client.Publish(replyCtx, imageCacheReplyChannel(client, request.ReplyID), payload).Err(); err != nil && replyCtx.Err() == nil {
				logger.LogWarn(replyCtx, common.LogText("failed to publish image compression cache node response: %v", err))
			}
			cancel()
		}
	}
}
