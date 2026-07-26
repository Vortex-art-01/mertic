package agent

import (
	"log"
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
	pollInterval   time.Duration
	reportInterval time.Duration
	reportedPolls  int64
}

func New(sender MetricsSender, pollInterval, reportInterval time.Duration) *Agent {
	return &Agent{
		collector:      NewCollector(),
		sender:         sender,
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
			log.Println(err)
		}
	}

	polls := a.collector.PollCount()
	if err := a.sender.SendCounter("PollCount", polls-a.reportedPolls); err != nil {
		log.Println(err)
		return
	}
	a.reportedPolls = polls
}
