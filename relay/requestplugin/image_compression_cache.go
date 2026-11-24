package requestplugin

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"image/jpeg"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/requestpatch"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

const (
	imageCompressionCacheEntryLimit = 4096
	imageEncoderVersion             = 1
)

type imageCompressionCacheKey struct {
	userID           int
	source, mime     [sha256.Size]byte
	version          int
	format           string
	quality, maxSide int
}

func imageCompressionKey(userID int, item inlineMedia, effect requestpatch.Effect) imageCompressionCacheKey {
	quality := 0
	if effect.Format == "jpeg" || effect.Format == "auto" {
		quality = effect.Quality
		if quality == 0 {
			quality = jpeg.DefaultQuality
		}
	}
	return imageCompressionCacheKey{
		userID: userID, source: sha256.Sum256([]byte(item.data)), mime: sha256.Sum256([]byte(item.mime)),
		version: imageEncoderVersion, format: effect.Format, quality: quality, maxSide: effect.MaxSide,
	}
}

type imageReencodeResult struct {
	data                         []byte
	originalMIME, compressedMIME string
	originalBytes                int
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

type ImageCompressionCacheStats struct {
	operation_setting.ImageCompressionCacheSettings
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
}

type cachedImageCompression struct {
	key                  imageCompressionCacheKey
	result               imageReencodeResult
	row                  ImageCompressionCacheEntry
	createdAt, expiresAt time.Time
}

type imageCompressionCache struct {
	mu          sync.Mutex
	now         func() time.Time
	settings    operation_setting.ImageCompressionCacheSettings
	entries     map[imageCompressionCacheKey]*list.Element
	lru         list.List
	expiryTimer *time.Timer
	generation  uint64
	stats       ImageCompressionCacheStats
}

var localImageCompressionCache = imageCompressionCache{now: time.Now}

func init() {
	operation_setting.RegisterImageCompressionCacheSettingsChange(localImageCompressionCache.settingsChanged)
}

func (c *imageCompressionCache) settingsChanged() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.syncSettings(c.now())
}

// syncSettings reconciles the latest settings while holding the cache lock.
func (c *imageCompressionCache) syncSettings(now time.Time) {
	settings := operation_setting.GetImageCompressionCacheSettings()
	if settings == c.settings {
		return
	}
	c.settings = settings
	c.generation++
	if !settings.Enabled {
		c.removeAll()
		return
	}
	c.pruneExpired(now)
	for node := c.lru.Front(); node != nil; node = node.Next() {
		entry := node.Value.(*cachedImageCompression)
		entry.expiresAt = entry.createdAt.Add(time.Duration(settings.TTLSeconds) * time.Second)
		entry.row.ExpiresAt = entry.expiresAt.Unix()
	}
	c.pruneExpired(now)
	c.enforceLimits(0)
	c.scheduleExpiry(now)
}

func (c *imageCompressionCache) lookup(key imageCompressionCacheKey) (imageReencodeResult, bool, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.syncSettings(now)
	if !c.settings.Enabled {
		return imageReencodeResult{}, false, 0
	}
	if node := c.entries[key]; node != nil {
		entry := node.Value.(*cachedImageCompression)
		if now.Before(entry.expiresAt) {
			c.lru.MoveToFront(node)
			c.stats.Hits++
			entry.row.Hits++
			entry.row.LastHitAt = now.Unix()
			return entry.result, true, c.generation
		}
		c.remove(node)
		c.stats.Expirations++
	}
	c.stats.Misses++
	return imageReencodeResult{}, false, c.generation
}

func (c *imageCompressionCache) store(key imageCompressionCacheKey, result imageReencodeResult, generation uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.syncSettings(now)
	if !c.settings.Enabled || generation != c.generation || c.entries[key] != nil || int64(len(result.data)) > c.settings.MaxBytes {
		return
	}
	c.enforceLimits(int64(len(result.data)))
	result.data = bytes.Clone(result.data)
	expiresAt := now.Add(time.Duration(c.settings.TTLSeconds) * time.Second)
	entry := &cachedImageCompression{key: key, result: result, createdAt: now, expiresAt: expiresAt, row: ImageCompressionCacheEntry{
		ID: common.GetUUID(), UserID: key.userID, OriginalMIME: result.originalMIME, CompressedMIME: result.compressedMIME,
		OriginalBytes: result.originalBytes, CompressedBytes: len(result.data), CreatedAt: now.Unix(), ExpiresAt: expiresAt.Unix(),
	}}
	if c.entries == nil {
		c.entries = make(map[imageCompressionCacheKey]*list.Element)
	}
	c.entries[key] = c.lru.PushFront(entry)
	c.stats.UsedBytes += int64(len(result.data))
	// All new entries have the current TTL; none expires before the pending timer.
	if c.expiryTimer == nil {
		c.scheduleExpiry(now)
	}
}

func (c *imageCompressionCache) enforceLimits(incoming int64) {
	for c.stats.UsedBytes+incoming > c.settings.MaxBytes || len(c.entries) == imageCompressionCacheEntryLimit && incoming > 0 {
		c.remove(c.lru.Back())
		c.stats.Evictions++
	}
}

func (c *imageCompressionCache) remove(node *list.Element) {
	entry := node.Value.(*cachedImageCompression)
	delete(c.entries, entry.key)
	c.lru.Remove(node)
	c.stats.UsedBytes -= int64(len(entry.result.data))
	if len(c.entries) == 0 {
		c.entries = nil
		c.stopTimer()
	}
}

func (c *imageCompressionCache) removeAll() int {
	count := len(c.entries)
	c.lru.Init()
	c.entries = nil
	c.stats.UsedBytes = 0
	c.stats.Removals += uint64(count)
	c.stopTimer()
	return count
}

func (c *imageCompressionCache) pruneExpired(now time.Time) {
	for node := c.lru.Back(); node != nil; {
		prev := node.Prev()
		if !now.Before(node.Value.(*cachedImageCompression).expiresAt) {
			c.remove(node)
			c.stats.Expirations++
		}
		node = prev
	}
}

func (c *imageCompressionCache) stopTimer() {
	if c.expiryTimer != nil {
		c.expiryTimer.Stop()
		c.expiryTimer = nil
	}
}

func (c *imageCompressionCache) scheduleExpiry(now time.Time) {
	c.stopTimer()
	var next time.Time
	for node := c.lru.Front(); node != nil; node = node.Next() {
		expiresAt := node.Value.(*cachedImageCompression).expiresAt
		if next.IsZero() || expiresAt.Before(next) {
			next = expiresAt
		}
	}
	if next.IsZero() {
		return
	}
	var timer *time.Timer
	timer = time.AfterFunc(next.Sub(now), func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.expiryTimer == timer {
			now := c.now()
			c.pruneExpired(now)
			c.scheduleExpiry(now)
		}
	})
	c.expiryTimer = timer
}

func GetImageCompressionCacheSnapshot(page, pageSize int) ImageCompressionCacheStats {
	return localImageCompressionCache.snapshot(page, pageSize)
}

func (c *imageCompressionCache) snapshot(page, pageSize int) ImageCompressionCacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.syncSettings(now)
	c.pruneExpired(now)
	result := c.stats
	result.ImageCompressionCacheSettings = c.settings
	result.EntryCount = len(c.entries)
	result.Page, result.PageSize = max(page, 1), min(max(pageSize, 1), 100)
	result.Entries = make([]ImageCompressionCacheEntry, 0)
	if result.Page-1 > result.EntryCount/result.PageSize {
		return result
	}
	skip := (result.Page - 1) * result.PageSize
	for node := c.lru.Front(); node != nil && len(result.Entries) < result.PageSize; node = node.Next() {
		if skip > 0 {
			skip--
			continue
		}
		result.Entries = append(result.Entries, node.Value.(*cachedImageCompression).row)
	}
	return result
}

func RemoveImageCompressionCacheEntries(ids []string, all bool) int {
	return localImageCompressionCache.removeEntries(ids, all)
}

func (c *imageCompressionCache) removeEntries(ids []string, all bool) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	c.syncSettings(now)
	c.pruneExpired(now)
	if all {
		// An encode already running must not put the cleared content back.
		c.generation++
		return c.removeAll()
	}
	selected := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		selected[id] = struct{}{}
	}
	removed := 0
	for node := c.lru.Back(); node != nil; {
		prev := node.Prev()
		if _, ok := selected[node.Value.(*cachedImageCompression).row.ID]; ok {
			c.remove(node)
			removed++
		}
		node = prev
	}
	if removed > 0 {
		c.generation++
		c.stats.Removals += uint64(removed)
	}
	return removed
}
