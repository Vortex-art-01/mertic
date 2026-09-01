package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Vortex-art-01/mertic/internal/model"
)

// senderMock ведёт себя как сервер: раскладывает пакет по метрикам и
// складывает приращения счётчиков.
type senderMock struct {
	gauges   map[string]float64
	counters map[string]int64
	batches  int
	err      error
}

func newSenderMock() *senderMock {
	return &senderMock{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (m *senderMock) SendBatch(_ context.Context, metrics []model.Metrics) error {
	if m.err != nil {
		return m.err
	}

	m.batches++

	for _, metric := range metrics {
		switch metric.MType {
		case model.Gauge:
			m.gauges[metric.ID] = *metric.Value
		case model.Counter:
			m.counters[metric.ID] += *metric.Delta
		}
	}

	return nil
}

// newTestAgent возвращает агент вместе с буфером, куда он пишет лог.
func newTestAgent(sender MetricsSender) (*Agent, *bytes.Buffer) {
	var logs bytes.Buffer
	a := New(sender, time.Second, time.Second, slog.New(slog.NewJSONHandler(&logs, nil)))
	return a, &logs
}

func TestAgentReportSendsAllMetricsInOneBatch(t *testing.T) {
	sender := newSenderMock()
	a, logs := newTestAgent(sender)

	a.collector.Poll()
	a.collector.Poll()
	a.Report(t.Context())

	if sender.batches != 1 {
		t.Errorf("sent %d batches, want 1", sender.batches)
	}
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
	a.Report(t.Context())
	a.collector.Poll()
	a.collector.Poll()
	a.Report(t.Context())

	// Сервер суммирует приращения: 1 + 2 = 3.
	if got := sender.counters["PollCount"]; got != 3 {
		t.Errorf("accumulated PollCount = %d, want 3", got)
	}
}

func TestAgentReportRetriesPollCountAfterError(t *testing.T) {
	sender := newSenderMock()
	a, logs := newTestAgent(sender)

	a.collector.Poll()
	sender.err = errors.New("server unavailable")
	a.Report(t.Context())

	sender.err = nil
	a.collector.Poll()
	a.Report(t.Context())

	// Первая отправка не удалась, поэтому второй отчёт
	// должен включать оба неотправленных приращения.
	if got := sender.counters["PollCount"]; got != 2 {
		t.Errorf("accumulated PollCount = %d, want 2", got)
	}

	// Отправка не прерывает работу агента, поэтому единственный след
	// неудачи — запись в логе.
	for _, want := range []string{"failed to send metrics", "server unavailable"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log must contain %q, got:\n%s", want, logs)
		}
	}
}

// До первого опроса отправлять нечего — пустой пакет до сервера не доходит.
func TestAgentReportSkipsEmptyBatch(t *testing.T) {
	sender := newSenderMock()
	a, logs := newTestAgent(sender)

	a.Report(t.Context())

	if sender.batches != 0 {
		t.Errorf("sent %d batches, want none", sender.batches)
	}
	if logs.Len() != 0 {
		t.Errorf("skipped report must not log, got:\n%s", logs)
	}
}

// Отмена контекста останавливает агент: Run возвращается сам.
func TestAgentRunStopsOnCanceledContext(t *testing.T) {
	a, _ := newTestAgent(newSenderMock())

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan struct{})
	go func() {
		defer close(done)
		a.Run(ctx)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Run did not return after the context was canceled")
	}
}
