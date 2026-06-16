package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

type gzipWriter struct {
	gin.ResponseWriter
	writer *gzip.Writer
}

func (g *gzipWriter) WriteString(s string) (int, error) {
	g.Header().Del("Content-Length")
	return g.writer.Write([]byte(s))
}

func (g *gzipWriter) Write(data []byte) (int, error) {
	g.Header().Del("Content-Length")
	return g.writer.Write(data)
}

func (g *gzipWriter) WriteHeader(code int) {
	g.Header().Del("Content-Length")
	g.ResponseWriter.WriteHeader(code)
}

var gzipPool = sync.Pool{
	New: func() interface{} {
		return gzip.NewWriter(io.Discard)
	},
}

func Gzip(level int) gin.HandlerFunc {
	return func(c *gin.Context) {
		if level < gzip.DefaultCompression || level > gzip.BestCompression {
			level = gzip.DefaultCompression
		}

		accept := c.GetHeader("Accept-Encoding")
		if !strings.Contains(accept, "gzip") {
			c.Next()
			return
		}

		w := gzipPool.Get().(*gzip.Writer)
		defer gzipPool.Put(w)
		defer w.Close()

		w.Reset(c.Writer)
		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")

		gw := &gzipWriter{
			ResponseWriter: c.Writer,
			writer:         w,
		}
		c.Writer = gw

		c.Next()

		contentType := c.Writer.Header().Get("Content-Type")
		if !shouldCompress(contentType) {
			c.Writer = gw.ResponseWriter
			return
		}
	}
}

func shouldCompress(contentType string) bool {
	compressibleTypes := []string{
		"text/plain",
		"text/html",
		"text/css",
		"text/javascript",
		"application/javascript",
		"application/json",
		"application/xml",
		"image/svg+xml",
		"application/vnd.api+json",
	}

	for _, t := range compressibleTypes {
		if strings.HasPrefix(contentType, t) {
			return true
		}
	}
	return false
}

type minSizeResponseWriter struct {
	gin.ResponseWriter
	body      []byte
	minSize   int
	threshold int
}

func GzipMinSize(minSize int, level int) gin.HandlerFunc {
	return func(c *gin.Context) {
		accept := c.GetHeader("Accept-Encoding")
		if !strings.Contains(accept, "gzip") {
			c.Next()
			return
		}

		w := &minSizeResponseWriter{
			ResponseWriter: c.Writer,
			minSize:        minSize,
			body:           make([]byte, 0),
		}
		c.Writer = w

		c.Next()

		if len(w.body) < minSize {
			w.ResponseWriter.WriteHeaderNow()
			w.ResponseWriter.Write(w.body)
			return
		}

		gz := gzipPool.Get().(*gzip.Writer)
		defer gzipPool.Put(gz)
		defer gz.Close()

		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")
		c.Writer.Header().Del("Content-Length")

		gz.Reset(w.ResponseWriter)
		gz.Write(w.body)
	}
}

func (m *minSizeResponseWriter) Write(b []byte) (int, error) {
	if m.threshold == 0 {
		m.body = append(m.body, b...)
		return len(b), nil
	}
	return m.ResponseWriter.Write(b)
}

func (m *minSizeResponseWriter) WriteHeader(code int) {
	if code >= http.StatusOK && code < http.StatusMultipleChoices {
		m.ResponseWriter.WriteHeader(code)
		m.threshold = 1
	} else {
		m.ResponseWriter.WriteHeader(code)
		m.threshold = -1
	}
}
