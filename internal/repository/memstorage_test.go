package repository_test

import (
	"sync"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/model"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

func TestMemStorageSaveBatch(t *testing.T) {
	storage := repository.NewMemStorage()

	if err := storage.AddCounter(t.Context(), "PollCount", 2); err != nil {
		t.Fatalf("AddCounter() error = %v", err)
	}

	value, delta := 42.5, int64(3)
	batch := []model.Metrics{
		{ID: "Alloc", MType: model.Gauge, Value: &value},
		{ID: "PollCount", MType: model.Counter, Delta: &delta},
	}

	if err := storage.SaveBatch(t.Context(), batch); err != nil {
		t.Fatalf("SaveBatch() error = %v", err)
	}

	if got, ok, err := storage.GetGauge(t.Context(), "Alloc"); err != nil || !ok || got != value {
		t.Errorf("gauge Alloc = %v, %v, %v; want %v, true, nil", got, ok, err, value)
	}
	// Приращение складывается с уже накопленным: 2 + 3.
	if got, ok, err := storage.GetCounter(t.Context(), "PollCount"); err != nil || !ok || got != 5 {
		t.Errorf("counter PollCount = %v, %v, %v; want 5, true, nil", got, ok, err)
	}
}

// Повторы внутри пакета обрабатываются так же, как отдельные запросы.
func TestMemStorageSaveBatchWithRepeatedNames(t *testing.T) {
	storage := repository.NewMemStorage()

	first, last := 1.5, 2.25
	one, two := int64(4), int64(6)
	batch := []model.Metrics{
		{ID: "Alloc", MType: model.Gauge, Value: &first},
		{ID: "PollCount", MType: model.Counter, Delta: &one},
		{ID: "Alloc", MType: model.Gauge, Value: &last},
		{ID: "PollCount", MType: model.Counter, Delta: &two},
	}

	if err := storage.SaveBatch(t.Context(), batch); err != nil {
		t.Fatalf("SaveBatch() error = %v", err)
	}

	if got, _, _ := storage.GetGauge(t.Context(), "Alloc"); got != last {
		t.Errorf("gauge Alloc = %v, want %v", got, last)
	}
	if got, _, _ := storage.GetCounter(t.Context(), "PollCount"); got != 10 {
		t.Errorf("counter PollCount = %v, want 10", got)
	}
}

// Параллельные пакеты не должны терять приращения: под -race тест ловит и
// гонку, и потерянные обновления.
func TestMemStorageSaveBatchConcurrently(t *testing.T) {
	const (
		writers = 16
		batches = 32
	)

	storage := repository.NewMemStorage()

	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range batches {
				value, delta := 1.0, int64(1)
				err := storage.SaveBatch(t.Context(), []model.Metrics{
					{ID: "Alloc", MType: model.Gauge, Value: &value},
					{ID: "PollCount", MType: model.Counter, Delta: &delta},
				})
				if err != nil {
					t.Errorf("SaveBatch() error = %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	if got, _, _ := storage.GetCounter(t.Context(), "PollCount"); got != writers*batches {
		t.Errorf("counter PollCount = %v, want %d", got, writers*batches)
	}
}
