package database

import (
	"math"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/Vortex-art-01/mertic/migrations"
)

// Проверка без базы: миграции лежат во встроенной ФС, и goose находит их по
// тому же пути, который использует Migrate. Опечатка в go:embed или в имени
// каталога иначе всплыла бы только на живой базе.
func TestMigrationsAreCollected(t *testing.T) {
	goose.SetBaseFS(migrations.FS)

	collected, err := goose.CollectMigrations(".", 0, math.MaxInt64)
	if err != nil {
		t.Fatalf("CollectMigrations() error = %v", err)
	}
	if len(collected) == 0 {
		t.Fatal("no migrations found in the embedded filesystem")
	}

	for _, m := range collected {
		if m.Version <= 0 {
			t.Errorf("migration %s has version %d, want a positive number", m.Source, m.Version)
		}
	}
}
