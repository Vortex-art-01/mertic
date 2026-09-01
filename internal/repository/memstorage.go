// Package repository содержит хранилища метрик: в памяти и в PostgreSQL.
//
// Оба реализуют один набор методов, поэтому сервер выбирает нужное на старте
// и дальше работает с ним одинаково. Контекст и ошибка в сигнатурах нужны
// хранилищу в базе; MemStorage их не использует, но повторяет форму, чтобы
// хранилища оставались взаимозаменяемыми.
package repository

import (
	"context"
	"maps"
	"sync"

	"github.com/Vortex-art-01/mertic/internal/model"
)

type MemStorage struct {
	mu       sync.Mutex
	gauges   map[string]float64
	counters map[string]int64
}

func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (s *MemStorage) SaveGauge(_ context.Context, name string, value float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gauges[name] = value
	return nil
}

func (s *MemStorage) AddCounter(_ context.Context, name string, value int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters[name] += value
	return nil
}

// SetCounter записывает абсолютное значение счётчика — в отличие от
// AddCounter, который накапливает приращения. Нужен при восстановлении
// из дампа, где лежит уже накопленная сумма.
func (s *MemStorage) SetCounter(_ context.Context, name string, value int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters[name] = value
	return nil
}

// SaveBatch применяет весь пакет метрик под одной блокировкой: параллельные
// запросы видят хранилище либо до пакета, либо после него целиком.
//
// Повторяющиеся имена внутри пакета обрабатываются так же, как отдельные
// запросы: у gauge остаётся последнее значение, приращения counter
// складываются.
func (s *MemStorage) SaveBatch(_ context.Context, metrics []model.Metrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, m := range metrics {
		switch m.MType {
		case model.Gauge:
			if m.Value != nil {
				s.gauges[m.ID] = *m.Value
			}
		case model.Counter:
			if m.Delta != nil {
				s.counters[m.ID] += *m.Delta
			}
		}
	}

	return nil
}

func (s *MemStorage) GetGauge(_ context.Context, name string) (float64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.gauges[name]
	return v, ok, nil
}

func (s *MemStorage) GetCounter(_ context.Context, name string) (int64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.counters[name]
	return v, ok, nil
}

func (s *MemStorage) Gauges(_ context.Context) (map[string]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.gauges), nil
}

func (s *MemStorage) Counters(_ context.Context) (map[string]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return maps.Clone(s.counters), nil
}
