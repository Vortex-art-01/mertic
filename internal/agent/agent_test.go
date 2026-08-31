package agent

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
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

// newTestAgent возвращает агент вместе с буфером, куда он пишет лог.
func newTestAgent(sender MetricsSender) (*Agent, *bytes.Buffer) {
	var logs bytes.Buffer
	a := New(sender, time.Second, time.Second, slog.New(slog.NewJSONHandler(&logs, nil)))
	return a, &logs
}

func TestAgentReportSendsAllMetrics(t *testing.T) {
	sender := newSenderMock()
	a, logs := newTestAgent(sender)

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
	if logs.Len() != 0 {
		t.Errorf("successful report must not log, got:\n%s", logs)
	}
}

func TestAgentReportSendsPollCountDelta(t *testing.T) {
	sender := newSenderMock()
	a, _ := newTestAgent(sender)

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
	a, logs := newTestAgent(sender)

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

	// Отправка не прерывает работу агента, поэтому единственный след
	// неудачи — запись в логе.
	for _, want := range []string{"failed to send counter", "PollCount", "server unavailable"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log must contain %q, got:\n%s", want, logs)
		}
	}
}
