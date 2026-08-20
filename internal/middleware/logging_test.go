package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/logger"
)

func TestWithLogging(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		uri            string
		handler        http.HandlerFunc
		expectedStatus int
		expectedSize   int
	}{
		{
			name:   "explicit status and body",
			method: http.MethodGet,
			uri:    "/value/gauge/Alloc",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("123.45"))
			},
			expectedStatus: http.StatusOK,
			expectedSize:   6,
		},
		{
			name:   "body without explicit status",
			method: http.MethodGet,
			uri:    "/",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("ok"))
			},
			expectedStatus: http.StatusOK,
			expectedSize:   2,
		},
		{
			name:   "empty response",
			method: http.MethodPost,
			uri:    "/update/counter/PollCount/1",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			expectedStatus: http.StatusOK,
			expectedSize:   0,
		},
		{
			name:   "error status",
			method: http.MethodPost,
			uri:    "/update/histogram/Alloc/1",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "unknown metric type", http.StatusBadRequest)
			},
			expectedStatus: http.StatusBadRequest,
			expectedSize:   len("unknown metric type\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			handler := WithLogging(logger.New(&buf))(tt.handler)

			req := httptest.NewRequest(tt.method, tt.uri, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var record map[string]any
			if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
				t.Fatalf("failed to parse log record %q: %v", buf.String(), err)
			}

			for field, want := range map[string]any{
				"level":  "INFO",
				"uri":    tt.uri,
				"method": tt.method,
				"status": float64(tt.expectedStatus),
				"size":   float64(tt.expectedSize),
			} {
				got, ok := record[field]
				if !ok {
					t.Errorf("log record has no %q field: %s", field, buf.String())
					continue
				}
				if got != want {
					t.Errorf("expected %q to be %v, got %v", field, want, got)
				}
			}

			duration, ok := record["duration"].(float64)
			if !ok {
				t.Errorf("log record has no numeric duration field: %s", buf.String())
			} else if duration < 0 {
				t.Errorf("expected non-negative duration, got %v", duration)
			}
		})
	}
}
