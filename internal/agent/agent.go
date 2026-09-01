package agent

import (
	"log/slog"
	"time"

	"github.com/Vortex-art-01/mertic/internal/model"
)

// pollCountMetric — имя счётчика опросов; сервер складывает присланные
// приращения сам.
const pollCountMetric = "PollCount"

type MetricsSender interface {
	SendBatch(metrics []model.Metrics) error
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

// Report отправляет всё собранное одним пакетом. Счётчик опросов сбрасывается
// только после успеха: неудачная отправка не должна терять приращения —
// следующий отчёт унесёт их вместе со своими.
func (a *Agent) Report() {
	metrics := a.batch()
	if len(metrics) == 0 {
		return
	}

	if err := a.sender.SendBatch(metrics); err != nil {
		a.log.Error("failed to send metrics",
			slog.Int("count", len(metrics)), slog.Any("error", err))
		return
	}

	a.collector.ResetPollCount()
}

// batch складывает собранное в один список: gauge как есть, накопленные
// опросы — приращением counter. Пустой список означает, что отправлять нечего.
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
