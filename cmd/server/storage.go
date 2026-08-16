package main

import (
	"log/slog"

	"github.com/Vortex-art-01/mertic/internal/dump"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

type metricsStorage interface {
	SaveGauge(name string, value float64)
	AddCounter(name string, value int64)
	GetGauge(name string) (float64, bool)
	GetCounter(name string) (int64, bool)
	Gauges() map[string]float64
	Counters() map[string]int64
}

type syncStorage struct {
	*repository.MemStorage
	dump *dump.File
	l    *slog.Logger
}

func (s *syncStorage) SaveGauge(name string, value float64) {
	s.MemStorage.SaveGauge(name, value)
	s.flush()
}

func (s *syncStorage) AddCounter(name string, value int64) {
	s.MemStorage.AddCounter(name, value)
	s.flush()
}

func (s *syncStorage) flush() {
	if err := s.dump.Save(); err != nil {
		s.l.Error("failed to save metrics", slog.Any("error", err))
	}
}
