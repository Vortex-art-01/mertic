package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Vortex-art-01/mertic/internal/model"
)

type MetricsSender interface {
	SendBatch(ctx context.Context, metrics []model.Metrics) error
}

var _ MetricsSender = (*Client)(nil)

type batch struct {
	metrics []model.Metrics
	delta   int64
}

type Agent struct {
	collector      *Collector
	sender         MetricsSender
	log            *slog.Logger
	pollInterval   time.Duration
	reportInterval time.Duration
	rateLimit      int
}

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

func (a *Agent) Run(ctx context.Context) {
	batches := make(chan batch)

	var sources sync.WaitGroup
	sources.Add(3)

	go func() {
		defer sources.Done()
		a.pollRuntime(ctx)
	}()

	go func() {
		defer sources.Done()
		a.pollSystem(ctx)
	}()

	go func() {
		defer sources.Done()
		a.report(ctx, batches)
	}()

	var workers sync.WaitGroup
	workers.Add(a.rateLimit)

	for range a.rateLimit {
		go func() {
			defer workers.Done()

			for b := range batches {
				a.send(ctx, b)
			}
		}()
	}

	sources.Wait()
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
