package requestplugin

import (
	"context"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/requestpatch"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

type imageCacheBatch struct {
	cache      *imageCompressionCache
	settings   operation_setting.ImageCompressionCacheSettings
	generation string
	keys       []string
	results    map[string]imageReencodeResult
	pending    []string
}

func beginImageCacheBatch(ctx context.Context, userID int, media []inlineMedia, effects []requestpatch.Effect) *imageCacheBatch {
	settings := operation_setting.GetImageCompressionCacheSettings()
	cache := sharedImageCompressionCache.Load()
	if userID <= 0 || cache == nil || !settings.Enabled || cache.bypassed() {
		return nil
	}
	var keys []string
	var unique []string
	occurrences := make(map[string]int)
	for index, effect := range effects {
		if effect.Type != requestpatch.EffectMediaReencode || effect.Media < 0 || effect.Media >= len(media) || media[effect.Media].size() > maxReencodeBytes {
			continue
		}
		if keys == nil {
			keys = make([]string, len(effects))
		}
		keys[index] = imageCompressionKey(userID, media[effect.Media], effect)
		key := keys[index]
		if occurrences[key] == 0 {
			unique = append(unique, key)
		}
		occurrences[key]++
	}
	if keys == nil {
		return nil
	}
	args := make([]any, 1, 1+2*len(unique))
	args[0] = common.NodeName
	for _, key := range unique {
		args = append(args, key, occurrences[key])
	}
	reply, ok := cache.run(ctx, settings, "lookup", args...)
	if imageCacheResponseStatus(reply, ok) != ImageCacheReady || len(reply) != 1+2*len(unique) {
		return nil
	}
	batch := &imageCacheBatch{cache: cache, settings: settings, generation: redisString(reply[0]), keys: keys, results: make(map[string]imageReencodeResult)}
	offset := 1
	for _, key := range unique {
		metadata, payload := redisString(reply[offset]), redisString(reply[offset+1])
		offset += 2
		if payload == "" {
			continue
		}
		var row ImageCompressionCacheEntry
		if err := common.UnmarshalJsonStr(metadata, &row); err != nil || row.UserID != userID || !validCachedImage(row, payload) {
			continue
		}
		batch.results[key] = imageReencodeResult{data: []byte(payload), originalMIME: row.OriginalMIME, compressedMIME: row.CompressedMIME, originalBytes: row.OriginalBytes}
	}
	return batch
}

func validCachedImage(row ImageCompressionCacheEntry, payload string) bool {
	switch row.CompressedMIME {
	case "image/jpeg", "image/png", "image/webp":
		return row.CompressedBytes == len(payload) && row.CompressedBytes < row.OriginalBytes && row.OriginalBytes <= maxReencodeBytes
	default:
		return false
	}
}

func (batch *imageCacheBatch) result(index int) (string, imageReencodeResult) {
	if batch == nil {
		return "", imageReencodeResult{}
	}
	key := batch.keys[index]
	return key, batch.results[key]
}

func (batch *imageCacheBatch) add(key string, result imageReencodeResult) {
	if key == "" {
		return
	}
	batch.results[key] = result
	batch.pending = append(batch.pending, key)
}

func (batch *imageCacheBatch) store(ctx context.Context, userID int) {
	if batch == nil || len(batch.pending) == 0 || ctx.Err() != nil || sharedImageCompressionCache.Load() != batch.cache {
		return
	}
	settings := operation_setting.GetImageCompressionCacheSettings()
	if settings != batch.settings || !settings.Enabled {
		return
	}
	args := []any{batch.generation}
	var bytes int64
	for _, key := range batch.pending {
		result := batch.results[key]
		size := int64(len(result.data))
		if bytes+size > settings.MaxBytes {
			continue
		}
		row := ImageCompressionCacheEntry{ID: common.GetUUID(), UserID: userID, OriginalMIME: result.originalMIME, CompressedMIME: result.compressedMIME, OriginalBytes: result.originalBytes, CompressedBytes: len(result.data)}
		metadata, err := common.Marshal(row)
		if err != nil {
			continue
		}
		bytes += size
		args = append(args, key, metadata, result.data)
	}
	if len(args) > 1 {
		batch.cache.run(ctx, settings, "store", args...)
	}
}
