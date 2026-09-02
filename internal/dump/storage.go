package dump

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/Vortex-art-01/mertic/internal/model"
)

type Storage interface {
	Gauges(ctx context.Context) (map[string]float64, error)
	Counters(ctx context.Context) (map[string]int64, error)
	SaveGauge(ctx context.Context, name string, value float64) error
	SetCounter(ctx context.Context, name string, value int64) error
}

type Metrics interface {
	Storage
	AddCounter(ctx context.Context, name string, value int64) error
	SaveBatch(ctx context.Context, metrics []model.Metrics) error
	GetGauge(ctx context.Context, name string) (float64, bool, error)
	GetCounter(ctx context.Context, name string) (int64, bool, error)
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

	f := newFile(ctx, cfg.Path, metrics, cfg.Restore, l)

	if cfg.Interval > 0 {
		go f.run(ctx, cfg.Interval)
		return metrics, f
	}

	return &syncStorage{Metrics: metrics, file: f, l: l}, f
}

// syncStorage сбрасывает дамп после каждой записи — режим STORE_INTERVAL=0.
type syncStorage struct {
	Metrics
	file *File
	l    *slog.Logger
}

func (s *syncStorage) SaveGauge(ctx context.Context, name string, value float64) error {
	if err := s.Metrics.SaveGauge(ctx, name, value); err != nil {
		return err
	}

	s.flush(ctx)

	return nil
}

func (s *syncStorage) AddCounter(ctx context.Context, name string, value int64) error {
	if err := s.Metrics.AddCounter(ctx, name, value); err != nil {
		return err
	}

	s.flush(ctx)

	return nil
}

// SaveBatch сбрасывает дамп один раз на весь пакет, а не на каждую метрику.
func (s *syncStorage) SaveBatch(ctx context.Context, metrics []model.Metrics) error {
	if err := s.Metrics.SaveBatch(ctx, metrics); err != nil {
		return err
	}

	s.flush(ctx)

	return nil
}

// flush не возвращает ошибку: метрика уже принята хранилищем, и отвечать
// клиенту отказом из-за недоступного диска не за что.
func (s *syncStorage) flush(ctx context.Context) {
	if err := s.file.save(ctx); err != nil {
		s.l.Error("failed to save metrics", slog.Any("error", err))
	}
}

type noopCloser struct{}

func (noopCloser) Close() error { return nil }
