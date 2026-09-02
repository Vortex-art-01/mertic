package agent

import (
	"context"
	"log/slog"
	"time"

	"github.com/Vortex-art-01/mertic/internal/model"
)

const pollCountMetric = "PollCount"

type MetricsSender interface {
	SendBatch(ctx context.Context, metrics []model.Metrics) error
}

var _ MetricsSender = (*Client)(nil)

type Agent struct {
	collector      *Collector
	sender         MetricsSender
	log            *slog.Logger
	pollInterval   time.Duration
	reportInterval time.Duration
}

func New(sender MetricsSender, pollInterval, reportInterval time.Duration, l *slog.Logger) *Agent {
	return &Agent{
		collector:      NewCollector(),
		sender:         sender,
		log:            l,
		pollInterval:   pollInterval,
		reportInterval: reportInterval,
	}
}

func (a *Agent) Run(ctx context.Context) {
	poll := time.NewTicker(a.pollInterval)
	defer poll.Stop()

	report := time.NewTicker(a.reportInterval)
	defer report.Stop()

	for {
		select {
		case <-poll.C:
			a.collector.Poll()
		case <-report.C:
			a.Report(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (a *Agent) Report(ctx context.Context) {
	metrics := a.batch()
	if len(metrics) == 0 {
		return
	}

	if err := a.sender.SendBatch(ctx, metrics); err != nil {
		a.log.Error("failed to send metrics",
			slog.Int("count", len(metrics)), slog.Any("error", err))
		return
	}

	a.collector.ResetPollCount()
}

func (a *Agent) batch() []model.Metrics {
	gauges := a.collector.Gauges()
	delta := a.collector.PollCount()

	metrics := make([]model.Metrics, 0, len(gauges)+1)

	for name, value := range gauges {
		metrics = append(metrics, model.Metrics{ID: name, MType: model.Gauge, Value: &value})
	}

	if delta != 0 {
		metrics = append(metrics, model.Metrics{
			ID: pollCountMetric, MType: model.Counter, Delta: &delta,
		})
	}

	return metrics
}
