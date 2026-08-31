package dump

import (
	"context"
	"io"
	"log/slog"
	"time"
)

type Storage interface {
	Gauges() map[string]float64
	Counters() map[string]int64
	SaveGauge(name string, value float64)
	SetCounter(name string, value int64)
}

type Metrics interface {
	Storage
	AddCounter(name string, value int64)
	GetGauge(name string) (float64, bool)
	GetCounter(name string) (int64, bool)
}

type Config struct {
	Path     string
	Interval time.Duration
	Restore  bool
}

func Attach(ctx context.Context, metrics Metrics, cfg Config, l *slog.Logger) (Metrics, io.Closer) {
	if cfg.Path == "" {
		return metrics, noopCloser{}
	}

	f := newFile(cfg.Path, metrics, cfg.Restore, l)

	if cfg.Interval > 0 {
		go f.run(ctx, cfg.Interval)
		return metrics, f
	}

	return &syncStorage{Metrics: metrics, file: f, l: l}, f
}

type syncStorage struct {
	Metrics
	file *File
	l    *slog.Logger
}

func (s *syncStorage) SaveGauge(name string, value float64) {
	s.Metrics.SaveGauge(name, value)
	s.flush()
}

func (s *syncStorage) AddCounter(name string, value int64) {
	s.Metrics.AddCounter(name, value)
	s.flush()
}

func (s *syncStorage) flush() {
	if err := s.file.save(); err != nil {
		s.l.Error("failed to save metrics", slog.Any("error", err))
	}
}

type noopCloser struct{}

func (noopCloser) Close() error { return nil }
