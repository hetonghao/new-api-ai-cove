package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingFrontendFS struct {
	static.ServeFileSystem
	opens atomic.Int32
}

func (f *countingFrontendFS) Open(name string) (http.File, error) {
	f.opens.Add(1)
	return f.ServeFileSystem.Open(name)
}

func performFrontendRequest(router http.Handler, method, target, acceptEncoding string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, nil)
	if acceptEncoding != "" {
		request.Header.Set("Accept-Encoding", acceptEncoding)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestServeFrontendFilesCompressesEachFileOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	frontendDir := t.TempDir()
	script := []byte(strings.Repeat("export const answer = 42;\n", 400))
	image := []byte("\x89PNG\r\n\x1a\nnot-really-an-image")
	require.NoError(t, os.MkdirAll(filepath.Join(frontendDir, "static", "js"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(frontendDir, "static", "js", "app.js"), script, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(frontendDir, "logo.png"), image, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(frontendDir, "index.html"), []byte("<html></html>"), 0o644))
	frontendFS := &countingFrontendFS{ServeFileSystem: static.LocalFile(frontendDir, false)}

	router := gin.New()
	router.NoRoute(ServeFrontendFiles(frontendFS), func(c *gin.Context) {
		c.String(http.StatusTeapot, "fallback")
	})

	plain := performFrontendRequest(router, http.MethodGet, "/static/js/app.js", "")
	require.Equal(t, http.StatusOK, plain.Code)
	assert.Empty(t, plain.Header().Get("Content-Encoding"))
	assert.Equal(t, script, plain.Body.Bytes())
	frontendFS.opens.Store(0)

	responses := make([]*httptest.ResponseRecorder, 8)
	var wg sync.WaitGroup
	for i := range responses {
		wg.Go(func() {
			responses[i] = performFrontendRequest(router, http.MethodGet, "/static/js/app.js", "gzip, deflate, br")
		})
	}
	wg.Wait()
	for _, response := range responses {
		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
		assert.Equal(t, "Accept-Encoding", response.Header().Get("Vary"))
		assert.Equal(t, plain.Header().Get("Content-Type"), response.Header().Get("Content-Type"))
		assert.Equal(t, strconv.Itoa(response.Body.Len()), response.Header().Get("Content-Length"))
		assert.Equal(t, responses[0].Body.Bytes(), response.Body.Bytes())
	}
	reader, err := gzip.NewReader(bytes.NewReader(responses[0].Body.Bytes()))
	require.NoError(t, err)
	decoded, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, script, decoded)

	head := performFrontendRequest(router, http.MethodHead, "/static/js/app.js", "gzip")
	assert.Equal(t, http.StatusOK, head.Code)
	assert.Equal(t, "gzip", head.Header().Get("Content-Encoding"))
	assert.Equal(t, strconv.Itoa(responses[0].Body.Len()), head.Header().Get("Content-Length"))
	assert.Empty(t, head.Body.Bytes())
	assert.Equal(t, int32(1), frontendFS.opens.Load(), "the file must be read and compressed once")

	png := performFrontendRequest(router, http.MethodGet, "/logo.png", "gzip")
	assert.Equal(t, http.StatusOK, png.Code)
	assert.Empty(t, png.Header().Get("Content-Encoding"))
	assert.Equal(t, image, png.Body.Bytes())

	index := performFrontendRequest(router, http.MethodGet, "/index.html", "gzip")
	assert.Equal(t, http.StatusMovedPermanently, index.Code)
	assert.Equal(t, "./", index.Header().Get("Location"))

	missing := performFrontendRequest(router, http.MethodGet, "/static/js/missing.js", "gzip")
	assert.Equal(t, http.StatusTeapot, missing.Code)
	assert.Equal(t, "fallback", missing.Body.String())
}

func TestServeFrontendFilesGzipRangeUsesCompressedBytes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	frontendDir := t.TempDir()
	script := []byte(strings.Repeat("export const answer = 42;\n", 400))
	require.NoError(t, os.WriteFile(filepath.Join(frontendDir, "app.js"), script, 0o644))
	frontendFS := &countingFrontendFS{ServeFileSystem: static.LocalFile(frontendDir, false)}
	router := gin.New()
	router.NoRoute(Cache(), ServeFrontendFiles(frontendFS))
	full := performFrontendRequest(router, http.MethodGet, "/app.js", "gzip")
	require.Equal(t, http.StatusOK, full.Code)
	compressed := full.Body.Bytes()
	etag := full.Header().Get("ETag")
	assert.Regexp(t, `^"gzip-[0-9a-f]{64}"$`, etag)
	assert.Equal(t, "bytes", full.Header().Get("Accept-Ranges"))
	require.NotEmpty(t, full.Header().Get("Last-Modified"))

	requestWithHeaders := func(method, encoding string, headers map[string]string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "/app.js", nil)
		request.Header.Set("Accept-Encoding", encoding)
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	var resumed []byte
	for _, tc := range []struct {
		name, byteRange, ifRange string
		start, end               int
		status                   int
	}{
		{name: "prefix", byteRange: "bytes=0-9", start: 0, end: 10, status: http.StatusPartialContent},
		{name: "open ended", byteRange: "bytes=10-", start: 10, end: len(compressed), status: http.StatusPartialContent},
		{name: "suffix", byteRange: "bytes=-8", start: len(compressed) - 8, end: len(compressed), status: http.StatusPartialContent},
		{name: "matching ETag", byteRange: "bytes=0-9", ifRange: etag, end: 10, status: http.StatusPartialContent},
		{name: "matching date", byteRange: "bytes=0-9", ifRange: full.Header().Get("Last-Modified"), end: 10, status: http.StatusPartialContent},
		{name: "old ETag", byteRange: "bytes=0-9", ifRange: `"old-gzip"`, end: len(compressed), status: http.StatusOK},
		{name: "weak ETag", byteRange: "bytes=0-9", ifRange: "W/" + etag, end: len(compressed), status: http.StatusOK},
		{name: "old date", byteRange: "bytes=0-9", ifRange: "Thu, 01 Jan 1970 00:00:00 GMT", end: len(compressed), status: http.StatusOK},
		{name: "multiple ranges", byteRange: "bytes=0-9,-8", end: len(compressed), status: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := requestWithHeaders(http.MethodGet, "gzip", map[string]string{"Range": tc.byteRange, "If-Range": tc.ifRange})
			require.Equal(t, tc.status, response.Code)
			assert.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
			assert.Equal(t, "Accept-Encoding", response.Header().Get("Vary"))
			assert.Equal(t, "bytes", response.Header().Get("Accept-Ranges"))
			assert.Equal(t, etag, response.Header().Get("ETag"))
			assert.Equal(t, full.Header().Get("Content-Type"), response.Header().Get("Content-Type"))
			assert.Equal(t, full.Header().Get("Cache-Control"), response.Header().Get("Cache-Control"))
			assert.Equal(t, strconv.Itoa(tc.end-tc.start), response.Header().Get("Content-Length"))
			assert.Equal(t, compressed[tc.start:tc.end], response.Body.Bytes())
			if tc.status == http.StatusPartialContent {
				assert.Equal(t, "bytes "+strconv.Itoa(tc.start)+"-"+strconv.Itoa(tc.end-1)+"/"+strconv.Itoa(len(compressed)), response.Header().Get("Content-Range"))
			} else {
				assert.Empty(t, response.Header().Get("Content-Range"))
			}
			if tc.name == "prefix" || tc.name == "open ended" {
				resumed = append(resumed, response.Body.Bytes()...)
			}
		})
	}
	require.Equal(t, compressed, resumed)
	reader, err := gzip.NewReader(bytes.NewReader(resumed))
	require.NoError(t, err)
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, script, decoded)
	headWithRange := requestWithHeaders(http.MethodHead, "gzip", map[string]string{"Range": "bytes=0-9"})
	assert.Equal(t, http.StatusOK, headWithRange.Code, "Range only applies to GET")
	assert.Equal(t, strconv.Itoa(len(compressed)), headWithRange.Header().Get("Content-Length"))
	assert.Equal(t, "gzip", headWithRange.Header().Get("Content-Encoding"))
	assert.Empty(t, headWithRange.Body.Bytes())

	for _, headers := range []map[string]string{
		{"If-None-Match": etag},
		{"If-Modified-Since": full.Header().Get("Last-Modified")},
	} {
		response := requestWithHeaders(http.MethodGet, "gzip", headers)
		assert.Equal(t, http.StatusNotModified, response.Code)
		assert.Empty(t, response.Body.Bytes())
		assert.Empty(t, response.Header().Get("Content-Length"))
		assert.Empty(t, response.Header().Get("Content-Encoding"))
		assert.Empty(t, response.Header().Get("Content-Type"))
		assert.Equal(t, etag, response.Header().Get("ETag"))
		assert.Equal(t, "Accept-Encoding", response.Header().Get("Vary"))
		assert.Equal(t, full.Header().Get("Cache-Control"), response.Header().Get("Cache-Control"))
	}
	outOfBounds := requestWithHeaders(http.MethodGet, "gzip", map[string]string{"Range": "bytes=" + strconv.Itoa(len(compressed)) + "-"})
	assert.Equal(t, http.StatusRequestedRangeNotSatisfiable, outOfBounds.Code)
	assert.Equal(t, "bytes */"+strconv.Itoa(len(compressed)), outOfBounds.Header().Get("Content-Range"))
	assert.Empty(t, outOfBounds.Header().Get("Content-Encoding"))
	assert.Empty(t, outOfBounds.Header().Get("Content-Length"), "the compressed file length must not describe the error body")
	assert.Equal(t, "text/plain; charset=utf-8", outOfBounds.Header().Get("Content-Type"))
	assert.Empty(t, outOfBounds.Header().Get("Cache-Control"))
	assert.NotEmpty(t, outOfBounds.Body.String())
	for _, headers := range []map[string]string{
		{"If-Match": `"old-gzip"`},
		{"If-Unmodified-Since": "Thu, 01 Jan 1970 00:00:00 GMT"},
	} {
		preconditionFailed := requestWithHeaders(http.MethodGet, "gzip", headers)
		assert.Equal(t, http.StatusPreconditionFailed, preconditionFailed.Code)
		assert.Empty(t, preconditionFailed.Body.Bytes())
		assert.Empty(t, preconditionFailed.Header().Get("Content-Length"))
		assert.Empty(t, preconditionFailed.Header().Get("Content-Encoding"))
		assert.Equal(t, "no-store", preconditionFailed.Header().Get("Cache-Control"))
	}
	assert.Equal(t, int32(1), frontendFS.opens.Load(), "ranges and conditional requests must reuse the compressed file")

	identity := requestWithHeaders(http.MethodGet, "identity", map[string]string{"Range": "bytes=0-9", "If-Range": etag})
	assert.Equal(t, http.StatusOK, identity.Code, "a gzip validator must not match the identity representation")
	assert.Empty(t, identity.Header().Get("Content-Encoding"))
	assert.NotEqual(t, etag, identity.Header().Get("ETag"))
	assert.Equal(t, script, identity.Body.Bytes())
	assert.Equal(t, "Accept-Encoding", identity.Header().Get("Vary"), "identity caches must also distinguish the gzip representation")

	for _, encoding := range []string{"br", "gzip;q=0", "gzip; q=0.000, br", "gzip;q=0, *;q=1", "xgzip"} {
		t.Run(encoding, func(t *testing.T) {
			response := requestWithHeaders(http.MethodGet, encoding, nil)
			assert.Equal(t, http.StatusOK, response.Code)
			assert.Empty(t, response.Header().Get("Content-Encoding"))
			assert.Equal(t, script, response.Body.Bytes())
			assert.Equal(t, strconv.Itoa(len(script)), response.Header().Get("Content-Length"))
		})
	}
	for _, encoding := range []string{"gzip;q=0.5", "GZIP;Q=1.0"} {
		response := requestWithHeaders(http.MethodGet, encoding, nil)
		assert.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
		assert.Equal(t, compressed, response.Body.Bytes())
	}

	updatedScript := []byte("export const answer = 43;\n")
	require.NoError(t, os.WriteFile(filepath.Join(frontendDir, "app.js"), updatedScript, 0o644))
	updatedRouter := gin.New()
	updatedRouter.NoRoute(ServeFrontendFiles(static.LocalFile(frontendDir, false)))
	updatedRequest := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	updatedRequest.Header.Set("Accept-Encoding", "gzip")
	updatedRequest.Header.Set("Range", "bytes=10-")
	updatedRequest.Header.Set("If-Range", etag)
	updated := httptest.NewRecorder()
	updatedRouter.ServeHTTP(updated, updatedRequest)
	assert.Equal(t, http.StatusOK, updated.Code, "a new build must not resume bytes from the old build")
	assert.NotEqual(t, etag, updated.Header().Get("ETag"))
	updatedReader, err := gzip.NewReader(updated.Body)
	require.NoError(t, err)
	defer updatedReader.Close()
	updatedBody, err := io.ReadAll(updatedReader)
	require.NoError(t, err)
	assert.Equal(t, updatedScript, updatedBody)
}
