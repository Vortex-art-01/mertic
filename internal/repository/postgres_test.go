package repository_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vortex-art-01/mertic/internal/database"
	"github.com/Vortex-art-01/mertic/internal/model"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

// newPostgres готовит хранилище на живой базе. Без TEST_DATABASE_DSN тест
// пропускается: поднимать PostgreSQL ради `go test ./...` не хочется, но и
// оставлять SQL непроверенным — тоже.
//
//	TEST_DATABASE_DSN='postgres://postgres:postgres@localhost:5432/praktikum?sslmode=disable' go test ./internal/repository/
func newPostgres(t *testing.T) (*repository.Postgres, *pgxpool.Pool) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set")
	}

	pool, err := database.New(t.Context(), dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(pool.Close)

	return repository.NewPostgres(pool), pool
}

// uniqueName разводит параллельные и повторные прогоны по разным строкам:
// база одна на всех, и общая таблица не чистится между тестами.
func uniqueName(t *testing.T, pool *pgxpool.Pool, table string) string {
	t.Helper()

	name := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())

	t.Cleanup(func() {
		// Контекст теста к этому моменту уже отменён, а строку убрать надо.
		if _, err := pool.Exec(context.Background(), `DELETE FROM `+table+` WHERE name = $1`, name); err != nil {
			t.Errorf("failed to clean up %s: %v", name, err)
		}
	})

	return name
}

func TestPostgresKeepsLastGauge(t *testing.T) {
	storage, pool := newPostgres(t)
	name := uniqueName(t, pool, "gauges")

	for _, want := range []float64{1.5, 2.25} {
		if err := storage.SaveGauge(t.Context(), name, want); err != nil {
			t.Fatalf("SaveGauge() error = %v", err)
		}

		got, ok, err := storage.GetGauge(t.Context(), name)
		if err != nil {
			t.Fatalf("GetGauge() error = %v", err)
		}
		if !ok || got != want {
			t.Errorf("gauge %s = %v, %v; want %v, true", name, got, ok, want)
		}
	}
}

// Приращения складывает база: сервер их не читает и не перезаписывает.
func TestPostgresAccumulatesCounter(t *testing.T) {
	storage, pool := newPostgres(t)
	name := uniqueName(t, pool, "counters")

	for _, delta := range []int64{3, 4} {
		if err := storage.AddCounter(t.Context(), name, delta); err != nil {
			t.Fatalf("AddCounter() error = %v", err)
		}
	}

	got, ok, err := storage.GetCounter(t.Context(), name)
	if err != nil {
		t.Fatalf("GetCounter() error = %v", err)
	}
	if !ok || got != 7 {
		t.Errorf("counter %s = %v, %v; want 7, true", name, got, ok)
	}
}

// Отсутствующая метрика — не ошибка: хендлеры отличают её по флагу.
func TestPostgresMissingMetric(t *testing.T) {
	storage, _ := newPostgres(t)

	if v, ok, err := storage.GetGauge(t.Context(), "missing"); err != nil || ok || v != 0 {
		t.Errorf("GetGauge() = %v, %v, %v; want 0, false, nil", v, ok, err)
	}
	if v, ok, err := storage.GetCounter(t.Context(), "missing"); err != nil || ok || v != 0 {
		t.Errorf("GetCounter() = %v, %v, %v; want 0, false, nil", v, ok, err)
	}
}

func TestPostgresListsMetrics(t *testing.T) {
	storage, pool := newPostgres(t)
	gaugeName := uniqueName(t, pool, "gauges")
	counterName := uniqueName(t, pool, "counters")

	if err := storage.SaveGauge(t.Context(), gaugeName, 10.5); err != nil {
		t.Fatalf("SaveGauge() error = %v", err)
	}
	if err := storage.AddCounter(t.Context(), counterName, 5); err != nil {
		t.Fatalf("AddCounter() error = %v", err)
	}

	gauges, err := storage.Gauges(t.Context())
	if err != nil {
		t.Fatalf("Gauges() error = %v", err)
	}
	if gauges[gaugeName] != 10.5 {
		t.Errorf("gauges[%s] = %v, want 10.5", gaugeName, gauges[gaugeName])
	}

	counters, err := storage.Counters(t.Context())
	if err != nil {
		t.Fatalf("Counters() error = %v", err)
	}
	if counters[counterName] != 5 {
		t.Errorf("counters[%s] = %v, want 5", counterName, counters[counterName])
	}
}

// SaveBatch пишет весь пакет за одну транзакцию: gauge перезаписывается,
// приращения counter складываются с тем, что уже лежит в базе.
func TestPostgresSaveBatch(t *testing.T) {
	storage, pool := newPostgres(t)
	gaugeName := uniqueName(t, pool, "gauges")
	counterName := uniqueName(t, pool, "counters")

	if err := storage.AddCounter(t.Context(), counterName, 2); err != nil {
		t.Fatalf("AddCounter() error = %v", err)
	}

	value, delta := 42.5, int64(3)
	batch := []model.Metrics{
		{ID: gaugeName, MType: model.Gauge, Value: &value},
		{ID: counterName, MType: model.Counter, Delta: &delta},
	}

	if err := storage.SaveBatch(t.Context(), batch); err != nil {
		t.Fatalf("SaveBatch() error = %v", err)
	}

	if got, ok, err := storage.GetGauge(t.Context(), gaugeName); err != nil || !ok || got != value {
		t.Errorf("gauge %s = %v, %v, %v; want %v, true, nil", gaugeName, got, ok, err, value)
	}
	if got, ok, err := storage.GetCounter(t.Context(), counterName); err != nil || !ok || got != 5 {
		t.Errorf("counter %s = %v, %v, %v; want 5, true, nil", counterName, got, ok, err)
	}
}

// Повторы внутри пакета — обычное дело: многострочный INSERT ... ON CONFLICT
// не может изменить одну строку дважды, поэтому имена схлопываются заранее.
func TestPostgresSaveBatchWithRepeatedNames(t *testing.T) {
	storage, pool := newPostgres(t)
	gaugeName := uniqueName(t, pool, "gauges")
	counterName := uniqueName(t, pool, "counters")

	first, last := 1.5, 2.25
	one, two := int64(4), int64(6)
	batch := []model.Metrics{
		{ID: gaugeName, MType: model.Gauge, Value: &first},
		{ID: counterName, MType: model.Counter, Delta: &one},
		{ID: gaugeName, MType: model.Gauge, Value: &last},
		{ID: counterName, MType: model.Counter, Delta: &two},
	}

	if err := storage.SaveBatch(t.Context(), batch); err != nil {
		t.Fatalf("SaveBatch() error = %v", err)
	}

	// У gauge остаётся последнее значение, приращения counter складываются.
	if got, _, err := storage.GetGauge(t.Context(), gaugeName); err != nil || got != last {
		t.Errorf("gauge %s = %v, %v; want %v", gaugeName, got, err, last)
	}
	if got, _, err := storage.GetCounter(t.Context(), counterName); err != nil || got != 10 {
		t.Errorf("counter %s = %v, %v; want 10", counterName, got, err)
	}
}

// Пустой пакет до базы не доходит и ошибкой не считается.
func TestPostgresSaveEmptyBatch(t *testing.T) {
	storage, _ := newPostgres(t)

	if err := storage.SaveBatch(t.Context(), nil); err != nil {
		t.Errorf("SaveBatch() error = %v, want nil", err)
	}
}
