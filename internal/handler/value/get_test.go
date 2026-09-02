package value

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestNew(t *testing.T) {
	getter := &mockValueGetter{
		gauges:   map[string]float64{"test_gauge": 10.5},
		counters: map[string]int64{"test_counter": 100},
	}

	tests := []struct {
		name           string
		metricType     string
		metricName     string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "known gauge",
			metricType:     "gauge",
			metricName:     "test_gauge",
			expectedStatus: http.StatusOK,
			expectedBody:   "10.5",
		},
		{
			name:           "known counter",
			metricType:     "counter",
			metricName:     "test_counter",
			expectedStatus: http.StatusOK,
			expectedBody:   "100",
		},
		{
			name:           "unknown gauge",
			metricType:     "gauge",
			metricName:     "missing",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unknown counter",
			metricType:     "counter",
			metricName:     "missing",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "unknown metric type",
			metricType:     "histogram",
			metricName:     "test_gauge",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := New(getter, slog.New(slog.DiscardHandler))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.SetPathValue("type", tt.metricType)
			req.SetPathValue("name", tt.metricName)

			w := httptest.NewRecorder()
			handler(w, req)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, res.StatusCode)
			}

			if tt.expectedStatus == http.StatusOK {
				body, err := io.ReadAll(res.Body)
				if err != nil {
					t.Fatalf("failed to read body: %v", err)
				}
				if string(body) != tt.expectedBody {
					t.Errorf("expected body %q, got %q", tt.expectedBody, string(body))
				}
			}
		})
	}
}

// Недоступное хранилище — это 500, а не 404: метрика может существовать,
// просто её не удалось прочитать.
func TestNewReportsStorageFailure(t *testing.T) {
	getter := &mockValueGetter{err: errors.New("storage is down")}
	handler := New(getter, slog.New(slog.DiscardHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("type", "gauge")
	req.SetPathValue("name", "test_gauge")

	w := httptest.NewRecorder()
	handler(w, req)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, res.StatusCode)
	}
}
