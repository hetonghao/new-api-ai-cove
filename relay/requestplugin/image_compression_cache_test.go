package requestplugin

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"testing"
	"testing/synctest"
	"time"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestImageCompressionCacheHitBypassesEncoderSlotsAndIsolatesUsers(t *testing.T) {
	enableImageCompressionCacheForTest(t)
	setupPlugins(t, `[{"id":"rp","name":"Rp","plugin":"rp-stages","enabled":true,"global":true,"config":{"effects":{"outbound":[{"type":"media.reencode","media":0,"format":"webp"}]}}}]`)
	var source bytes.Buffer
	require.NoError(t, (&png.Encoder{CompressionLevel: png.NoCompression}).Encode(&source, image.NewGray(image.Rect(0, 0, 128, 128))))
	data := base64.StdEncoding.EncodeToString(source.Bytes())
	body := []byte(`{"model":"gpt-up","messages":[{"content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,` + data + `"}}]}]}`)
	call := Call{Stage: StageOutbound, Body: body, User: jsplugin.TransformUser{ID: 7401}}
	first := NewPlan().Run(t.Context(), call)
	require.Nil(t, first.Failed)
	expected := gjson.GetBytes(first.Body, "messages.0.content.0.image_url.url").String()
	require.Contains(t, expected, "data:image/webp;base64,")

	for range cap(reencodeSlots) {
		reencodeSlots <- struct{}{}
	}
	t.Cleanup(func() {
		for range cap(reencodeSlots) {
			<-reencodeSlots
		}
	})
	// A real hook can run, but any re-encoding has to wait for a slot.
	hitCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	call.Body = []byte(`{"model":"gpt-up","messages":[{"content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + data + `"}}]}]}`)
	hit := NewPlan().Run(hitCtx, call)
	require.Nil(t, hit.Failed)
	out := gjson.GetBytes(hit.Body, "messages.0.content.0.source")
	require.Equal(t, "image/webp", out.Get("media_type").String())
	require.Equal(t, expected, "data:image/webp;base64,"+out.Get("data").String())

	call.User.ID++
	missCtx, cancelMiss := context.WithTimeout(t.Context(), time.Second)
	defer cancelMiss()
	miss := NewPlan().Run(missCtx, call)
	require.NotNil(t, miss.Failed)
	require.Equal(t, errorMedia, miss.Failed.Class)
	require.Equal(t, call.Body, miss.Body)
	stats := GetImageCompressionCacheSnapshot(1, 25)
	require.Len(t, stats.Entries, 1)
	require.EqualValues(t, 1, stats.Entries[0].Hits)
	require.Equal(t, 7401, stats.Entries[0].UserID)

	// The dry run can simulate user 7401, but cannot reuse their real cache entry.
	dryCtx, cancelDry := context.WithTimeout(t.Context(), time.Second)
	defer cancelDry()
	result := DryRun(dryCtx, DryRunInput{Body: call.Body, User: jsplugin.TransformUser{ID: 7401}})
	require.NotNil(t, result.Failed)
	require.Equal(t, errorMedia, result.Failed.Class)
	require.Equal(t, stats, GetImageCompressionCacheSnapshot(1, 25))
}

func TestImageCompressionCacheEvictsLeastRecentlyUsedAndBoundsEntries(t *testing.T) {
	settings := enableImageCompressionCacheForTest(t)
	settings.MaxBytes = 1 << 20
	require.NoError(t, operation_setting.SetImageCompressionCacheSettings(settings))
	cache := imageCompressionCache{now: time.Now}
	t.Cleanup(func() { cache.removeEntries(nil, true) })

	first, second, third := imageCompressionCacheKey{userID: 1}, imageCompressionCacheKey{userID: 2}, imageCompressionCacheKey{userID: 3}
	data := imageReencodeResult{data: make([]byte, 400<<10)}
	for _, key := range []imageCompressionCacheKey{first, second} {
		_, _, generation := cache.lookup(key)
		cache.store(key, data, generation)
	}
	require.True(t, cacheHasImage(&cache, first))
	_, _, generation := cache.lookup(third)
	cache.store(third, data, generation)
	require.False(t, cacheHasImage(&cache, second))
	require.True(t, cacheHasImage(&cache, first))
	require.True(t, cacheHasImage(&cache, third))
	stats := cache.snapshot(1, 25)
	require.Equal(t, int64(800<<10), stats.UsedBytes)
	require.EqualValues(t, 1, stats.Evictions)
	require.Equal(t, 2, stats.EntryCount)

	cache.removeEntries(nil, true)
	tiny := imageReencodeResult{data: []byte{1}}
	for userID := 1; userID <= imageCompressionCacheEntryLimit+1; userID++ {
		key := imageCompressionCacheKey{userID: userID}
		_, _, generation := cache.lookup(key)
		cache.store(key, tiny, generation)
	}
	require.False(t, cacheHasImage(&cache, first))
	require.True(t, cacheHasImage(&cache, imageCompressionCacheKey{userID: imageCompressionCacheEntryLimit + 1}))
	stats = cache.snapshot(1, 1)
	require.Equal(t, imageCompressionCacheEntryLimit, stats.EntryCount)
	require.EqualValues(t, imageCompressionCacheEntryLimit, stats.UsedBytes)
	require.Len(t, stats.Entries, 1)
}

func TestImageCompressionCacheExpiresWithoutRequests(t *testing.T) {
	settings := enableImageCompressionCacheForTest(t)
	settings.TTLSeconds = 60
	require.NoError(t, operation_setting.SetImageCompressionCacheSettings(settings))
	synctest.Test(t, func(t *testing.T) {
		cache := imageCompressionCache{now: time.Now}
		key := imageCompressionCacheKey{userID: 1}
		_, _, generation := cache.lookup(key)
		cache.store(key, imageReencodeResult{data: []byte{1}}, generation)
		time.Sleep(59 * time.Second)
		require.True(t, cacheHasImage(&cache, key))
		time.Sleep(time.Second)
		synctest.Wait()
		// Inspect the retained bytes before calling a path that prunes expired entries.
		require.Zero(t, cache.stats.UsedBytes)
		require.Nil(t, cache.entries)
		require.EqualValues(t, 1, cache.stats.Expirations)
		require.False(t, cacheHasImage(&cache, key))
	})
}

func TestImageCompressionCacheRemovalAndDisableDropEntriesAndStaleEncodes(t *testing.T) {
	settings := enableImageCompressionCacheForTest(t)
	key := imageCompressionCacheKey{userID: 11}
	data := imageReencodeResult{data: []byte{1, 2, 3}, originalMIME: "image/png", compressedMIME: "image/webp", originalBytes: 100}
	add := func(userID int) string {
		t.Helper()
		key := imageCompressionCacheKey{userID: userID}
		_, hit, generation := localImageCompressionCache.lookup(key)
		require.False(t, hit)
		localImageCompressionCache.store(key, data, generation)
		stats := GetImageCompressionCacheSnapshot(1, 1)
		require.Len(t, stats.Entries, 1)
		row := stats.Entries[0]
		require.Equal(t, userID, row.UserID)
		require.Equal(t, "image/png", row.OriginalMIME)
		require.Equal(t, "image/webp", row.CompressedMIME)
		require.Equal(t, 100, row.OriginalBytes)
		require.Equal(t, 3, row.CompressedBytes)
		require.NotEmpty(t, row.ID)
		return row.ID
	}
	first, second, third := add(11), add(12), add(13)
	stats := GetImageCompressionCacheSnapshot(1, 25)
	require.EqualValues(t, 9, stats.UsedBytes)
	require.Equal(t, 3, stats.EntryCount)
	require.Equal(t, 1, RemoveImageCompressionCacheEntries([]string{first}, false))
	replacement := add(11)
	require.NotEqual(t, first, replacement)
	require.Equal(t, 0, RemoveImageCompressionCacheEntries([]string{first}, false))
	require.Equal(t, 2, RemoveImageCompressionCacheEntries([]string{second, third}, false))
	_, _, generation := localImageCompressionCache.lookup(imageCompressionCacheKey{userID: 99})
	require.Equal(t, 1, RemoveImageCompressionCacheEntries(nil, true))
	localImageCompressionCache.store(key, data, generation)
	require.Zero(t, GetImageCompressionCacheSnapshot(1, 25).EntryCount)

	add(14)
	settings.Enabled = false
	require.NoError(t, operation_setting.SetImageCompressionCacheSettings(settings))
	stats = GetImageCompressionCacheSnapshot(1, 25)
	require.False(t, stats.Enabled)
	require.Zero(t, stats.EntryCount)
	require.Zero(t, stats.UsedBytes)
	require.Empty(t, stats.Entries)
	require.False(t, cacheHasImage(&localImageCompressionCache, imageCompressionCacheKey{userID: 14}))
}

func enableImageCompressionCacheForTest(t *testing.T) operation_setting.ImageCompressionCacheSettings {
	t.Helper()
	old := operation_setting.GetImageCompressionCacheSettings()
	settings := old
	settings.Enabled = true
	require.NoError(t, operation_setting.SetImageCompressionCacheSettings(settings))
	t.Cleanup(func() { require.NoError(t, operation_setting.SetImageCompressionCacheSettings(old)) })
	return settings
}

func cacheHasImage(cache *imageCompressionCache, key imageCompressionCacheKey) bool {
	_, hit, _ := cache.lookup(key)
	return hit
}
