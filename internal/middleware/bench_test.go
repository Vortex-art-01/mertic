package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/hash"
	"github.com/Vortex-art-01/mertic/internal/logger"
)

// benchBody по размеру как пакет метрик от агента: около 2,5 КБ.
var benchBody = "[" + strings.Repeat(`{"id":"HeapAlloc","type":"gauge","value":123456.5},`, 49) + "{}]"

// discardWriter ничего не хранит: httptest.NewRecorder добавил бы к замеру
// собственные аллокации.
type discardWriter struct {
	header http.Header
}

func (w *discardWriter) Header() http.Header         { return w.header }
func (w *discardWriter) WriteHeader(int)             {}
func (w *discardWriter) Write(b []byte) (int, error) { return len(b), nil }

// benchServe гоняет один и тот же запрос через handler. Запрос и его тело
// переиспользуются, чтобы в замер попала только работа мидлвари.
func benchServe(b *testing.B, handler http.Handler, req *http.Request, body []byte) {
	b.Helper()

	w := &discardWriter{header: make(http.Header)}
	reader := bytes.NewReader(nil)
	readCloser := io.NopCloser(reader)

	b.ReportAllocs()
	for b.Loop() {
		clear(w.header)
		reader.Reset(body)
		req.Body = readCloser

		handler.ServeHTTP(w, req)
	}
}

// drainAndReply читает тело запроса и отвечает маленьким JSON — как /updates/.
var drainAndReply = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	_, _ = io.Copy(io.Discard, r.Body)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
})

func BenchmarkWithGzip(b *testing.B) {
	compressed := gzipBytes(b, benchBody)

	b.Run("request", func(b *testing.B) {
		req := httptest.NewRequest(http.MethodPost, "/updates/", nil)
		req.Header.Set("Content-Encoding", "gzip")

		benchServe(b, WithGzip(drainAndReply), req, compressed)
	})

	b.Run("response", func(b *testing.B) {
		req := httptest.NewRequest(http.MethodPost, "/updates/", nil)
		req.Header.Set("Accept-Encoding", "gzip")

		benchServe(b, WithGzip(drainAndReply), req, []byte(benchBody))
	})

	// Запрос агента: сжато и тело запроса, и ответ.
	b.Run("both", func(b *testing.B) {
		req := httptest.NewRequest(http.MethodPost, "/updates/", nil)
		req.Header.Set("Content-Encoding", "gzip")
		req.Header.Set("Accept-Encoding", "gzip")

		benchServe(b, WithGzip(drainAndReply), req, compressed)
	})

	b.Run("passthrough", func(b *testing.B) {
		req := httptest.NewRequest(http.MethodPost, "/updates/", nil)

		benchServe(b, WithGzip(drainAndReply), req, []byte(benchBody))
	})
}

func BenchmarkWithHash(b *testing.B) {
	body := []byte(benchBody)
	handler := WithHash(testKey)(drainAndReply)

	b.Run("signed", func(b *testing.B) {
		req := httptest.NewRequest(http.MethodPost, "/updates/", nil)
		req.Header.Set(hash.Header, hash.Sign(body, testKey))

		benchServe(b, handler, req, body)
	})

	b.Run("unsigned", func(b *testing.B) {
		req := httptest.NewRequest(http.MethodPost, "/updates/", nil)

		benchServe(b, handler, req, body)
	})
}

func BenchmarkWithLogging(b *testing.B) {
	req := httptest.NewRequest(http.MethodPost, "/updates/", nil)

	benchServe(b, WithLogging(logger.New(io.Discard))(drainAndReply), req, []byte(benchBody))
}

func BenchmarkWithBodyLimit(b *testing.B) {
	req := httptest.NewRequest(http.MethodPost, "/updates/", nil)

	benchServe(b, WithBodyLimit(1<<20)(drainAndReply), req, []byte(benchBody))
}

// Вся цепочка в том порядке, как на сервере, на запросе агента.
func BenchmarkMiddlewareChain(b *testing.B) {
	handler := WithLogging(logger.New(io.Discard))(WithGzip(WithBodyLimit(1 << 20)(WithHash(testKey)(drainAndReply))))

	req := httptest.NewRequest(http.MethodPost, "/updates/", nil)
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set(hash.Header, hash.Sign([]byte(benchBody), testKey))

	benchServe(b, handler, req, gzipBytes(b, benchBody))
}
