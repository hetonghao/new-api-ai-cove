package router

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

//go:embed web/dist web/dist/index.html
var webRouterTestAssets embed.FS

func newWebRouterTestAssets(t *testing.T) WebAssets {
	t.Helper()

	defaultIndex, err := webRouterTestAssets.ReadFile("web/dist/index.html")
	require.NoError(t, err)

	return WebAssets{
		BuildFS:   webRouterTestAssets,
		IndexPage: defaultIndex,
	}
}

func newWebRouterTestEngine(t *testing.T) *gin.Engine {
	t.Helper()

	gin.SetMode(gin.TestMode)

	engine := gin.New()
	SetWebRouter(engine, newWebRouterTestAssets(t), func(c *gin.Context) { c.Next() })
	return engine
}

func firstDefaultStaticAsset(t *testing.T, dir string, suffix string) string {
	t.Helper()

	entries, err := webRouterTestAssets.ReadDir(path.Join("web/dist", dir))
	require.NoError(t, err)

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			return path.Join("/", dir, entry.Name())
		}
	}

	t.Fatalf("no %s asset found in %s", suffix, dir)
	return ""
}

func TestWebRouterServesDownloadsLatestJSONWithGzipWithoutIndexFallback(t *testing.T) {
	engine := newWebRouterTestEngine(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/downloads/latest.json", nil)
	request.Header.Set("Accept-Encoding", "gzip")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "gzip", recorder.Header().Get("Content-Encoding"))
	require.Equal(t, "Accept-Encoding", recorder.Header().Get("Vary"))
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	require.Equal(t, strconv.Itoa(recorder.Body.Len()), recorder.Header().Get("Content-Length"))
	reader, err := gzip.NewReader(recorder.Body)
	require.NoError(t, err)
	defer reader.Close()
	actualBody, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.JSONEq(t, `{"version":"1.0.3","platforms":["darwin-aarch64","windows-x86_64"]}`, string(actualBody))
}

func TestWebRouterServesDesktopInstallerWithGzipAndWithContentLength(t *testing.T) {
	engine := newWebRouterTestEngine(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/downloads/ai-cove-design-desktop-windows.exe", nil)
	request.Header.Set("Accept-Encoding", "gzip")

	engine.ServeHTTP(recorder, request)

	expectedBody, err := webRouterTestAssets.ReadFile("web/dist/downloads/ai-cove-design-desktop-windows.exe")
	require.NoError(t, err)

	compressedSize := recorder.Body.Len()
	reader, err := gzip.NewReader(recorder.Body)
	require.NoError(t, err)
	defer reader.Close()
	actualBody, err := io.ReadAll(reader)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "gzip", recorder.Header().Get("Content-Encoding"))
	require.Equal(t, "Accept-Encoding", recorder.Header().Get("Vary"))
	require.NotEmpty(t, recorder.Header().Get("Content-Length"))
	require.Equal(t, strconv.Itoa(compressedSize), recorder.Header().Get("Content-Length"))
	require.True(t, bytes.Equal(expectedBody, actualBody))
}

func TestWebRouterResumesGzipDesktopInstaller(t *testing.T) {
	server := httptest.NewServer(newWebRouterTestEngine(t))
	t.Cleanup(server.Close)
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}, Timeout: 5 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	installerURL := server.URL + "/downloads/ai-cove-design-desktop-windows.exe"
	request := func(method, byteRange, ifRange string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, installerURL, nil)
		require.NoError(t, err)
		req.Header.Set("Accept-Encoding", "gzip")
		req.Header.Set("Range", byteRange)
		req.Header.Set("If-Range", ifRange)
		response, err := client.Do(req)
		require.NoError(t, err)
		return response
	}

	head := request(http.MethodHead, "", "")
	require.Equal(t, http.StatusOK, head.StatusCode)
	require.Equal(t, "gzip", head.Header.Get("Content-Encoding"))
	require.Equal(t, "bytes", head.Header.Get("Accept-Ranges"))
	require.Greater(t, head.ContentLength, int64(10))
	etag := head.Header.Get("ETag")
	require.Regexp(t, `^"gzip-[0-9a-f]{64}"$`, etag)
	headBody, err := io.ReadAll(head.Body)
	require.NoError(t, err)
	require.NoError(t, head.Body.Close())
	require.Empty(t, headBody)

	var resumed []byte
	for _, part := range []struct {
		byteRange  string
		start, end int64
	}{
		{byteRange: "bytes=0-9", start: 0, end: 10},
		{byteRange: "bytes=10-", start: 10, end: head.ContentLength},
	} {
		response := request(http.MethodGet, part.byteRange, etag)
		require.Equal(t, http.StatusPartialContent, response.StatusCode)
		require.Equal(t, "gzip", response.Header.Get("Content-Encoding"))
		require.Equal(t, "Accept-Encoding", response.Header.Get("Vary"))
		require.Equal(t, etag, response.Header.Get("ETag"))
		require.Equal(t, "bytes "+strconv.FormatInt(part.start, 10)+"-"+strconv.FormatInt(part.end-1, 10)+"/"+strconv.FormatInt(head.ContentLength, 10), response.Header.Get("Content-Range"))
		require.Equal(t, part.end-part.start, response.ContentLength)
		require.False(t, response.Uncompressed, "the client must retain encoded bytes for Range offsets")
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Len(t, body, int(part.end-part.start))
		resumed = append(resumed, body...)
	}
	require.Len(t, resumed, int(head.ContentLength))
	reader, err := gzip.NewReader(bytes.NewReader(resumed))
	require.NoError(t, err)
	defer reader.Close()
	actualBody, err := io.ReadAll(reader)
	require.NoError(t, err)
	expectedBody, err := webRouterTestAssets.ReadFile("web/dist/downloads/ai-cove-design-desktop-windows.exe")
	require.NoError(t, err)
	require.Equal(t, expectedBody, actualBody)

	outOfBounds := request(http.MethodGet, "bytes="+strconv.FormatInt(head.ContentLength, 10)+"-", etag)
	require.Equal(t, http.StatusRequestedRangeNotSatisfiable, outOfBounds.StatusCode)
	require.Equal(t, "bytes */"+strconv.FormatInt(head.ContentLength, 10), outOfBounds.Header.Get("Content-Range"))
	require.Empty(t, outOfBounds.Header.Get("Content-Encoding"))
	errorBody, err := io.ReadAll(outOfBounds.Body)
	require.NoError(t, err)
	require.NoError(t, outOfBounds.Body.Close())
	require.NotEmpty(t, errorBody)
	require.Equal(t, int64(len(errorBody)), outOfBounds.ContentLength)

	conditionalRequest, err := http.NewRequest(http.MethodGet, installerURL, nil)
	require.NoError(t, err)
	conditionalRequest.Header.Set("Accept-Encoding", "gzip")
	conditionalRequest.Header.Set("If-Match", `"old-gzip"`)
	preconditionFailed, err := client.Do(conditionalRequest)
	require.NoError(t, err)
	require.Equal(t, http.StatusPreconditionFailed, preconditionFailed.StatusCode)
	require.Empty(t, preconditionFailed.Header.Get("Content-Encoding"))
	require.Equal(t, int64(0), preconditionFailed.ContentLength)
	failedBody, err := io.ReadAll(preconditionFailed.Body)
	require.NoError(t, err)
	require.NoError(t, preconditionFailed.Body.Close())
	require.Empty(t, failedBody)
}

func TestWebRouterServesStaticJavaScriptWithGzipAndWithImmutableCache(t *testing.T) {
	engine := newWebRouterTestEngine(t)
	recorder := httptest.NewRecorder()
	assetPath := firstDefaultStaticAsset(t, "static/js", ".js")
	request := httptest.NewRequest(http.MethodGet, assetPath, nil)
	request.Header.Set("Accept-Encoding", "gzip")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "gzip", recorder.Header().Get("Content-Encoding"))
	require.Equal(t, "Accept-Encoding", recorder.Header().Get("Vary"))
	require.Equal(t, "public, max-age=31536000, immutable", recorder.Header().Get("Cache-Control"))
	require.Equal(t, strconv.Itoa(recorder.Body.Len()), recorder.Header().Get("Content-Length"))
	expectedBody, err := webRouterTestAssets.ReadFile(path.Join("web/dist", assetPath))
	require.NoError(t, err)
	reader, err := gzip.NewReader(recorder.Body)
	require.NoError(t, err)
	defer reader.Close()
	actualBody, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, expectedBody, actualBody)
}

func TestWebRouterServesStaticCssWithGzipAndWithImmutableCache(t *testing.T) {
	engine := newWebRouterTestEngine(t)
	recorder := httptest.NewRecorder()
	assetPath := firstDefaultStaticAsset(t, "static/css", ".css")
	request := httptest.NewRequest(http.MethodGet, assetPath, nil)
	request.Header.Set("Accept-Encoding", "gzip")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "gzip", recorder.Header().Get("Content-Encoding"))
	require.Equal(t, "Accept-Encoding", recorder.Header().Get("Vary"))
	require.Equal(t, "public, max-age=31536000, immutable", recorder.Header().Get("Cache-Control"))
	require.Equal(t, strconv.Itoa(recorder.Body.Len()), recorder.Header().Get("Content-Length"))
	expectedBody, err := webRouterTestAssets.ReadFile(path.Join("web/dist", assetPath))
	require.NoError(t, err)
	reader, err := gzip.NewReader(recorder.Body)
	require.NoError(t, err)
	defer reader.Close()
	actualBody, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, expectedBody, actualBody)
}
