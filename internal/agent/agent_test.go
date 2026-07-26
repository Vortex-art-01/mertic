package agent

import (
	"errors"
	"testing"
	"time"
)

type senderMock struct {
	gauges     map[string]float64
	counters   map[string]int64
	counterErr error
}

func newSenderMock() *senderMock {
	return &senderMock{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (m *senderMock) SendGauge(name string, value float64) error {
	m.gauges[name] = value
	return nil
}

func (m *senderMock) SendCounter(name string, delta int64) error {
	if m.counterErr != nil {
		return m.counterErr
	}
	m.counters[name] += delta
	return nil
}

func TestAgentReportSendsAllMetrics(t *testing.T) {
	sender := newSenderMock()
	a := New(sender, time.Second, time.Second)

	a.collector.Poll()
	a.collector.Poll()
	a.Report()

	for _, name := range wantGauges {
		if _, ok := sender.gauges[name]; !ok {
			t.Errorf("gauge %q not sent", name)
		}
	}
	if got := sender.counters["PollCount"]; got != 2 {
		t.Errorf("PollCount = %d, want 2", got)
	}
}

func TestAgentReportSendsPollCountDelta(t *testing.T) {
	sender := newSenderMock()
	a := New(sender, time.Second, time.Second)

	a.collector.Poll()
	a.Report()
	a.collector.Poll()
	a.collector.Poll()
	a.Report()

	// Сервер суммирует приращения: 1 + 2 = 3.
	if got := sender.counters["PollCount"]; got != 3 {
		t.Errorf("accumulated PollCount = %d, want 3", got)
	}
}

func TestAgentReportRetriesPollCountAfterError(t *testing.T) {
	sender := newSenderMock()
	a := New(sender, time.Second, time.Second)

	a.collector.Poll()
	sender.counterErr = errors.New("server unavailable")
	a.Report()

	sender.counterErr = nil
	a.collector.Poll()
	a.Report()

	// Первая отправка не удалась, поэтому второй отчёт
	// должен включать оба неотправленных приращения.
	if got := sender.counters["PollCount"]; got != 2 {
		t.Errorf("accumulated PollCount = %d, want 2", got)
	}
}
