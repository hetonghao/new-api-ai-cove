package requestplugin

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image/jpeg"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/requestpatch"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/go-redis/redis/v8"
)

const (
	imageEncoderVersion  = 2
	imageCachePrefix     = "ai-cove:img-cache:v1"
	imageCacheBudget     = 150 * time.Millisecond
	imageCacheRetryDelay = 2 * time.Second
)

//go:embed image_compression_cache.lua
var imageCacheLua string
var (
	imageCacheScript  = redis.NewScript(imageCacheLua)
	imageBase64Unwrap = strings.NewReplacer("\r", "", "\n", "")
)

var ErrImageCacheUnavailable = errors.New("shared image compression cache is unavailable")
var ErrImageCacheChanged = errors.New("shared image compression cache changed")

type ImageCacheStatus string

const (
	ImageCacheReady         ImageCacheStatus = "ready"
	ImageCacheNotConfigured ImageCacheStatus = "not_configured"
	ImageCacheUnavailable   ImageCacheStatus = "unavailable"
	ImageCacheSynchronizing ImageCacheStatus = "synchronizing"
)

type imageCompressionCache struct {
	client     *redis.Client
	retryAfter atomic.Int64
}

var sharedImageCompressionCache atomic.Pointer[imageCompressionCache]

// ConfigureImageCompressionCacheRedis only configures a separate endpoint. It
// does not require Redis to be reachable for New API to start.
func ConfigureImageCompressionCacheRedis(url string) error {
	var cache *imageCompressionCache
	var configErr error
	if url != "" {
		options, err := redis.ParseURL(url)
		if err != nil || (common.RDB != nil && options.Addr == common.RDB.Options().Addr) {
			configErr = errors.New("IMAGE_COMPRESSION_CACHE_REDIS_URL must point to a dedicated Redis instance")
		} else {
			options.DialTimeout = imageCacheBudget
			options.ReadTimeout = imageCacheBudget
			options.WriteTimeout = imageCacheBudget
			options.PoolTimeout = imageCacheBudget
			options.MaxRetries = -1
			options.PoolSize = 8
			cache = &imageCompressionCache{client: redis.NewClient(options)}
		}
	}
	if previous := sharedImageCompressionCache.Swap(cache); previous != nil {
		return errors.Join(configErr, previous.client.Close())
	}
	return configErr
}

func imageCompressionKey(userID int, item inlineMedia, effect requestpatch.Effect) string {
	quality := 0
	if effect.Format == "jpeg" || effect.Format == "auto" {
		quality = effect.Quality
		if quality == 0 {
			quality = jpeg.DefaultQuality
		}
	}
	// Use the canonical representation of standard Base64, ignoring line wraps
	// and unused padding bits accepted by the same decoder as encodeMedia.
	data := item.data
	if strings.ContainsAny(data, "\r\n") {
		data = imageBase64Unwrap.Replace(data)
	}
	source := sha256.Sum256([]byte(data))
	if len(data) >= 4 && data[len(data)-1] == '=' {
		start := len(data) - 4
		var terminal [3]byte
		if size, err := base64.StdEncoding.Decode(terminal[:], []byte(data[start:])); err == nil {
			var normalized [4]byte
			base64.StdEncoding.Encode(normalized[:], terminal[:size])
			if string(normalized[:]) != data[start:] {
				source = sha256.Sum256([]byte(data[:start] + string(normalized[:])))
			}
		}
	}
	mime := sha256.Sum256([]byte(item.mime))
	key := sha256.Sum256(fmt.Appendf(nil, "%d:%d:%x:%x:%s:%d:%d", imageEncoderVersion, userID, source, mime, effect.Format, quality, effect.MaxSide))
	return hex.EncodeToString(key[:])
}

type imageReencodeResult struct {
	data                         []byte
	originalMIME, compressedMIME string
	originalBytes                int
	cacheHit                     bool
}

type ImageCompressionCacheEntry struct {
	ID              string `json:"id"`
	UserID          int    `json:"user_id"`
	OriginalMIME    string `json:"original_mime"`
	CompressedMIME  string `json:"compressed_mime"`
	OriginalBytes   int    `json:"original_bytes"`
	CompressedBytes int    `json:"compressed_bytes"`
	Hits            uint64 `json:"hits"`
	CreatedAt       int64  `json:"created_at"`
	LastHitAt       int64  `json:"last_hit_at"`
	ExpiresAt       int64  `json:"expires_at"`
}

type ImageCacheNodeStats struct {
	NodeName string `json:"node_name"`
	Hits     uint64 `json:"hits"`
	Misses   uint64 `json:"misses"`
}

type ImageCompressionCacheStats struct {
	operation_setting.ImageCompressionCacheSettings
	Status      ImageCacheStatus             `json:"status"`
	CacheID     string                       `json:"cache_id,omitempty"`
	UsedBytes   int64                        `json:"used_bytes"`
	EntryCount  int                          `json:"entry_count"`
	Hits        uint64                       `json:"hits"`
	Misses      uint64                       `json:"misses"`
	Evictions   uint64                       `json:"evictions"`
	Expirations uint64                       `json:"expirations"`
	Removals    uint64                       `json:"removals"`
	Page        int                          `json:"page"`
	PageSize    int                          `json:"page_size"`
	Entries     []ImageCompressionCacheEntry `json:"entries"`
	Nodes       []ImageCacheNodeStats        `json:"nodes"`
}

func (c *imageCompressionCache) run(parent context.Context, settings operation_setting.ImageCompressionCacheSettings, op string, args ...any) ([]any, bool) {
	ctx, cancel := context.WithTimeout(parent, imageCacheBudget)
	defer cancel()
	enabled := "0"
	if settings.Enabled {
		enabled = "1"
	}
	values, err := imageCacheScript.Run(ctx, c.client, []string{imageCachePrefix}, append([]any{op, settings.Revision, enabled, settings.MaxBytes, settings.TTLSeconds, common.GetUUID()}, args...)...).Slice()
	if err != nil || len(values) == 0 {
		// A disconnected client does not mean the cache is unavailable for others.
		if parent.Err() == nil {
			c.retryAfter.Store(time.Now().Add(imageCacheRetryDelay).UnixNano())
		}
		return nil, false
	}
	c.retryAfter.Store(0)
	return values, true
}

func (c *imageCompressionCache) bypassed() bool {
	return c.retryAfter.Load() > time.Now().UnixNano()
}

func ApplyImageCompressionCacheSettings(ctx context.Context) ImageCacheStatus {
	cache := sharedImageCompressionCache.Load()
	if cache == nil {
		return ImageCacheNotConfigured
	}
	result, ok := cache.run(ctx, operation_setting.GetImageCompressionCacheSettings(), "configure")
	return imageCacheResponseStatus(result, ok)
}

func imageCacheResponseStatus(result []any, ok bool) ImageCacheStatus {
	if !ok {
		return ImageCacheUnavailable
	}
	if redisString(result[0]) == "" {
		return ImageCacheSynchronizing
	}
	return ImageCacheReady
}

func redisString(value any) string {
	text, _ := value.(string)
	return text
}

func redisCount(value any) uint64 {
	switch number := value.(type) {
	case string:
		parsed, _ := strconv.ParseUint(number, 10, 64)
		return parsed
	case int64:
		return uint64(max(0, number))
	default:
		return 0
	}
}

func decodeCacheNodeCounters(fields []any) []ImageCacheNodeStats {
	nodes := make([]ImageCacheNodeStats, 0)
	indexes := make(map[string]int)
	for i := 0; i+1 < len(fields); i += 2 {
		field := redisString(fields[i])
		split := strings.LastIndexByte(field, ':')
		if split < 1 {
			continue
		}
		name, kind := field[:split], field[split+1:]
		index, exists := indexes[name]
		if !exists {
			index = len(nodes)
			indexes[name] = index
			nodes = append(nodes, ImageCacheNodeStats{NodeName: name})
		}
		if kind == "hits" {
			nodes[index].Hits = redisCount(fields[i+1])
		}
		if kind == "misses" {
			nodes[index].Misses = redisCount(fields[i+1])
		}
	}
	slices.SortFunc(nodes, func(a, b ImageCacheNodeStats) int { return strings.Compare(a.NodeName, b.NodeName) })
	return nodes
}

func GetImageCompressionCacheSnapshot(ctx context.Context, page, pageSize int) ImageCompressionCacheStats {
	stats := ImageCompressionCacheStats{
		ImageCompressionCacheSettings: operation_setting.GetImageCompressionCacheSettings(),
		Status:                        ImageCacheNotConfigured, Page: page, PageSize: pageSize,
		Entries: []ImageCompressionCacheEntry{}, Nodes: []ImageCacheNodeStats{},
	}
	cache := sharedImageCompressionCache.Load()
	if cache == nil {
		return stats
	}
	reply, ok := cache.run(ctx, stats.ImageCompressionCacheSettings, "snapshot", (page-1)*pageSize, pageSize)
	stats.Status = imageCacheResponseStatus(reply, ok)
	if stats.Status != ImageCacheReady {
		return stats
	}
	if !stats.readSnapshot(reply) {
		stats.Status = ImageCacheUnavailable
	}
	return stats
}

func (stats *ImageCompressionCacheStats) readSnapshot(reply []any) bool {
	if len(reply) != 5 {
		return false
	}
	counts, countsOK := reply[1].([]any)
	rows, rowsOK := reply[3].([]any)
	nodes, nodesOK := reply[4].([]any)
	if !countsOK || len(counts) != 6 || !rowsOK || !nodesOK {
		return false
	}
	for _, raw := range rows {
		var row ImageCompressionCacheEntry
		if err := common.UnmarshalJsonStr(redisString(raw), &row); err != nil || row.ID == "" {
			return false
		}
		stats.Entries = append(stats.Entries, row)
	}
	stats.CacheID = redisString(reply[0])
	stats.UsedBytes = int64(redisCount(counts[0]))
	stats.Hits, stats.Misses = redisCount(counts[1]), redisCount(counts[2])
	stats.Evictions, stats.Expirations, stats.Removals = redisCount(counts[3]), redisCount(counts[4]), redisCount(counts[5])
	stats.EntryCount = int(redisCount(reply[2]))
	stats.Nodes = decodeCacheNodeCounters(nodes)
	return true
}

func RemoveImageCompressionCacheEntries(ctx context.Context, cacheID string, ids []string, all bool) (int, error) {
	cache := sharedImageCompressionCache.Load()
	if cache == nil {
		return 0, ErrImageCacheUnavailable
	}
	clearAll := "0"
	if all {
		clearAll = "1"
	}
	args := []any{cacheID, clearAll}
	for _, id := range ids {
		args = append(args, id)
	}
	reply, ok := cache.run(ctx, operation_setting.GetImageCompressionCacheSettings(), "remove", args...)
	switch imageCacheResponseStatus(reply, ok) {
	case ImageCacheSynchronizing:
		return 0, ErrImageCacheChanged
	case ImageCacheReady:
		if len(reply) == 2 {
			if removed, ok := reply[1].(int64); ok {
				if removed >= 0 {
					return int(removed), nil
				}
				return 0, ErrImageCacheChanged
			}
		}
	}
	return 0, ErrImageCacheUnavailable
}
