package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/model"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

func TestRouter(t *testing.T) {
	repo := repository.NewMemStorage()
	repo.SaveGauge("Alloc", 123.45)
	repo.AddCounter("PollCount", 5)

	ts := httptest.NewServer(newRouter(repo, slog.New(slog.DiscardHandler)))
	defer ts.Close()

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "get known gauge",
			method:         http.MethodGet,
			path:           "/value/gauge/Alloc",
			expectedStatus: http.StatusOK,
			expectedBody:   "123.45",
		},
		{
			name:           "get known counter",
			method:         http.MethodGet,
			path:           "/value/counter/PollCount",
			expectedStatus: http.StatusOK,
			expectedBody:   "5",
		},
		{
			name:           "get unknown metric",
			method:         http.MethodGet,
			path:           "/value/gauge/Missing",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "get unknown metric type",
			method:         http.MethodGet,
			path:           "/value/histogram/Alloc",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "update gauge",
			method:         http.MethodPost,
			path:           "/update/gauge/Alloc/321.5",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "update counter",
			method:         http.MethodPost,
			path:           "/update/counter/PollCount/3",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "update unknown type",
			method:         http.MethodPost,
			path:           "/update/histogram/Alloc/1",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "update without name and value",
			method:         http.MethodPost,
			path:           "/update/gauge",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(tt.method, ts.URL+tt.path, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			res, err := ts.Client().Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer res.Body.Close()

			if res.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, res.StatusCode)
			}

			if tt.expectedBody != "" {
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

	t.Run("json endpoints round trip", func(t *testing.T) {
		post := func(t *testing.T, path, body string) *http.Response {
			t.Helper()

			res, err := ts.Client().Post(ts.URL+path, "application/json", strings.NewReader(body))
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			return res
		}

		update := post(t, "/update", `{"id":"HeapSys","type":"gauge","value":42.5}`)
		defer update.Body.Close()

		if update.StatusCode != http.StatusOK {
			t.Fatalf("update status = %d, want %d", update.StatusCode, http.StatusOK)
		}
		if got := update.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("update Content-Type = %q, want %q", got, "application/json")
		}

		// Значение должно быть видно и через текстовый эндпоинт.
		res, err := ts.Client().Get(ts.URL + "/value/gauge/HeapSys")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if string(body) != "42.5" {
			t.Errorf("plain value = %q, want %q", body, "42.5")
		}

		value := post(t, "/value", `{"id":"HeapSys","type":"gauge"}`)
		defer value.Body.Close()

		if value.StatusCode != http.StatusOK {
			t.Fatalf("value status = %d, want %d", value.StatusCode, http.StatusOK)
		}

		var got model.Metrics
		if err := json.NewDecoder(value.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if got.Value == nil || *got.Value != 42.5 {
			t.Errorf("value = %v, want 42.5", got.Value)
		}

		t.Run("trailing slash", func(t *testing.T) {
			update := post(t, "/update/", `{"id":"PollCount","type":"counter","delta":2}`)
			defer update.Body.Close()

			if update.StatusCode != http.StatusOK {
				t.Errorf("update status = %d, want %d", update.StatusCode, http.StatusOK)
			}

			value := post(t, "/value/", `{"id":"PollCount","type":"counter"}`)
			defer value.Body.Close()

			if value.StatusCode != http.StatusOK {
				t.Errorf("value status = %d, want %d", value.StatusCode, http.StatusOK)
			}
		})
	})

	t.Run("index page lists metrics", func(t *testing.T) {
		res, err := ts.Client().Get(ts.URL + "/")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, res.StatusCode)
		}

		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}

		for _, want := range []string{"Alloc", "PollCount"} {
			if !strings.Contains(string(body), want) {
				t.Errorf("expected page to contain %q, got:\n%s", want, body)
			}
		}
	})
}
