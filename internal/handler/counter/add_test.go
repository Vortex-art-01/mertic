package counter

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/audit"
)

type mockAuditor struct {
	events []audit.Event
}

func (m *mockAuditor) Notify(event audit.Event) {
	m.events = append(m.events, event)
}

type mockCounterSaver struct {
	savedName  string
	savedValue int64
	err        error
}

func (m *mockCounterSaver) AddCounter(_ context.Context, name string, value int64) error {
	if m.err != nil {
		return m.err
	}

	m.savedName = name
	m.savedValue += value

	return nil
}

func (m *mockCounterSaver) GetCounter(_ context.Context, name string) (int64, bool, error) {
	if name == m.savedName {
		return m.savedValue, true, nil
	}
	return 0, false, nil
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
			handler := New(saver, &mockAuditor{}, slog.New(slog.DiscardHandler))

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

// Сбой хранилища — не вина клиента: приращение потеряно, и об этом нужно
// сказать пятисотым, а не тихим 200.
func TestNewReportsStorageFailure(t *testing.T) {
	saver := &mockCounterSaver{err: errors.New("storage is down")}
	handler := New(saver, &mockAuditor{}, slog.New(slog.DiscardHandler))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.SetPathValue("name", "test_counter")
	req.SetPathValue("value", "100")

	w := httptest.NewRecorder()
	handler(w, req)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, res.StatusCode)
	}
}

// В аудит попадают только принятые метрики — с именем и адресом клиента.
func TestNewNotifiesAuditor(t *testing.T) {
	tests := []struct {
		name        string
		metricName  string
		metricValue string
		saveErr     error
		wantEvent   bool
	}{
		{name: "accepted metric", metricName: "test_counter", metricValue: "100", wantEvent: true},
		{name: "empty metric name", metricName: "", metricValue: "100"},
		{name: "invalid metric value", metricName: "test_counter", metricValue: "invalid"},
		{
			name:        "storage failure",
			metricName:  "test_counter",
			metricValue: "100",
			saveErr:     errors.New("storage is down"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auditor := &mockAuditor{}
			handler := New(&mockCounterSaver{err: tt.saveErr}, auditor, slog.New(slog.DiscardHandler))

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.RemoteAddr = "192.168.0.42:5555"
			req.SetPathValue("name", tt.metricName)
			req.SetPathValue("value", tt.metricValue)

			handler(httptest.NewRecorder(), req)

			if !tt.wantEvent {
				if len(auditor.events) != 0 {
					t.Errorf("events = %+v, want none", auditor.events)
				}
				return
			}

			if len(auditor.events) != 1 {
				t.Fatalf("got %d events, want 1", len(auditor.events))
			}

			event := auditor.events[0]
			if !slices.Equal(event.Metrics, []string{tt.metricName}) {
				t.Errorf("metrics = %v, want [%s]", event.Metrics, tt.metricName)
			}
			if event.IPAddress != "192.168.0.42" {
				t.Errorf("ip_address = %q, want %q", event.IPAddress, "192.168.0.42")
			}
		})
	}
}
