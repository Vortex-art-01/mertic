package dump

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/repository"
)

// benchRepo заполняет хранилище size метриками поровну двух типов.
func benchRepo(b *testing.B, size int) *repository.MemStorage {
	b.Helper()

	ctx := context.Background()
	repo := repository.NewMemStorage()

	for i := range size {
		name := fmt.Sprintf("Metric%04d", i)

		if i%2 == 0 {
			_ = repo.SaveGauge(ctx, name, float64(i)*1.5)
		} else {
			_ = repo.AddCounter(ctx, name, int64(i))
		}
	}

	return repo
}

// 44 метрики присылает один агент, 1000 — запас на пользовательские метрики.
func BenchmarkFile(b *testing.B) {
	ctx := context.Background()
	l := slog.New(slog.DiscardHandler)

	for _, size := range []int{44, 1000} {
		repo := benchRepo(b, size)

		b.Run(fmt.Sprintf("snapshot/%d", size), func(b *testing.B) {
			f := newFile(ctx, filepath.Join(b.TempDir(), "metrics-db.json"), repo, false, l)

			b.ReportAllocs()
			for b.Loop() {
				if _, err := f.snapshot(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})

		// В режиме STORE_INTERVAL=0 такая запись идёт на каждое изменение.
		b.Run(fmt.Sprintf("save/%d", size), func(b *testing.B) {
			f := newFile(ctx, filepath.Join(b.TempDir(), "metrics-db.json"), repo, false, l)

			b.ReportAllocs()
			for b.Loop() {
				if err := f.save(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run(fmt.Sprintf("load/%d", size), func(b *testing.B) {
			f := newFile(ctx, filepath.Join(b.TempDir(), "metrics-db.json"), repo, false, l)
			if err := f.save(ctx); err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()
			for b.Loop() {
				if err := f.load(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
