package repository

import (
	"maps"
	"sync"
)

type Repository interface {
	SaveGauge(name string, value float64)
	AddCounter(name string, value int64)
	GetGauge(name string) (float64, bool)
	GetCounter(name string) (int64, bool)
	Gauges() map[string]float64
	Counters() map[string]int64
}

type MemStorage struct {
	mu       sync.RWMutex
	gauges   map[string]float64
	counters map[string]int64
}

var _ Repository = (*MemStorage)(nil)

func NewMemStorage() *MemStorage {
	return &MemStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (s *MemStorage) SaveGauge(name string, value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gauges[name] = value
}

func (s *MemStorage) AddCounter(name string, value int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counters[name] += value
}

func (s *MemStorage) GetGauge(name string) (float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.gauges[name]
	return v, ok
}

func (s *MemStorage) GetCounter(name string) (int64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.counters[name]
	return v, ok
}

func (s *MemStorage) Gauges() map[string]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return maps.Clone(s.gauges)
}

func (s *MemStorage) Counters() map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return maps.Clone(s.counters)
}
