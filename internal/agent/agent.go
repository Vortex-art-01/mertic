// Package agent собирает метрики рантайма Go и системы и отправляет их на сервер.
//
// [Collector] накапливает метрики между отправками, [Client] отправляет пакет на
// POST /updates/, а [Agent] запускает их по расписанию: опрашивает источники
// каждые pollInterval, раз в reportInterval собирает пакет и передаёт его
// воркерам, число которых ограничивает одновременные запросы к серверу.
package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Vortex-art-01/mertic/internal/model"
)

// MetricsSender отправляет пакет метрик на сервер. Его реализует [Client].
type MetricsSender interface {
	SendBatch(ctx context.Context, metrics []model.Metrics) error
}

var _ MetricsSender = (*Client)(nil)

type batch struct {
	metrics []model.Metrics
	delta   int64
}

// Agent опрашивает метрики и отправляет их на сервер по расписанию. Создаётся
// функцией [New] и запускается методом [Agent.Run].
type Agent struct {
	collector      *Collector
	sender         MetricsSender
	log            *slog.Logger
	pollInterval   time.Duration
	reportInterval time.Duration
	rateLimit      int
}

// New возвращает агента, который каждые pollInterval опрашивает метрики, раз в
// reportInterval отправляет накопленное через sender и держит не больше
// rateLimit одновременных отправок; rateLimit меньше единицы считается единицей.
// Ошибки опроса и отправки агент пишет в l.
func New(sender MetricsSender, pollInterval, reportInterval time.Duration, rateLimit int, l *slog.Logger) *Agent {
	if rateLimit < 1 {
		rateLimit = 1
	}

	return &Agent{
		collector:      NewCollector(),
		sender:         sender,
		log:            l,
		pollInterval:   pollInterval,
		reportInterval: reportInterval,
		rateLimit:      rateLimit,
	}
}

// Run опрашивает и отправляет метрики, пока не отменён ctx, и возвращается, когда
// остановятся все горутины агента. Если пакет так и не удалось отправить,
// приращение PollCount не теряется: оно уйдёт со следующим пакетом.
func (a *Agent) Run(ctx context.Context) {
	batches := make(chan batch)

	sources := []func(context.Context){
		a.pollRuntime,
		a.pollSystem,
		func(ctx context.Context) { a.report(ctx, batches) },
	}

	var running sync.WaitGroup

	for _, source := range sources {
		running.Go(func() { source(ctx) })
	}

	var workers sync.WaitGroup

	for range a.rateLimit {
		workers.Go(func() {
			for b := range batches {
				a.send(ctx, b)
			}
		})
	}

	running.Wait()
	close(batches)
	workers.Wait()
}

func (a *Agent) pollRuntime(ctx context.Context) {
	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.collector.PollRuntime()
		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) pollSystem(ctx context.Context) {
	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := a.collector.PollSystem(ctx); err != nil {
				a.log.Error("failed to poll system metrics", slog.Any("error", err))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) report(ctx context.Context, batches chan<- batch) {
	ticker := time.NewTicker(a.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b := a.collect()

			select {
			case batches <- b:
			case <-ctx.Done():
				// Пакет уже никто не разберёт — возвращаем приращение,
				// чтобы оно не потерялось вместе с ним.
				a.collector.AddPollCount(b.delta)
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) collect() batch {
	gauges, delta := a.collector.Take()

	metrics := make([]model.Metrics, 0, len(gauges)+1)

	for name, value := range gauges {
		metrics = append(metrics, model.Metrics{ID: name, MType: model.Gauge, Value: &value})
	}

	if delta != 0 {
		metrics = append(metrics, model.Metrics{
			ID: pollCountMetric, MType: model.Counter, Delta: &delta,
		})
	}

	return batch{metrics: metrics, delta: delta}
}

func (a *Agent) send(ctx context.Context, b batch) {
	if len(b.metrics) == 0 {
		return
	}

	if err := a.sender.SendBatch(ctx, b.metrics); err != nil {
		a.log.Error("failed to send metrics",
			slog.Int("count", len(b.metrics)), slog.Any("error", err))

		a.collector.AddPollCount(b.delta)
	}
}
