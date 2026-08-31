// Package dump сохраняет значения метрик в файл и восстанавливает их
// при старте сервера.
package dump

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/Vortex-art-01/mertic/internal/model"
)

type File struct {
	mu      sync.Mutex
	path    string
	storage Storage
	l       *slog.Logger
}

// newFile создаёт дамп и, если попросили, сразу восстанавливает из него
// метрики: отдельного шага инициализации у файла нет.
func newFile(ctx context.Context, path string, storage Storage, restore bool, l *slog.Logger) *File {
	f := &File{path: path, storage: storage, l: l}

	if restore {
		if err := f.load(ctx); err != nil {
			l.Warn("failed to restore metrics", slog.Any("error", err))
		}
	}

	return f
}

// Close дописывает последний дамп. Контекст сервера к этому моменту уже
// отменён, поэтому последняя запись идёт с чистым.
func (f *File) Close() error {
	return f.save(context.Background())
}

func (f *File) save(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	metrics, err := f.snapshot(ctx)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("encode metrics: %w", err)
	}

	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(f.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", f.path, err)
	}

	defer os.Remove(tmp.Name())

	if err := writeAndClose(tmp, data); err != nil {
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}

	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return fmt.Errorf("chmod %s: %w", tmp.Name(), err)
	}

	if err := os.Rename(tmp.Name(), f.path); err != nil {
		return fmt.Errorf("replace %s: %w", f.path, err)
	}

	return nil
}

func writeAndClose(file *os.File, data []byte) error {
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}

	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}

	return file.Close()
}

func (f *File) load(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := os.ReadFile(f.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		f.l.Info("dump file not found, starting with empty storage",
			slog.String("file", f.path))
		return nil
	case err != nil:
		return fmt.Errorf("read %s: %w", f.path, err)
	case len(data) == 0:
		return nil
	}

	var metrics []model.Metrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return fmt.Errorf("decode %s: %w", f.path, err)
	}

	for _, m := range metrics {
		var err error

		switch m.MType {
		case model.Gauge:
			if m.Value != nil {
				err = f.storage.SaveGauge(ctx, m.ID, *m.Value)
			}
		case model.Counter:
			if m.Delta != nil {
				err = f.storage.SetCounter(ctx, m.ID, *m.Delta)
			}
		default:
			f.l.Warn("skipping metric of unknown type",
				slog.String("metric", m.ID), slog.String("type", m.MType))
		}

		if err != nil {
			return fmt.Errorf("restore %s: %w", m.ID, err)
		}
	}

	f.l.Info("metrics restored",
		slog.String("file", f.path), slog.Int("count", len(metrics)))

	return nil
}

func (f *File) run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := f.save(ctx); err != nil {
				f.l.Error("failed to save metrics", slog.Any("error", err))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (f *File) snapshot(ctx context.Context) ([]model.Metrics, error) {
	gauges, err := f.storage.Gauges(ctx)
	if err != nil {
		return nil, err
	}

	counters, err := f.storage.Counters(ctx)
	if err != nil {
		return nil, err
	}

	metrics := make([]model.Metrics, 0, len(gauges)+len(counters))

	for _, name := range slices.Sorted(maps.Keys(gauges)) {
		value := gauges[name]
		metrics = append(metrics, model.Metrics{ID: name, MType: model.Gauge, Value: &value})
	}
	for _, name := range slices.Sorted(maps.Keys(counters)) {
		delta := counters[name]
		metrics = append(metrics, model.Metrics{ID: name, MType: model.Counter, Delta: &delta})
	}

	return metrics, nil
}
