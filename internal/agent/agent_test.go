package agent

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Vortex-art-01/mertic/internal/model"
)

// senderMock ведёт себя как сервер: раскладывает пакет по метрикам и
// складывает приращения счётчиков.
type senderMock struct {
	mu       sync.Mutex
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
	m.mu.Lock()
	defer m.mu.Unlock()

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

func (m *senderMock) fail(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.err = err
}

// newTestAgent возвращает агент вместе с буфером, куда он пишет лог.
func newTestAgent(sender MetricsSender) (*Agent, *bytes.Buffer) {
	var logs bytes.Buffer
	a := New(sender, time.Second, time.Second, 1, slog.New(slog.NewJSONHandler(&logs, nil)))
	return a, &logs
}

// report повторяет один шаг отчётной горутины: снять накопленное и отправить.
func report(ctx context.Context, a *Agent) {
	a.send(ctx, a.collect())
}

func TestAgentReportSendsAllMetricsInOneBatch(t *testing.T) {
	sender := newSenderMock()
	a, logs := newTestAgent(sender)

	a.collector.PollRuntime()
	a.collector.PollRuntime()
	if err := a.collector.PollSystem(t.Context()); err != nil {
		t.Fatalf("PollSystem: %v", err)
	}
	report(t.Context(), a)

	if sender.batches != 1 {
		t.Errorf("sent %d batches, want 1", sender.batches)
	}
	for _, name := range slices.Concat(wantRuntimeGauges, wantSystemGauges) {
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

	a.collector.PollRuntime()
	report(t.Context(), a)
	a.collector.PollRuntime()
	a.collector.PollRuntime()
	report(t.Context(), a)

	// Сервер суммирует приращения: 1 + 2 = 3.
	if got := sender.counters["PollCount"]; got != 3 {
		t.Errorf("accumulated PollCount = %d, want 3", got)
	}
}

func TestAgentReportRetriesPollCountAfterError(t *testing.T) {
	sender := newSenderMock()
	a, logs := newTestAgent(sender)

	a.collector.PollRuntime()
	sender.fail(errors.New("server unavailable"))
	report(t.Context(), a)

	sender.fail(nil)
	a.collector.PollRuntime()
	report(t.Context(), a)

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

	report(t.Context(), a)

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

// blockingSender задерживает каждую отправку до закрытия release
// и отмечает, сколько отправок шло одновременно.
type blockingSender struct {
	started  chan struct{}
	release  chan struct{}
	inFlight atomic.Int64
	peak     atomic.Int64
}

func (s *blockingSender) SendBatch(ctx context.Context, _ []model.Metrics) error {
	inFlight := s.inFlight.Add(1)
	defer s.inFlight.Add(-1)

	for peak := s.peak.Load(); inFlight > peak; peak = s.peak.Load() {
		if s.peak.CompareAndSwap(peak, inFlight) {
			break
		}
	}

	// Сигнал теста не должен блокировать воркер: после того как тест
	// перестал читать, лишние отправки просто не отмечаются.
	select {
	case s.started <- struct{}{}:
	default:
	}

	select {
	case <-s.release:
	case <-ctx.Done():
	}

	return nil
}

// Пул из rateLimit воркеров не даёт уйти на сервер большему числу
// одновременных запросов, сколько бы отчётов ни накопилось.
func TestAgentRunLimitsConcurrentSends(t *testing.T) {
	const rateLimit = 2

	sender := &blockingSender{
		started: make(chan struct{}, 16),
		release: make(chan struct{}),
	}

	var logs bytes.Buffer
	a := New(sender, time.Millisecond, time.Millisecond, rateLimit,
		slog.New(slog.NewJSONHandler(&logs, nil)))

	ctx, cancel := context.WithCancel(t.Context())

	done := make(chan struct{})
	go func() {
		defer close(done)
		a.Run(ctx)
	}()

	// Отчёты идут чаще, чем воркеры их разбирают, поэтому пул занимает
	// ровно rateLimit отправок и дальше не растёт.
	for range rateLimit {
		select {
		case <-sender.started:
		case <-time.After(5 * time.Second):
			t.Fatal("worker pool did not reach the rate limit")
		}
	}

	select {
	case <-sender.started:
		t.Error("more sends started than the rate limit allows")
	case <-time.After(200 * time.Millisecond):
	}

	close(sender.release)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after the context was canceled")
	}

	if peak := sender.peak.Load(); peak > rateLimit {
		t.Errorf("peak concurrent sends = %d, want at most %d", peak, rateLimit)
	}
}

// Пул без воркеров некому разбирать: некорректный лимит поднимается до единицы.
func TestNewRaisesNonPositiveRateLimit(t *testing.T) {
	for _, rateLimit := range []int{-1, 0} {
		a := New(newSenderMock(), time.Second, time.Second, rateLimit, slog.Default())
		if a.rateLimit != 1 {
			t.Errorf("New(rateLimit=%d) kept %d, want 1", rateLimit, a.rateLimit)
		}
	}
}
