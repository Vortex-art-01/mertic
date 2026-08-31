package repository_test

import (
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Vortex-art-01/mertic/internal/database"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

// newPostgres готовит хранилище на живой базе. Без TEST_DATABASE_DSN тест
// пропускается: поднимать PostgreSQL ради `go test ./...` не хочется, но и
// оставлять SQL непроверенным — тоже.
//
//	TEST_DATABASE_DSN='postgres://postgres:postgres@localhost:5432/praktikum?sslmode=disable' go test ./internal/repository/
func newPostgres(t *testing.T) (*repository.Postgres, *sql.DB) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN is not set")
	}

	db, err := database.New(dsn)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close database: %v", err)
		}
	})

	if err := database.Migrate(t.Context(), db); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	return repository.NewPostgres(db), db
}

// uniqueName разводит параллельные и повторные прогоны по разным строкам:
// база одна на всех, и общая таблица не чистится между тестами.
func uniqueName(t *testing.T, db *sql.DB, table string) string {
	t.Helper()

	name := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())

	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM `+table+` WHERE name = $1`, name); err != nil {
			t.Errorf("failed to clean up %s: %v", name, err)
		}
	})

	return name
}

func TestPostgresKeepsLastGauge(t *testing.T) {
	storage, db := newPostgres(t)
	name := uniqueName(t, db, "gauges")

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
	storage, db := newPostgres(t)
	name := uniqueName(t, db, "counters")

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
	storage, db := newPostgres(t)
	gaugeName := uniqueName(t, db, "gauges")
	counterName := uniqueName(t, db, "counters")

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
