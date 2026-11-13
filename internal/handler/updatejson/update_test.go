package updatejson

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

type mockStorage struct {
	gauges   map[string]float64
	counters map[string]int64
	err      error
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		gauges:   make(map[string]float64),
		counters: make(map[string]int64),
	}
}

func (m *mockStorage) SaveGauge(_ context.Context, name string, value float64) error {
	if m.err != nil {
		return m.err
	}
	m.gauges[name] = value
	return nil
}

func (m *mockStorage) AddCounter(_ context.Context, name string, value int64) error {
	if m.err != nil {
		return m.err
	}
	m.counters[name] += value
	return nil
}

func (m *mockStorage) GetGauge(_ context.Context, name string) (float64, bool, error) {
	if m.err != nil {
		return 0, false, m.err
	}
	v, ok := m.gauges[name]
	return v, ok, nil
}

func (m *mockStorage) GetCounter(_ context.Context, name string) (int64, bool, error) {
	if m.err != nil {
		return 0, false, m.err
	}
	v, ok := m.counters[name]
	return v, ok, nil
}

// do выполняет запрос и возвращает ответ вместе с тем, что хендлер написал в лог.
func do(t *testing.T, storage MetricsStorage, body string) (*http.Response, string) {
	t.Helper()

	var logs bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&logs, nil))

	req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	New(storage, &mockAuditor{}, l)(w, req)

	return w.Result(), logs.String()
}

func TestNewSavesGauge(t *testing.T) {
	storage := newMockStorage()

	res, _ := do(t, storage, `{"id":"Alloc","type":"gauge","value":123.45}`)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if got := res.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	if storage.gauges["Alloc"] != 123.45 {
		t.Errorf("stored gauge = %v, want 123.45", storage.gauges["Alloc"])
	}

	var got model.Metrics
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.ID != "Alloc" || got.MType != model.Gauge {
		t.Errorf("response = %+v, want id Alloc of type gauge", got)
	}
	if got.Value == nil || *got.Value != 123.45 {
		t.Errorf("response value = %v, want 123.45", got.Value)
	}
	if got.Delta != nil {
		t.Errorf("response delta = %v, want nil for gauge", *got.Delta)
	}
}

// Сервер накапливает counter, поэтому в ответе должна быть сумма приращений.
func TestNewAccumulatesCounter(t *testing.T) {
	storage := newMockStorage()
	storage.counters["PollCount"] = 5

	res, _ := do(t, storage, `{"id":"PollCount","type":"counter","delta":3}`)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if storage.counters["PollCount"] != 8 {
		t.Errorf("stored counter = %d, want 8", storage.counters["PollCount"])
	}

	var got model.Metrics
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Delta == nil || *got.Delta != 8 {
		t.Errorf("response delta = %v, want 8", got.Delta)
	}
	if got.Value != nil {
		t.Errorf("response value = %v, want nil for counter", *got.Value)
	}
}

func TestNewRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		expectedStatus int
		expectedReason string
	}{
		{
			name:           "malformed json",
			body:           `{"id":"Alloc","type":`,
			expectedStatus: http.StatusBadRequest,
			expectedReason: "invalid json body",
		},
		{
			name:           "empty name",
			body:           `{"id":"","type":"gauge","value":1}`,
			expectedStatus: http.StatusNotFound,
			expectedReason: "metric name is required",
		},
		{
			name:           "unknown type",
			body:           `{"id":"Alloc","type":"histogram","value":1}`,
			expectedStatus: http.StatusBadRequest,
			expectedReason: "unknown metric type",
		},
		{
			name:           "gauge without value",
			body:           `{"id":"Alloc","type":"gauge"}`,
			expectedStatus: http.StatusBadRequest,
			expectedReason: "gauge value is required",
		},
		{
			name:           "counter without delta",
			body:           `{"id":"PollCount","type":"counter"}`,
			expectedStatus: http.StatusBadRequest,
			expectedReason: "counter delta is required",
		},
		{
			name:           "gauge value of wrong type",
			body:           `{"id":"Alloc","type":"gauge","value":"123.45"}`,
			expectedStatus: http.StatusBadRequest,
			expectedReason: "invalid json body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := newMockStorage()

			res, logs := do(t, storage, tt.body)
			defer res.Body.Close()

			if res.StatusCode != tt.expectedStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.expectedStatus)
			}
			if len(storage.gauges) != 0 || len(storage.counters) != 0 {
				t.Errorf("storage must stay empty, got gauges %v, counters %v", storage.gauges, storage.counters)
			}

			// Код ответа пишет middleware, а причину — сам хендлер:
			// без неё по логу не понять, чем именно плох запрос.
			if !strings.Contains(logs, tt.expectedReason) {
				t.Errorf("reason %q not logged, got:\n%s", tt.expectedReason, logs)
			}
		})
	}
}

func TestNewDoesNotLogSuccess(t *testing.T) {
	res, logs := do(t, newMockStorage(), `{"id":"Alloc","type":"gauge","value":1}`)
	defer res.Body.Close()

	if logs != "" {
		t.Errorf("successful update must not log, got:\n%s", logs)
	}
}

// Нулевое значение должно сохраняться: Delta и Value объявлены указателями,
// чтобы отличать "0" от незаданного значения.
func TestNewSavesZeroValues(t *testing.T) {
	storage := newMockStorage()

	res, _ := do(t, storage, `{"id":"Alloc","type":"gauge","value":0}`)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if _, ok := storage.gauges["Alloc"]; !ok {
		t.Error("gauge with zero value was not stored")
	}
}

// Метрика, которую не удалось сохранить, не должна выглядеть принятой.
func TestNewReportsStorageFailure(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "gauge", body: `{"id":"Alloc","type":"gauge","value":1}`},
		{name: "counter", body: `{"id":"PollCount","type":"counter","delta":1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := newMockStorage()
			storage.err = errors.New("storage is down")

			res, logs := do(t, storage, tt.body)
			defer res.Body.Close()

			if res.StatusCode != http.StatusInternalServerError {
				t.Errorf("status = %d, want %d", res.StatusCode, http.StatusInternalServerError)
			}
			if !strings.Contains(logs, "storage is down") {
				t.Errorf("storage error not logged, got:\n%s", logs)
			}
		})
	}
}

// С выключенным аудитом событие не собирается и никому не отправляется.
func TestNewSkipsEventWhenAuditIsDisabled(t *testing.T) {
	storage := newMockStorage()
	auditor := &mockAuditor{disabled: true}

	req := httptest.NewRequest(http.MethodPost, "/update",
		strings.NewReader(`{"id":"Alloc","type":"gauge","value":1}`))
	w := httptest.NewRecorder()

	New(storage, auditor, slog.New(slog.DiscardHandler))(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if _, ok := storage.gauges["Alloc"]; !ok {
		t.Error("gauge was not stored")
	}
	if len(auditor.events) != 0 {
		t.Errorf("events = %+v, want none", auditor.events)
	}
}

// В аудит попадают только принятые метрики — с именем и адресом клиента.
func TestNewNotifiesAuditor(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		storageErr  error
		wantMetrics []string
	}{
		{
			name:        "gauge",
			body:        `{"id":"Alloc","type":"gauge","value":1}`,
			wantMetrics: []string{"Alloc"},
		},
		{
			name:        "counter",
			body:        `{"id":"PollCount","type":"counter","delta":1}`,
			wantMetrics: []string{"PollCount"},
		},
		{name: "malformed json", body: `{"id":"Alloc","type":`},
		{name: "empty name", body: `{"id":"","type":"gauge","value":1}`},
		{name: "unknown type", body: `{"id":"Alloc","type":"histogram","value":1}`},
		{name: "gauge without value", body: `{"id":"Alloc","type":"gauge"}`},
		{
			name:       "storage failure",
			body:       `{"id":"Alloc","type":"gauge","value":1}`,
			storageErr: errors.New("storage is down"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := newMockStorage()
			storage.err = tt.storageErr

			auditor := &mockAuditor{}

			req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(tt.body))
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
