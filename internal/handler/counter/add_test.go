package counter

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockCounterSaver struct {
	savedName  string
	savedValue int64
}

func (m *mockCounterSaver) AddCounter(name string, value int64) {
	m.savedName = name
	m.savedValue += value
}

func (m *mockCounterSaver) GetCounter(name string) (int64, bool) {
	if name == m.savedName {
		return m.savedValue, true
	}
	return 0, false
}

func TestNew(t *testing.T) {
	tests := []struct {
		name           string
		metricName     string
		metricValue    string
		expectedStatus int
		expectedSaved  int64
	}{
		{
			name:           "valid counter",
			metricName:     "test_counter",
			metricValue:    "100",
			expectedStatus: http.StatusOK,
			expectedSaved:  100,
		},
		{
			name:           "empty metric name",
			metricName:     "",
			metricValue:    "100",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid metric value",
			metricName:     "test_counter",
			metricValue:    "invalid",
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			saver := &mockCounterSaver{}
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
					t.Errorf("expected value %d, got %d", tt.expectedSaved, saver.savedValue)
				}
			}
		})
	}
}
