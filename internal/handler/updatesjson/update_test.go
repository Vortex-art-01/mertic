package updatesjson

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/audit"
	"github.com/Vortex-art-01/mertic/internal/model"
)

type mockAuditor struct {
	events   []audit.Event
	disabled bool
}

func (m *mockAuditor) Enabled() bool { return !m.disabled }

func (m *mockAuditor) Notify(event audit.Event) {
	m.events = append(m.events, event)
}

// mockStorage повторяет поведение настоящих хранилищ: у gauge остаётся
// последнее значение, приращения counter складываются.
type mockStorage struct {
	gauges   map[string]float64
	counters map[string]int64
	calls    int
	err      error
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (m *mockStorage) SaveBatch(_ context.Context, metrics []model.Metrics) error {
	if m.err != nil {
		return m.err
	}

	m.calls++

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

// do выполняет запрос и возвращает ответ вместе с тем, что хендлер написал в лог.
func do(t *testing.T, storage MetricsStorage, body string) (*http.Response, string) {
	t.Helper()

	var logs bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&logs, nil))

	req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	New(storage, &mockAuditor{}, l)(w, req)

	return w.Result(), logs.String()
}

func TestNewSavesWholeBatch(t *testing.T) {
	storage := newMockStorage()

	res, logs := do(t, storage, `[
		{"id":"Alloc","type":"gauge","value":42.5},
		{"id":"PollCount","type":"counter","delta":3}
	]`)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if got := res.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}

	// Пакет уходит в хранилище целиком, одним вызовом.
	if storage.calls != 1 {
		t.Errorf("storage called %d times, want 1", storage.calls)
	}
	if got := storage.gauges["Alloc"]; got != 42.5 {
		t.Errorf("Alloc = %v, want 42.5", got)
	}
	if got := storage.counters["PollCount"]; got != 3 {
		t.Errorf("PollCount = %d, want 3", got)
	}
	if logs != "" {
		t.Errorf("successful update must not log, got:\n%s", logs)
	}
}

// Один и тот же счётчик может прийти в пакете несколько раз — приращения
// должны сложиться, как если бы метрики пришли по одной.
func TestNewAccumulatesRepeatedCounter(t *testing.T) {
	storage := newMockStorage()

	res, _ := do(t, storage, `[
		{"id":"PollCount","type":"counter","delta":2},
		{"id":"PollCount","type":"counter","delta":5}
	]`)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if got := storage.counters["PollCount"]; got != 7 {
		t.Errorf("PollCount = %d, want 7", got)
	}
}

// Пустой пакет — не ошибка, но и хранилище тревожить незачем.
func TestNewAcceptsEmptyBatch(t *testing.T) {
	storage := newMockStorage()

	res, _ := do(t, storage, `[]`)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if storage.calls != 0 {
		t.Errorf("storage called %d times, want none", storage.calls)
	}
}

func TestNewRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"invalid json", `{`, http.StatusBadRequest},
		{"garbage after the batch", `[{"id":"Alloc","type":"gauge","value":1}] oops`, http.StatusBadRequest},
		{"single metric instead of a list", `{"id":"Alloc","type":"gauge","value":1}`, http.StatusBadRequest},
		{"missing name", `[{"id":"","type":"gauge","value":1}]`, http.StatusNotFound},
		{"unknown type", `[{"id":"Alloc","type":"histogram","value":1}]`, http.StatusBadRequest},
		{"gauge without value", `[{"id":"Alloc","type":"gauge"}]`, http.StatusBadRequest},
		{"counter without delta", `[{"id":"PollCount","type":"counter"}]`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := newMockStorage()

			res, logs := do(t, storage, tt.body)
			defer res.Body.Close()

			if res.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}
			if storage.calls != 0 {
				t.Errorf("rejected batch must not reach storage, got %d calls", storage.calls)
			}
			if !strings.Contains(logs, "batch update rejected") {
				t.Errorf("log must explain the refusal, got:\n%s", logs)
			}
		})
	}
}

// Половина пакета — это не пакет: одна негодная метрика отменяет весь запрос.
func TestNewRejectsWholeBatchOnSingleInvalidMetric(t *testing.T) {
	storage := newMockStorage()

	res, _ := do(t, storage, `[
		{"id":"Alloc","type":"gauge","value":42.5},
		{"id":"Broken","type":"histogram","value":1}
	]`)
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusBadRequest)
	}
	if storage.calls != 0 {
		t.Errorf("nothing must be saved, got %d storage calls", storage.calls)
	}
}

// Слайсы под пакеты переиспользуются, а JSON заполняет существующие элементы на месте.
// Если не обнулять их, gauge без значения прошёл бы проверку со значением из прошлого
// запроса, а новое число записалось бы по указателю, который хранит чужой пакет.
func TestNewDoesNotLeakFieldsBetweenRequests(t *testing.T) {
	storage := newMockStorage()

	for range 20 {
		res, _ := do(t, storage, `[
			{"id":"Alloc","type":"gauge","value":42.5},
			{"id":"PollCount","type":"counter","delta":3}
		]`)
		res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Fatalf("valid batch: status = %d, want %d", res.StatusCode, http.StatusOK)
		}

		res, _ = do(t, storage, `[{"id":"NoValue","type":"gauge"}]`)
		res.Body.Close()

		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("gauge without value: status = %d, want %d", res.StatusCode, http.StatusBadRequest)
		}

		res, _ = do(t, storage, `[{"id":"NoDelta","type":"counter"}]`)
		res.Body.Close()

		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("counter without delta: status = %d, want %d", res.StatusCode, http.StatusBadRequest)
		}
	}

	if _, ok := storage.gauges["NoValue"]; ok {
		t.Error("gauge without value reached the storage")
	}
	if got := storage.counters["PollCount"]; got != 60 {
		t.Errorf("PollCount = %d, want 60", got)
	}
}

// Пакет больше типичного разбирается как обычно, но его слайс в пул не возвращается:
// из-за одного огромного пакета память не должна оставаться занятой.
func TestNewHandlesBatchLargerThanPooled(t *testing.T) {
	var body strings.Builder

	body.WriteByte('[')
	for i := range maxPooledBatch + 1 {
		if i > 0 {
			body.WriteByte(',')
		}
		fmt.Fprintf(&body, `{"id":"Metric%d","type":"gauge","value":%d}`, i, i)
	}
	body.WriteByte(']')

	storage := newMockStorage()

	res, _ := do(t, storage, body.String())
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if len(storage.gauges) != maxPooledBatch+1 {
		t.Errorf("saved %d gauges, want %d", len(storage.gauges), maxPooledBatch+1)
	}

	for range 100 {
		batch := batches.Get().(*[]model.Metrics)
		if cap(*batch) > maxPooledBatch {
			t.Fatalf("slice with capacity %d came back from the pool", cap(*batch))
		}
		defer batches.Put(batch)
	}
}

func TestNewReportsStorageFailure(t *testing.T) {
	storage := newMockStorage()
	storage.err = errors.New("database is down")

	res, logs := do(t, storage, `[{"id":"Alloc","type":"gauge","value":1}]`)
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusInternalServerError)
	}
	// Подробности сбоя остаются в логе, клиенту их знать незачем.
	for _, want := range []string{"batch update failed", "database is down"} {
		if !strings.Contains(logs, want) {
			t.Errorf("log must contain %q, got:\n%s", want, logs)
		}
	}
}

// С выключенным аудитом событие не собирается и никому не отправляется.
func TestNewSkipsEventWhenAuditIsDisabled(t *testing.T) {
	auditor := &mockAuditor{disabled: true}

	req := httptest.NewRequest(http.MethodPost, "/updates/",
		strings.NewReader(`[{"id":"Alloc","type":"gauge","value":1}]`))
	w := httptest.NewRecorder()

	New(newMockStorage(), auditor, slog.New(slog.DiscardHandler))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if len(auditor.events) != 0 {
		t.Errorf("events = %+v, want none", auditor.events)
	}
}

// На весь пакет — одно событие со всеми именами. Отклонённый пакет и пустой
// в аудит не попадают: в первом ничего не принято, во втором нечего принимать.
func TestNewNotifiesAuditor(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		storageErr  error
		wantMetrics []string
	}{
		{
			name: "whole batch",
			body: `[
				{"id":"Alloc","type":"gauge","value":42.5},
				{"id":"PollCount","type":"counter","delta":3}
			]`,
			wantMetrics: []string{"Alloc", "PollCount"},
		},
		{name: "empty batch", body: `[]`},
		{name: "invalid json", body: `{`},
		{
			name: "batch with invalid metric",
			body: `[
				{"id":"Alloc","type":"gauge","value":42.5},
				{"id":"Broken","type":"histogram","value":1}
			]`,
		},
		{
			name:       "storage failure",
			body:       `[{"id":"Alloc","type":"gauge","value":1}]`,
			storageErr: errors.New("database is down"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := newMockStorage()
			storage.err = tt.storageErr

			auditor := &mockAuditor{}

			req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(tt.body))
			req.RemoteAddr = "192.168.0.42:5555"

			New(storage, auditor, slog.New(slog.DiscardHandler))(httptest.NewRecorder(), req)

			if tt.wantMetrics == nil {
				if len(auditor.events) != 0 {
					t.Errorf("events = %+v, want none", auditor.events)
				}
				return
			}

			if len(auditor.events) != 1 {
				t.Fatalf("got %d events, want 1", len(auditor.events))
			}

			event := auditor.events[0]
			if !slices.Equal(event.Metrics, tt.wantMetrics) {
				t.Errorf("metrics = %v, want %v", event.Metrics, tt.wantMetrics)
			}
			if event.IPAddress != "192.168.0.42" {
				t.Errorf("ip_address = %q, want %q", event.IPAddress, "192.168.0.42")
			}
		})
	}
}
