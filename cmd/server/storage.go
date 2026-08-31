package main

import "context"

// metricsStorage — общая часть хранилищ в памяти и в базе; сервер выбирает
// одно из них на старте (см. run) и дальше работает с ним одинаково.
type metricsStorage interface {
	SaveGauge(ctx context.Context, name string, value float64) error
	AddCounter(ctx context.Context, name string, value int64) error
	GetGauge(ctx context.Context, name string) (float64, bool, error)
	GetCounter(ctx context.Context, name string) (int64, bool, error)
	Gauges(ctx context.Context) (map[string]float64, error)
	Counters(ctx context.Context) (map[string]int64, error)
}
