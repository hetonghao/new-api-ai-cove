package middleware

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	ginGzip "github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// precompressedFile is the gzip encoding of one frontend build file.
type precompressedFile struct {
	once        sync.Once
	body        []byte
	contentType string
	etag        string
	modtime     time.Time
	err         error
}

// ServeFrontendFiles serves files from the embedded frontend build. The build
// never changes while the process runs, so each file is gzip-compressed once
// and the bytes are reused, instead of recompressing multi-megabyte chunks on
// every request. The cache holds at most one entry per build file. Clients
// without gzip, the image types the gzip middleware skips, directories, and
// the index.html redirect keep using http.FileServer.
func ServeFrontendFiles(frontendFS static.ServeFileSystem) gin.HandlerFunc {
	fileServer := http.FileServer(frontendFS)
	var cache sync.Map // URL path -> *precompressedFile
	return func(c *gin.Context) {
		urlPath := c.Request.URL.Path
		if !frontendFS.Exists("/", urlPath) {
			return
		}
		c.Abort()
		c.Header("Vary", "Accept-Encoding")
		acceptsGzip := false
		for coding := range strings.SplitSeq(c.GetHeader("Accept-Encoding"), ",") {
			encoding, params, err := mime.ParseMediaType(coding)
			if err != nil || encoding != "gzip" {
				continue
			}
			quality := 1.0
			if value, ok := params["q"]; ok {
				quality, err = strconv.ParseFloat(value, 64)
			}
			acceptsGzip = err == nil && quality > 0 && quality <= 1
			break
		}
		if !acceptsGzip ||
			ginGzip.DefaultExcludedExtentions.Contains(path.Ext(urlPath)) ||
			strings.HasSuffix(urlPath, "/index.html") {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		value, ok := cache.Load(urlPath)
		if !ok {
			value, _ = cache.LoadOrStore(urlPath, &precompressedFile{})
		}
		file := value.(*precompressedFile)
		file.once.Do(func() {
			f, err := frontendFS.Open(urlPath)
			if err != nil {
				file.err = err
				return
			}
			defer f.Close()
			info, err := f.Stat()
			if err != nil {
				file.err = err
				return
			}
			if info.IsDir() {
				file.err = errors.New("directory")
				return
			}
			raw, err := io.ReadAll(f)
			if err != nil {
				file.err = err
				return
			}
			var compressed bytes.Buffer
			writer := gzip.NewWriter(&compressed)
			if _, err := writer.Write(raw); err != nil {
				file.err = err
				return
			}
			if err := writer.Close(); err != nil {
				file.err = err
				return
			}
			// Match http.FileServer: extension first, then content sniffing.
			file.contentType = mime.TypeByExtension(path.Ext(urlPath))
			if file.contentType == "" {
				file.contentType = http.DetectContentType(raw)
			}
			file.body = bytes.Clone(compressed.Bytes())
			file.etag = fmt.Sprintf(`"gzip-%x"`, sha256.Sum256(file.body))
			file.modtime = info.ModTime()
		})
		if file.err != nil {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		header := c.Writer.Header()
		header.Set("Content-Encoding", "gzip")
		header.Set("Content-Type", file.contentType)
		header.Set("Content-Length", strconv.Itoa(len(file.body)))
		header.Set("ETag", file.etag)
		request := c.Request
		if request.Method != http.MethodGet || strings.Contains(request.Header.Get("Range"), ",") {
			// shortcut: serve multiple gzip ranges as a full response until multipart encoding is needed.
			request = request.Clone(request.Context())
			request.Header.Del("Range")
		}
		http.ServeContent(c.Writer, request, urlPath, file.modtime, bytes.NewReader(file.body))
		if c.Writer.Status() == http.StatusPreconditionFailed {
			// Gin has not committed the status-only 412; it has no gzip body or file length.
			header.Del("Content-Length")
			header.Del("Content-Encoding")
			header.Set("Cache-Control", "no-store")
		}
	}
}
