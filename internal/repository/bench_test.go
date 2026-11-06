package repository

import (
	"context"
	"fmt"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/model"
)

// agentBatchSize — примерно столько метрик агент присылает за раз: 28 из
// рантайма, память, загрузка ядер и PollCount.
const agentBatchSize = 44

// benchBatch готовит пакет как у агента: все метрики gauge, а последняя — counter.
func benchBatch(size int) []model.Metrics {
	metrics := make([]model.Metrics, size)

	for i := range metrics {
		metrics[i] = model.Metrics{ID: fmt.Sprintf("Metric%04d", i), MType: model.Gauge}

		value := float64(i) * 1.5
		metrics[i].Value = &value
	}

	delta := int64(5)
	metrics[size-1] = model.Metrics{ID: "PollCount", MType: model.Counter, Delta: &delta}

	return metrics
}

func BenchmarkMemStorage(b *testing.B) {
	ctx := context.Background()
	batch := benchBatch(agentBatchSize)

	b.Run("SaveGauge", func(b *testing.B) {
		s := NewMemStorage()

		b.ReportAllocs()
		for b.Loop() {
			_ = s.SaveGauge(ctx, "Alloc", 123.45)
		}
	})

	b.Run("AddCounter", func(b *testing.B) {
		s := NewMemStorage()

		b.ReportAllocs()
		for b.Loop() {
			_ = s.AddCounter(ctx, "PollCount", 1)
		}
	})

	b.Run("GetGauge", func(b *testing.B) {
		s := NewMemStorage()
		_ = s.SaveBatch(ctx, batch)

		b.ReportAllocs()
		for b.Loop() {
			_, _, _ = s.GetGauge(ctx, batch[0].ID)
		}
	})

	b.Run("SaveBatch", func(b *testing.B) {
		s := NewMemStorage()

		b.ReportAllocs()
		for b.Loop() {
			_ = s.SaveBatch(ctx, batch)
		}
	})

	// Запись и чтение вперемешку из всех горутин: так хранилище и работает
	// под нагрузкой, и именно здесь виден спор за мьютекс.
	b.Run("Parallel", func(b *testing.B) {
		s := NewMemStorage()
		_ = s.SaveBatch(ctx, batch)

		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = s.SaveBatch(ctx, batch)
				_, _, _ = s.GetGauge(ctx, batch[0].ID)
			}
		})
	})

	// Копия всех значений нужна странице со списком и дампу.
	b.Run("Gauges", func(b *testing.B) {
		s := NewMemStorage()
		_ = s.SaveBatch(ctx, benchBatch(1000))

		b.ReportAllocs()
		for b.Loop() {
			_, _ = s.Gauges(ctx)
		}
	})
}

// Разбор пакета — единственная часть записи в PostgreSQL, которую можно замерить
// без живой базы.
func BenchmarkSplitBatch(b *testing.B) {
	batch := benchBatch(agentBatchSize)

	b.ReportAllocs()
	for b.Loop() {
		_, _ = splitBatch(batch)
	}
}
