package gauge

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockGaugeSaver struct {
	savedName  string
	savedValue float64
}

func (m *mockGaugeSaver) SaveGauge(name string, value float64) {
	m.savedName = name
	m.savedValue = value
}

func TestNew(t *testing.T) {
	tests := []struct {
		name           string
		metricName     string
		metricValue    string
		expectedStatus int
		expectedSaved  float64
	}{
		{
			name:           "valid gauge",
			metricName:     "test_gauge",
			metricValue:    "10.5",
			expectedStatus: http.StatusOK,
			expectedSaved:  10.5,
		},
		{
			name:           "empty metric name",
			metricName:     "",
			metricValue:    "10.5",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid metric value",
			metricName:     "test_gauge",
			metricValue:    "invalid",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saver := &mockGaugeSaver{}
			handler := New(saver)

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.SetPathValue("name", tt.metricName)
			req.SetPathValue("value", tt.metricValue)

			w := httptest.NewRecorder()
			handler(w, req)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, res.StatusCode)
			}

			if tt.expectedStatus == http.StatusOK {
				if saver.savedName != tt.metricName {
					t.Errorf("expected name %s, got %s", tt.metricName, saver.savedName)
				}
				if saver.savedValue != tt.expectedSaved {
					t.Errorf("expected value %f, got %f", tt.expectedSaved, saver.savedValue)
				}
			}
		})
	}
}
