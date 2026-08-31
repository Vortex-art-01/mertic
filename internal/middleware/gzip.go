package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

var compressibleTypes = []string{"application/json", "text/html"}

func compressible(contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))

	for _, t := range compressibleTypes {
		if mediaType == t {
			return true
		}
	}
	return false
}

func acceptsGzip(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Accept-Encoding")), "gzip")
}

func sentGzip(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Content-Encoding")), "gzip")
}

type compressWriter struct {
	http.ResponseWriter
	zw      *gzip.Writer
	decided bool
}

func (w *compressWriter) decide() {
	if w.decided {
		return
	}
	w.decided = true

	if !compressible(w.Header().Get("Content-Type")) {
		return
	}

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Del("Content-Length")

	w.zw = gzip.NewWriter(w.ResponseWriter)
}

func (w *compressWriter) WriteHeader(status int) {
	w.decide()
	w.ResponseWriter.WriteHeader(status)
}

func (w *compressWriter) Write(b []byte) (int, error) {
	w.decide()

	if w.zw == nil {
		return w.ResponseWriter.Write(b)
	}
	return w.zw.Write(b)
}

func (w *compressWriter) Close() error {
	if w.zw == nil {
		return nil
	}
	return w.zw.Close()
}

type compressReader struct {
	body io.ReadCloser
	zr   *gzip.Reader
}

func (r *compressReader) Read(p []byte) (int, error) {
	return r.zr.Read(p)
}

func (r *compressReader) Close() error {
	if err := r.zr.Close(); err != nil {
		_ = r.body.Close()
		return err
	}
	return r.body.Close()
}

func WithGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sentGzip(r) {
			zr, err := gzip.NewReader(r.Body)
			if err != nil {
				http.Error(w, "invalid gzip body", http.StatusBadRequest)
				return
			}

			cr := &compressReader{body: r.Body, zr: zr}
			defer cr.Close()

			r.Body = cr
		}

		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Add("Vary", "Accept-Encoding")

		cw := &compressWriter{ResponseWriter: w}
		defer cw.Close()

		next.ServeHTTP(cw, r)
	})
}
