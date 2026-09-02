package dump

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Vortex-art-01/mertic/internal/repository"
)

func attach(t *testing.T, cfg Config) (Metrics, *repository.MemStorage) {
	t.Helper()

	repo := repository.NewMemStorage()

	storage, closer := Attach(t.Context(), repo, cfg, slog.New(slog.DiscardHandler))
	t.Cleanup(func() {
		if err := closer.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	return storage, repo
}

// Нулевой интервал — синхронный режим: дамп должен оказаться на диске сразу
// после обновления, без ожидания тика и без Save() со стороны вызывающего кода.
func TestAttachSyncWritesDumpOnUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics-db.json")

	storage, _ := attach(t, Config{Path: path})

	if err := storage.SaveGauge(t.Context(), "Alloc", 42.5); err != nil {
		t.Fatalf("SaveGauge() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("dump was not written after SaveGauge: %v", err)
	}

	if err := storage.AddCounter(t.Context(), "PollCount", 3); err != nil {
		t.Fatalf("AddCounter() error = %v", err)
	}

	_, restored := attach(t, Config{Path: path, Restore: true})
	if v, ok, _ := restored.GetGauge(t.Context(), "Alloc"); !ok || v != 42.5 {
		t.Errorf("gauge Alloc = %v, %v; want 42.5, true", v, ok)
	}
	if v, ok, _ := restored.GetCounter(t.Context(), "PollCount"); !ok || v != 3 {
		t.Errorf("counter PollCount = %v, %v; want 3, true", v, ok)
	}
}

// Обновления идут мимо обёртки: в асинхронном режиме их на диск относит
// фоновая запись по тикам.
func TestAttachAsyncWritesDumpPeriodically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics-db.json")

	storage, _ := attach(t, Config{Path: path, Interval: 10 * time.Millisecond})
	if err := storage.SaveGauge(t.Context(), "Alloc", 1); err != nil {
		t.Fatalf("SaveGauge() error = %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		select {
		case <-deadline:
			t.Fatal("dump was not written within the deadline")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// Close дописывает то, что фоновая запись не успела сохранить.
func TestAttachCloseWritesFinalDump(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics-db.json")

	repo := repository.NewMemStorage()
	storage, closer := Attach(t.Context(), repo, Config{Path: path, Interval: time.Hour},
		slog.New(slog.DiscardHandler))

	if err := storage.SaveGauge(t.Context(), "Alloc", 7); err != nil {
		t.Fatalf("SaveGauge() error = %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("dump was written before the first tick")
	}

	if err := closer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	_, restored := attach(t, Config{Path: path, Restore: true})
	if v, ok, _ := restored.GetGauge(t.Context(), "Alloc"); !ok || v != 7 {
		t.Errorf("gauge Alloc = %v, %v; want 7, true", v, ok)
	}
}

// Без файла хранилище остаётся тем же самым, а Close ничего не делает.
func TestAttachWithoutPathKeepsStorage(t *testing.T) {
	dir := t.TempDir()

	storage, repo := attach(t, Config{Path: ""})
	if storage != Metrics(repo) {
		t.Error("Attach returned a wrapper for an empty path, want the storage itself")
	}

	if err := storage.SaveGauge(t.Context(), "Alloc", 1); err != nil {
		t.Fatalf("SaveGauge() error = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read %s: %v", dir, err)
	}
	if len(entries) != 0 {
		t.Errorf("directory contains %d files, want none", len(entries))
	}
}
