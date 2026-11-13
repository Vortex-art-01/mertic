package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/audit"
	"github.com/Vortex-art-01/mertic/internal/hash"
	"github.com/Vortex-art-01/mertic/internal/logger"
	"github.com/Vortex-art-01/mertic/internal/model"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

const benchKey = "secret"

// discardWriter ничего не хранит: httptest.NewRecorder добавил бы к замеру
// собственные аллокации.
type discardWriter struct {
	header http.Header
	status int
}

func (w *discardWriter) Header() http.Header         { return w.header }
func (w *discardWriter) WriteHeader(status int)      { w.status = status }
func (w *discardWriter) Write(b []byte) (int, error) { return len(b), nil }

// benchBatchJSON — тело запроса агента: 43 gauge и PollCount.
func benchBatchJSON(b *testing.B) []byte {
	b.Helper()

	metrics := make([]model.Metrics, 44)

	for i := range metrics {
		value := float64(i) * 123.45
		metrics[i] = model.Metrics{ID: fmt.Sprintf("Metric%02d", i), MType: model.Gauge, Value: &value}
	}

	delta := int64(5)
	metrics[len(metrics)-1] = model.Metrics{ID: "PollCount", MType: model.Counter, Delta: &delta}

	data, err := json.Marshal(metrics)
	if err != nil {
		b.Fatal(err)
	}

	return data
}

func gzipped(tb testing.TB, data []byte) []byte {
	tb.Helper()

	var buf bytes.Buffer

	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		tb.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		tb.Fatal(err)
	}

	return buf.Bytes()
}

// BenchmarkRouter гоняет запросы через весь сервер, кроме сети: роутер, мидлвари,
// хендлер и хранилище в памяти. Логгер настоящий, но пишет в никуда.
func BenchmarkRouter(b *testing.B) {
	batch := benchBatchJSON(b)

	agentHeaders := map[string]string{
		"Content-Type":     "application/json",
		"Content-Encoding": "gzip",
		"Accept-Encoding":  "gzip",
		hash.Header:        hash.Sign(batch, benchKey),
	}
	jsonHeaders := map[string]string{"Content-Type": "application/json"}
	acceptGzip := map[string]string{"Content-Type": "application/json", "Accept-Encoding": "gzip"}

	tests := []struct {
		name    string
		method  string
		path    string
		body    []byte
		headers map[string]string
		key     string
	}{
		{
			name:   "updates_agent", // главный сценарий: сжатый подписанный пакет
			method: http.MethodPost, path: "/updates/", body: gzipped(b, batch),
			headers: agentHeaders, key: benchKey,
		},
		{
			name:   "updates_plain",
			method: http.MethodPost, path: "/updates/", body: batch,
			headers: jsonHeaders,
		},
		{
			name:   "update_json_gauge",
			method: http.MethodPost, path: "/update/", body: []byte(`{"id":"Metric00","type":"gauge","value":123.45}`),
			headers: jsonHeaders,
		},
		{
			name:   "update_json_counter_gzip",
			method: http.MethodPost, path: "/update/", body: []byte(`{"id":"PollCount","type":"counter","delta":5}`),
			headers: acceptGzip,
		},
		{name: "update_url_gauge", method: http.MethodPost, path: "/update/gauge/Metric00/123.45"},
		{name: "update_url_counter", method: http.MethodPost, path: "/update/counter/PollCount/5"},
		{
			name:   "value_json_gzip",
			method: http.MethodPost, path: "/value/", body: []byte(`{"id":"Metric00","type":"gauge"}`),
			headers: acceptGzip,
		},
		{name: "value_url", method: http.MethodGet, path: "/value/gauge/Metric00"},
		{
			name:   "index_gzip",
			method: http.MethodGet, path: "/",
			headers: map[string]string{"Accept-Encoding": "gzip"},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			ctx := context.Background()
			l := logger.New(io.Discard)

			repo := repository.NewMemStorage()
			_ = repo.SaveGauge(ctx, "Metric00", 123.45)

			var seed []model.Metrics
			if err := json.Unmarshal(batch, &seed); err != nil {
				b.Fatal(err)
			}
			_ = repo.SaveBatch(ctx, seed)

			subject := audit.NewSubject(l)
			defer subject.Close()

			router := newRouter(routerDeps{storage: repo, auditor: subject, key: tt.key, log: l})

			req := httptest.NewRequest(tt.method, tt.path, nil)
			for name, value := range tt.headers {
				req.Header.Set(name, value)
			}

			w := &discardWriter{header: make(http.Header)}
			reader := bytes.NewReader(nil)

			// Как и на настоящем сервере, у запроса без тела r.Body — http.NoBody.
			var body io.ReadCloser = http.NoBody
			if tt.body != nil {
				body = io.NopCloser(reader)
			}

			b.ReportAllocs()
			for b.Loop() {
				clear(w.header)
				reader.Reset(tt.body)
				req.Body = body

				router.ServeHTTP(w, req)

				if w.status != http.StatusOK {
					b.Fatalf("status = %d, want %d", w.status, http.StatusOK)
				}
			}
		})
	}
}
