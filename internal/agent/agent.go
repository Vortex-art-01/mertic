package agent

import (
	"log/slog"
	"time"
)

type MetricsSender interface {
	SendGauge(name string, value float64) error
	SendCounter(name string, delta int64) error
}

var _ MetricsSender = (*Client)(nil)

type Agent struct {
	collector      *Collector
	sender         MetricsSender
	log            *slog.Logger
	pollInterval   time.Duration
	reportInterval time.Duration
	reportedPolls  int64
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

// Run запускает бесконечный цикл сбора и отправки метрик.
func (a *Agent) Run() {
	lastReport := time.Now()

	for {
		time.Sleep(a.pollInterval)
		a.collector.Poll()

		if time.Since(lastReport) >= a.reportInterval {
			a.Report()
			lastReport = time.Now()
		}
	}
}

func (a *Agent) Report() {
	for name, value := range a.collector.Gauges() {
		if err := a.sender.SendGauge(name, value); err != nil {
			a.log.Error("failed to send gauge",
				slog.String("metric", name), slog.Any("error", err))
		}
	}

	if err := a.sender.SendCounter("PollCount", a.collector.PollCount()); err != nil {
		a.log.Error("failed to send counter",
			slog.String("metric", "PollCount"), slog.Any("error", err))
		return
	}
	a.collector.pollCount = 0
}
