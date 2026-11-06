package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Vortex-art-01/mertic/internal/model"
)

// benchBatch — пакет, как его отправляет агент: 43 gauge и PollCount.
func benchBatch() []model.Metrics {
	metrics := make([]model.Metrics, 44)

	for i := range metrics {
		value := float64(i) * 1.5
		metrics[i] = model.Metrics{ID: fmt.Sprintf("Metric%04d", i), MType: model.Gauge, Value: &value}
	}

	delta := int64(5)
	metrics[len(metrics)-1] = model.Metrics{ID: pollCountMetric, MType: model.Counter, Delta: &delta}

	return metrics
}

func BenchmarkCollector(b *testing.B) {
	ctx := context.Background()

	b.Run("PollRuntime", func(b *testing.B) {
		c := NewCollector()

		b.ReportAllocs()
		for b.Loop() {
			c.PollRuntime()
		}
	})

	b.Run("PollSystem", func(b *testing.B) {
		c := NewCollector()

		b.ReportAllocs()
		for b.Loop() {
			if err := c.PollSystem(ctx); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Take", func(b *testing.B) {
		c := NewCollector()
		c.PollRuntime()

		b.ReportAllocs()
		for b.Loop() {
			_, _ = c.Take()
		}
	})
}

// Сборка пакета из накопленных значений перед отправкой.
func BenchmarkAgentCollect(b *testing.B) {
	a := New(newSenderMock(), time.Second, time.Second, 1, slog.New(slog.DiscardHandler))
	a.collector.PollRuntime()

	b.ReportAllocs()
	for b.Loop() {
		_ = a.collect()
	}
}

func BenchmarkGzipped(b *testing.B) {
	data, err := json.Marshal(benchBatch())
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := gzipped(data); err != nil {
			b.Fatal(err)
		}
	}
}

// Вся отправка: JSON, gzip, подпись и запрос по настоящему соединению. Принимающий
// сервер работает в том же процессе, поэтому его аллокации тоже попадают в замер.
func BenchmarkClientSendBatch(b *testing.B) {
	ctx := context.Background()
	metrics := benchBatch()

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "secret")

	b.ReportAllocs()
	for b.Loop() {
		if err := client.SendBatch(ctx, metrics); err != nil {
			b.Fatal(err)
		}
	}
}
