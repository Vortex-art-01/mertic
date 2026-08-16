package index

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type mockMetricsLister struct {
	gauges   map[string]float64
	counters map[string]int64
}

func (m *mockMetricsLister) Gauges() map[string]float64 {
	return m.gauges
}

func (m *mockMetricsLister) Counters() map[string]int64 {
	return m.counters
}

func TestNew(t *testing.T) {
	lister := &mockMetricsLister{
		gauges:   map[string]float64{"Alloc": 123.45},
		counters: map[string]int64{"PollCount": 7},
	}

	handler := New(lister, slog.New(slog.DiscardHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, res.StatusCode)
	}

	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html content type, got %q", ct)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	for _, want := range []string{"Alloc = 123.45", "PollCount = 7"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("expected body to contain %q, got:\n%s", want, body)
		}
	}
}
