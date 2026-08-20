package dump

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Vortex-art-01/mertic/internal/model"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

func newTestFile(t *testing.T, path string, restore bool) (*File, *repository.MemStorage) {
	t.Helper()

	repo := repository.NewMemStorage()
	return New(path, repo, restore, slog.New(slog.DiscardHandler)), repo
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics-db.json")

	saver, source := newTestFile(t, path, false)
	source.SaveGauge("Alloc", 123.45)
	source.AddCounter("PollCount", 42)

	if err := saver.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read dump: %v", err)
	}

	var metrics []model.Metrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		t.Fatalf("dump is not valid json: %v\n%s", err, data)
	}
	if len(metrics) != 2 {
		t.Fatalf("dump has %d metrics, want 2:\n%s", len(metrics), data)
	}

	_, restored := newTestFile(t, path, true)

	if v, ok := restored.GetGauge("Alloc"); !ok || v != 123.45 {
		t.Errorf("gauge Alloc = %v, %v; want 123.45, true", v, ok)
	}
	if v, ok := restored.GetCounter("PollCount"); !ok || v != 42 {
		t.Errorf("counter PollCount = %v, %v; want 42, true", v, ok)
	}
}

// Счётчик хранится в дампе накопленной суммой: восстановление не должно
// прибавлять её к тому, что уже есть в хранилище.
func TestLoadReplacesCounter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics-db.json")

	if err := os.WriteFile(path, []byte(`[{"id":"PollCount","type":"counter","delta":10}]`), 0o644); err != nil {
		t.Fatalf("failed to write dump: %v", err)
	}

	loader, repo := newTestFile(t, path, false)
	repo.AddCounter("PollCount", 7)

	if err := loader.load(); err != nil {
		t.Fatalf("load() error = %v", err)
	}

	if v, _ := repo.GetCounter("PollCount"); v != 10 {
		t.Errorf("counter PollCount = %d, want 10", v)
	}
}

func TestLoadMissingFile(t *testing.T) {
	loader, repo := newTestFile(t, filepath.Join(t.TempDir(), "missing.json"), false)

	if err := loader.load(); err != nil {
		t.Fatalf("load() error = %v, want nil for a missing file", err)
	}
	if got := len(repo.Gauges()) + len(repo.Counters()); got != 0 {
		t.Errorf("storage has %d metrics, want 0", got)
	}
}

func TestLoadBrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(path, []byte("{oops"), 0o644); err != nil {
		t.Fatalf("failed to write dump: %v", err)
	}

	loader, _ := newTestFile(t, path, false)
	if err := loader.load(); err == nil {
		t.Error("load() error = nil, want an error for a broken file")
	}
}

// Каталог под файл создаётся сам: путь по умолчанию указывает на /tmp,
// которого может не быть.
func TestSaveCreatesDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "metrics-db.json")

	saver, repo := newTestFile(t, path, false)
	repo.SaveGauge("Alloc", 1)

	if err := saver.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("dump was not created: %v", err)
	}
}

func TestRunSavesPeriodically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics-db.json")

	saver, repo := newTestFile(t, path, false)
	repo.SaveGauge("Alloc", 1)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		saver.Run(ctx, 10*time.Millisecond)
	}()

	deadline := time.After(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("dump was not written within the deadline")
		case <-time.After(5 * time.Millisecond):
		}
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after the context was cancelled")
	}
}

func TestSaveReplacesPreviousDump(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics-db.json")

	saver, repo := newTestFile(t, path, false)

	for _, value := range []float64{1, 2} {
		repo.SaveGauge("Alloc", value)
		if err := saver.Save(); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	_, restored := newTestFile(t, path, true)
	if v, ok := restored.GetGauge("Alloc"); !ok || v != 2 {
		t.Errorf("gauge Alloc = %v, %v; want 2, true", v, ok)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read %s: %v", dir, err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != filepath.Base(path) {
		t.Errorf("directory contains %v, want only %s", names, filepath.Base(path))
	}
}
