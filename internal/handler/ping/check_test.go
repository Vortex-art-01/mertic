package ping

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockPinger struct {
	err error
}

func (m mockPinger) PingContext(ctx context.Context) error {
	return m.err
}

func TestNew(t *testing.T) {
	tests := []struct {
		name           string
		pinger         Pinger
		expectedStatus int
	}{
		{
			name:           "database is available",
			pinger:         mockPinger{},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "database is unavailable",
			pinger:         mockPinger{err: errors.New("connection refused")},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name:           "database is not configured",
			pinger:         nil,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := New(tt.pinger, slog.New(slog.DiscardHandler))

			req := httptest.NewRequest(http.MethodGet, "/ping", nil)
			w := httptest.NewRecorder()
			handler(w, req)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, res.StatusCode)
			}
		})
	}
}

// Ограничение по времени задаётся хендлером: без него зависшая база держала
// бы запрос до собственных таймаутов драйвера.
func TestNewLimitsPingTime(t *testing.T) {
	var (
		called   bool
		deadline bool
	)

	pinger := funcPinger(func(ctx context.Context) error {
		called = true
		_, deadline = ctx.Deadline()
		return ctx.Err()
	})

	w := httptest.NewRecorder()
	New(pinger, slog.New(slog.DiscardHandler))(w, httptest.NewRequest(http.MethodGet, "/ping", nil))

	if !called {
		t.Fatal("pinger was not called")
	}
	if !deadline {
		t.Error("ping context has no deadline")
	}
	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Result().StatusCode)
	}
}

type funcPinger func(ctx context.Context) error

func (f funcPinger) PingContext(ctx context.Context) error {
	return f(ctx)
}
