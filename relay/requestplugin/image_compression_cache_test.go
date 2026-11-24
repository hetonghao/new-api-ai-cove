package requestplugin

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestImageCompressionCacheHitBypassesEncoderSlotsAndIsolatesUsers(t *testing.T) {
	enableImageCompressionCacheForTest(t)
	server := miniredis.RunT(t)
	url := "redis://" + server.Addr()
	require.NoError(t, ConfigureImageCompressionCacheRedis(url))
	t.Cleanup(func() { require.NoError(t, ConfigureImageCompressionCacheRedis("")) })
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
	// A second client has no process-local copy of the encoded result.
	require.NoError(t, ConfigureImageCompressionCacheRedis(url))

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
	// Standard decoding ignores the unused bits in the final character as
	// well as line breaks. They must not create a distinct cache entry.
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	padding := len(data) - len(strings.TrimRight(data, "="))
	require.Positive(t, padding)
	last := len(data) - padding - 1
	variant := data[:last] + string(alphabet[strings.IndexByte(alphabet, data[last])+1]) + data[last+1:]
	wrapped := strings.Join([]string{variant[:64], variant[64:]}, `\r\n`)
	call.Body = []byte(`{"model":"gpt-up","messages":[{"content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + wrapped + `"}}]}]}`)
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
	stats := GetImageCompressionCacheSnapshot(t.Context(), 1, 25)
	require.Len(t, stats.Entries, 1)
	require.EqualValues(t, 1, stats.Entries[0].Hits)
	require.Equal(t, 7401, stats.Entries[0].UserID)

	// The dry run can simulate user 7401, but cannot reuse their real cache entry.
	dryCtx, cancelDry := context.WithTimeout(t.Context(), time.Second)
	defer cancelDry()
	result := DryRun(dryCtx, DryRunInput{Body: call.Body, User: jsplugin.TransformUser{ID: 7401}})
	require.NotNil(t, result.Failed)
	require.Equal(t, errorMedia, result.Failed.Class)
	require.Equal(t, stats, GetImageCompressionCacheSnapshot(t.Context(), 1, 25))
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

func TestSharedImageCompressionCacheCapacityExpiryAndRemoval(t *testing.T) {
	settings := enableImageCompressionCacheForTest(t)
	settings.MaxBytes, settings.TTLSeconds = 1<<20, 60
	require.NoError(t, operation_setting.SetImageCompressionCacheSettings(settings))
	server := miniredis.RunT(t)
	url := "redis://" + server.Addr()
	require.NoError(t, ConfigureImageCompressionCacheRedis(url))
	t.Cleanup(func() { require.NoError(t, ConfigureImageCompressionCacheRedis("")) })
	start := time.Unix(1764000000, 0)
	server.SetTime(start)

	cache := sharedImageCompressionCache.Load()
	insert := func(key string, size int) {
		t.Helper()
		stats := GetImageCompressionCacheSnapshot(t.Context(), 1, 25)
		batch := imageCacheBatch{cache: cache, settings: settings, generation: stats.CacheID, results: make(map[string]imageReencodeResult)}
		batch.add(key, imageReencodeResult{data: bytes.Repeat([]byte{1}, size), originalMIME: "image/png", compressedMIME: "image/webp", originalBytes: size + 50})
		batch.store(t.Context(), 41)
	}
	insert("old", 400<<10)
	insert("second", 400<<10)
	insert("third", 400<<10)
	snapshot := GetImageCompressionCacheSnapshot(t.Context(), 1, 25)
	require.Equal(t, ImageCacheReady, snapshot.Status)
	require.Equal(t, int64(800<<10), snapshot.UsedBytes)
	require.Equal(t, 2, snapshot.EntryCount)
	require.EqualValues(t, 1, snapshot.Evictions)
	ids := []string{snapshot.Entries[0].ID, snapshot.Entries[1].ID}
	removed, err := RemoveImageCompressionCacheEntries(t.Context(), snapshot.CacheID, ids, false)
	require.NoError(t, err)
	require.Equal(t, 2, removed)
	_, err = RemoveImageCompressionCacheEntries(t.Context(), snapshot.CacheID, ids, false)
	require.ErrorIs(t, err, ErrImageCacheChanged)
	insert("expiring", 400<<10)
	server.FastForward(time.Minute)
	server.SetTime(start.Add(time.Minute))
	expired := GetImageCompressionCacheSnapshot(t.Context(), 1, 25)
	require.Equal(t, 0, expired.EntryCount)
	require.EqualValues(t, 0, expired.UsedBytes)
	require.EqualValues(t, 1, expired.Expirations)

	insert("configured", 400<<10)
	oldSettings := settings
	settings.Revision++
	settings.Enabled = false
	require.NoError(t, operation_setting.SetImageCompressionCacheSettings(settings))
	require.Equal(t, ImageCacheReady, ApplyImageCompressionCacheSettings(t.Context()))
	require.Zero(t, GetImageCompressionCacheSnapshot(t.Context(), 1, 25).EntryCount)
	// A node waiting for the database configuration update cannot re-enable the cache.
	reply, ok := cache.run(t.Context(), oldSettings, "lookup", "late-node", "configured", 1)
	require.Equal(t, ImageCacheSynchronizing, imageCacheResponseStatus(reply, ok))
}

func TestSharedImageCompressionCacheDropsEntriesAfterSkippedRevision(t *testing.T) {
	settings := enableImageCompressionCacheForTest(t)
	server := miniredis.RunT(t)
	require.NoError(t, ConfigureImageCompressionCacheRedis("redis://"+server.Addr()))
	t.Cleanup(func() { require.NoError(t, ConfigureImageCompressionCacheRedis("")) })
	snapshot := GetImageCompressionCacheSnapshot(t.Context(), 1, 25)
	batch := imageCacheBatch{cache: sharedImageCompressionCache.Load(), settings: settings, generation: snapshot.CacheID, results: make(map[string]imageReencodeResult)}
	batch.add("before-disable", imageReencodeResult{data: []byte{1}, originalMIME: "image/png", compressedMIME: "image/webp", originalBytes: 16})
	batch.store(t.Context(), 41)
	require.Equal(t, 1, GetImageCompressionCacheSnapshot(t.Context(), 1, 25).EntryCount)
	// Redis was unreachable for the disabled revision and first observes re-enable.
	settings.Revision += 2
	require.NoError(t, operation_setting.SetImageCompressionCacheSettings(settings))
	require.Equal(t, ImageCacheReady, ApplyImageCompressionCacheSettings(t.Context()))
	require.Zero(t, GetImageCompressionCacheSnapshot(t.Context(), 1, 25).EntryCount)
}

func TestUnavailableImageCacheDoesNotKeepLocalResults(t *testing.T) {
	enableImageCompressionCacheForTest(t)
	server := miniredis.RunT(t)
	url := "redis://" + server.Addr()
	require.NoError(t, ConfigureImageCompressionCacheRedis(url))
	t.Cleanup(func() { require.NoError(t, ConfigureImageCompressionCacheRedis("")) })
	setupPlugins(t, `[{"id":"rp","name":"Rp","plugin":"rp-stages","enabled":true,"global":true,"config":{"effects":{"outbound":[{"type":"media.reencode","media":0,"format":"webp"}]}}}]`)
	var source bytes.Buffer
	require.NoError(t, (&png.Encoder{CompressionLevel: png.NoCompression}).Encode(&source, image.NewGray(image.Rect(0, 0, 128, 128))))
	imageURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(source.Bytes())
	body := []byte(`{"model":"gpt-up","input":[{"type":"input_image","image_url":"` + imageURL + `"}]}`)
	call := Call{Stage: StageOutbound, Body: body, User: jsplugin.TransformUser{ID: 7401}}
	warm := NewPlan().Run(t.Context(), call)
	require.Contains(t, gjson.GetBytes(warm.Body, "input.0.image_url").String(), "data:image/webp;base64,")
	server.Close()
	withoutCache := NewPlan().Run(t.Context(), call)
	require.Equal(t, warm.Body, withoutCache.Body)
	require.Equal(t, ImageCacheUnavailable, GetImageCompressionCacheSnapshot(t.Context(), 1, 25).Status)
	for range cap(reencodeSlots) {
		reencodeSlots <- struct{}{}
	}
	defer func() {
		for range cap(reencodeSlots) {
			<-reencodeSlots
		}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	miss := NewPlan().Run(ctx, call)
	require.NotNil(t, miss.Failed)
	require.Equal(t, errorMedia, miss.Failed.Class)
}
