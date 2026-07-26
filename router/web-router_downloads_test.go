package router

import (
	"bytes"
	"embed"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"strconv"
	"strings"
	"testing"

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
	SetWebRouter(engine, newWebRouterTestAssets(t))
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

func TestWebRouterServesDownloadsLatestJSONWithoutGzipOrIndexFallback(t *testing.T) {
	engine := newWebRouterTestEngine(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/downloads/latest.json", nil)
	request.Header.Set("Accept-Encoding", "gzip")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "", recorder.Header().Get("Content-Encoding"))
	require.Equal(t, "", recorder.Header().Get("Vary"))
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	require.JSONEq(t, `{"version":"1.0.3","platforms":["darwin-aarch64","windows-x86_64"]}`, recorder.Body.String())
	require.Equal(t, strconv.Itoa(recorder.Body.Len()), recorder.Header().Get("Content-Length"))
}

func TestWebRouterServesDesktopInstallerWithoutGzipAndWithContentLength(t *testing.T) {
	engine := newWebRouterTestEngine(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/downloads/ai-cove-design-desktop-windows.exe", nil)
	request.Header.Set("Accept-Encoding", "gzip")

	engine.ServeHTTP(recorder, request)

	expectedBody, err := webRouterTestAssets.ReadFile("web/dist/downloads/ai-cove-design-desktop-windows.exe")
	require.NoError(t, err)

	actualBody, err := io.ReadAll(recorder.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "", recorder.Header().Get("Content-Encoding"))
	require.Equal(t, "", recorder.Header().Get("Vary"))
	require.NotEmpty(t, recorder.Header().Get("Content-Length"))
	require.Equal(t, strconv.Itoa(len(expectedBody)), recorder.Header().Get("Content-Length"))
	require.True(t, bytes.Equal(expectedBody, actualBody))
}

func TestWebRouterServesStaticJavaScriptWithoutOriginGzipAndWithImmutableCache(t *testing.T) {
	engine := newWebRouterTestEngine(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, firstDefaultStaticAsset(t, "static/js", ".js"), nil)
	request.Header.Set("Accept-Encoding", "gzip")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "", recorder.Header().Get("Content-Encoding"))
	require.Equal(t, "", recorder.Header().Get("Vary"))
	require.Equal(t, "public, max-age=31536000, immutable", recorder.Header().Get("Cache-Control"))
	require.NotEmpty(t, recorder.Body.String())
}

func TestWebRouterServesStaticCssWithoutOriginGzipAndWithImmutableCache(t *testing.T) {
	engine := newWebRouterTestEngine(t)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, firstDefaultStaticAsset(t, "static/css", ".css"), nil)
	request.Header.Set("Accept-Encoding", "gzip")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "", recorder.Header().Get("Content-Encoding"))
	require.Equal(t, "", recorder.Header().Get("Vary"))
	require.Equal(t, "public, max-age=31536000, immutable", recorder.Header().Get("Cache-Control"))
	require.NotEmpty(t, recorder.Body.String())
}
