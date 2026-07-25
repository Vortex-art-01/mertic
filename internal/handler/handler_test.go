package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/repository"
)

func TestUpdateHandlers(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		target     string
		wantStatus int
	}{
		{"gauge ok", http.MethodPost, "/update/gauge/Alloc/123.45", http.StatusOK},
		{"gauge int value ok", http.MethodPost, "/update/gauge/Alloc/100", http.StatusOK},
		{"counter ok", http.MethodPost, "/update/counter/PollCount/527", http.StatusOK},
		{"counter negative ok", http.MethodPost, "/update/counter/PollCount/-5", http.StatusOK},
		{"unknown type", http.MethodPost, "/update/unknown/testCounter/100", http.StatusBadRequest},
		{"gauge bad value", http.MethodPost, "/update/gauge/Alloc/none", http.StatusBadRequest},
		{"counter bad value", http.MethodPost, "/update/counter/PollCount/1.5", http.StatusBadRequest},
		{"no metric name", http.MethodPost, "/update/counter/527", http.StatusNotFound},
		{"no name trailing slash", http.MethodPost, "/update/counter/", http.StatusNotFound},
		{"empty path", http.MethodPost, "/update/", http.StatusNotFound},
	}

	router := NewRouter(repository.NewMemStorage())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, nil)
			req.Header.Set("Content-Type", "text/plain")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("%s %s: got status %d, want %d", tt.method, tt.target, rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestCounterAccumulatesAndGaugeReplaces(t *testing.T) {
	repo := repository.NewMemStorage()
	router := NewRouter(repo)

	send := func(target string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, target, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST %s: got status %d, want %d", target, rec.Code, http.StatusOK)
		}
	}

	send("/update/counter/PollCount/10")
	send("/update/counter/PollCount/5")
	if got, ok := repo.GetCounter("PollCount"); !ok || got != 15 {
		t.Errorf("counter PollCount = %d (ok=%v), want 15", got, ok)
	}

	send("/update/gauge/Alloc/1.5")
	send("/update/gauge/Alloc/2.5")
	if got, ok := repo.GetGauge("Alloc"); !ok || got != 2.5 {
		t.Errorf("gauge Alloc = %v (ok=%v), want 2.5", got, ok)
	}
}
