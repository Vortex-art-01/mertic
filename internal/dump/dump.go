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

type Storage interface {
	Gauges() map[string]float64
	Counters() map[string]int64
	SaveGauge(name string, value float64)
	SetCounter(name string, value int64)
}

type File struct {
	mu      sync.Mutex
	path    string
	storage Storage
	l       *slog.Logger
}

func New(path string, storage Storage, restore bool, l *slog.Logger) *File {
	f := &File{path: path, storage: storage, l: l}

	if restore {
		if err := f.load(); err != nil {
			l.Warn("failed to restore metrics", slog.Any("error", err))
		}
	}

	return f
}

func (f *File) Path() string {
	return f.path
}

func (f *File) Save() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	data, err := json.MarshalIndent(f.snapshot(), "", "  ")
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

func (f *File) load() error {
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
		switch m.MType {
		case model.Gauge:
			if m.Value != nil {
				f.storage.SaveGauge(m.ID, *m.Value)
			}
		case model.Counter:
			if m.Delta != nil {
				f.storage.SetCounter(m.ID, *m.Delta)
			}
		default:
			f.l.Warn("skipping metric of unknown type",
				slog.String("metric", m.ID), slog.String("type", m.MType))
		}
	}

	f.l.Info("metrics restored",
		slog.String("file", f.path), slog.Int("count", len(metrics)))

	return nil
}

func (f *File) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := f.Save(); err != nil {
				f.l.Error("failed to save metrics", slog.Any("error", err))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (f *File) snapshot() []model.Metrics {
	gauges := f.storage.Gauges()
	counters := f.storage.Counters()

	metrics := make([]model.Metrics, 0, len(gauges)+len(counters))

	for _, name := range slices.Sorted(maps.Keys(gauges)) {
		value := gauges[name]
		metrics = append(metrics, model.Metrics{ID: name, MType: model.Gauge, Value: &value})
	}
	for _, name := range slices.Sorted(maps.Keys(counters)) {
		delta := counters[name]
		metrics = append(metrics, model.Metrics{ID: name, MType: model.Counter, Delta: &delta})
	}

	return metrics
}
