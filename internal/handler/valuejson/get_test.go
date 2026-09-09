package valuejson

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/model"
)

type mockValueGetter struct {
	gauges   map[string]float64
	counters map[string]int64
	err      error
}

func (m *mockValueGetter) GetGauge(_ context.Context, name string) (float64, bool, error) {
	if m.err != nil {
		return 0, false, m.err
	}
	v, ok := m.gauges[name]
	return v, ok, nil
}

func (m *mockValueGetter) GetCounter(_ context.Context, name string) (int64, bool, error) {
	if m.err != nil {
		return 0, false, m.err
	}
	v, ok := m.counters[name]
	return v, ok, nil
}

// do выполняет запрос и возвращает ответ вместе с тем, что хендлер написал в лог.
func do(t *testing.T, getter ValueGetter, body string) (*http.Response, string) {
	t.Helper()

	var logs bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&logs, nil))

	req := httptest.NewRequest(http.MethodPost, "/value", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	New(getter, l)(w, req)

	return w.Result(), logs.String()
}

func newMockValueGetter() *mockValueGetter {
	return &mockValueGetter{
		gauges:   map[string]float64{"Alloc": 10.5},
		counters: map[string]int64{"PollCount": 100},
	}
}

func TestNewReturnsGauge(t *testing.T) {
	res, _ := do(t, newMockValueGetter(), `{"id":"Alloc","type":"gauge"}`)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if got := res.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}

	var got model.Metrics
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.ID != "Alloc" || got.MType != model.Gauge {
		t.Errorf("response = %+v, want id Alloc of type gauge", got)
	}
	if got.Value == nil || *got.Value != 10.5 {
		t.Errorf("response value = %v, want 10.5", got.Value)
	}
	if got.Delta != nil {
		t.Errorf("response delta = %v, want nil for gauge", *got.Delta)
	}
}

func TestNewReturnsCounter(t *testing.T) {
	res, _ := do(t, newMockValueGetter(), `{"id":"PollCount","type":"counter"}`)
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}

	var got model.Metrics
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Delta == nil || *got.Delta != 100 {
		t.Errorf("response delta = %v, want 100", got.Delta)
	}
	if got.Value != nil {
		t.Errorf("response value = %v, want nil for counter", *got.Value)
	}
}

// Значение из запроса игнорируется: сервер отдаёт то, что хранит.
func TestNewIgnoresRequestValue(t *testing.T) {
	res, _ := do(t, newMockValueGetter(), `{"id":"Alloc","type":"gauge","value":777}`)
	defer res.Body.Close()

	var got model.Metrics
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Value == nil || *got.Value != 10.5 {
		t.Errorf("response value = %v, want 10.5", got.Value)
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
			name:           "unknown gauge",
			body:           `{"id":"Missing","type":"gauge"}`,
			expectedStatus: http.StatusNotFound,
			expectedReason: "metric not found",
		},
		{
			name:           "unknown counter",
			body:           `{"id":"Missing","type":"counter"}`,
			expectedStatus: http.StatusNotFound,
			expectedReason: "metric not found",
		},
		{
			name:           "unknown metric type",
			body:           `{"id":"Alloc","type":"histogram"}`,
			expectedStatus: http.StatusNotFound,
			expectedReason: "unknown metric type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, logs := do(t, newMockValueGetter(), tt.body)
			defer res.Body.Close()

			if res.StatusCode != tt.expectedStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.expectedStatus)
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
	res, logs := do(t, newMockValueGetter(), `{"id":"Alloc","type":"gauge"}`)
	defer res.Body.Close()

	if logs != "" {
		t.Errorf("successful request must not log, got:\n%s", logs)
	}
}

// Недоступное хранилище — это 500, а не 404: метрика может существовать,
// просто её не удалось прочитать.
func TestNewReportsStorageFailure(t *testing.T) {
	getter := newMockValueGetter()
	getter.err = errors.New("storage is down")

	res, logs := do(t, getter, `{"id":"Alloc","type":"gauge"}`)
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusInternalServerError)
	}
	if !strings.Contains(logs, "storage is down") {
		t.Errorf("storage error not logged, got:\n%s", logs)
	}
}
