package transport

import (
	"bytes"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tdewolff/minify/v2"
	"github.com/tdewolff/minify/v2/html"
)

type minifyWriter struct {
	gin.ResponseWriter
	buf         *bytes.Buffer
	statusCode  int
	wroteHeader bool
}

func (w *minifyWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.statusCode = code
	w.wroteHeader = true
}

func (w *minifyWriter) Write(data []byte) (int, error) {
	return w.buf.Write(data)
}

func minifyMiddleware() gin.HandlerFunc {
	m := minify.New()
	m.Add("text/html", &html.Minifier{
		KeepDocumentTags: true,
		KeepEndTags:      true,
	})

	return func(c *gin.Context) {
		buf := &bytes.Buffer{}
		mw := &minifyWriter{ResponseWriter: c.Writer, buf: buf}
		c.Writer = mw
		c.Next()

		ct := mw.Header().Get("Content-Type")
		body := buf.Bytes()
		if strings.Contains(ct, "text/html") && len(body) > 0 {
			minified, err := m.Bytes("text/html", body)
			if err == nil {
				body = minified
				mw.ResponseWriter.Header().Del("Content-Length")
			}
		}
		if mw.wroteHeader {
			mw.ResponseWriter.WriteHeader(mw.statusCode)
		}
		if len(body) > 0 {
			mw.ResponseWriter.Write(body)
		}
	}
}
