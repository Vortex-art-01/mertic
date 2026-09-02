package updatesjson

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/model"
)

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
	New(storage, l)(w, req)

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
