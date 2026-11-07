package index

import (
	"context"
	"errors"
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
	err      error
}

func (m *mockMetricsLister) Gauges(_ context.Context) (map[string]float64, error) {
	return m.gauges, m.err
}

func (m *mockMetricsLister) Counters(_ context.Context) (map[string]int64, error) {
	return m.counters, m.err
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

// Страница собирается вручную, поэтому её разметка зафиксирована целиком: сначала
// gauge, потом counter, внутри типа — по алфавиту.
func TestNewRendersWholePage(t *testing.T) {
	lister := &mockMetricsLister{
		gauges:   map[string]float64{"HeapAlloc": 1024, "Alloc": 123.45, "Tiny": 0.000001},
		counters: map[string]int64{"PollCount": 7, "Errors": -1},
	}

	w := httptest.NewRecorder()
	New(lister, slog.New(slog.DiscardHandler))(w, httptest.NewRequest(http.MethodGet, "/", nil))

	const want = `<!DOCTYPE html>
<html lang="ru">
<head>
	<meta charset="utf-8">
	<title>Metrics</title>
</head>
<body>
	<h1>Metrics</h1>
	<ul>
		<li>Alloc = 123.45</li>
		<li>HeapAlloc = 1024</li>
		<li>Tiny = 0.000001</li>
		<li>Errors = -1</li>
		<li>PollCount = 7</li>
	</ul>
</body>
</html>
`

	if got := w.Body.String(); got != want {
		t.Errorf("page:\n%s\nwant:\n%s", got, want)
	}
}

func TestNewRendersEmptyPage(t *testing.T) {
	w := httptest.NewRecorder()
	New(&mockMetricsLister{}, slog.New(slog.DiscardHandler))(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if strings.Contains(w.Body.String(), "<li>") {
		t.Errorf("empty storage must render an empty list, got:\n%s", w.Body.String())
	}
}

// Имя метрики задаёт клиент: без экранирования любой мог бы встроить в страницу скрипт.
func TestNewEscapesMetricNames(t *testing.T) {
	lister := &mockMetricsLister{
		gauges:   map[string]float64{`<script>alert("gauge")</script>`: 1},
		counters: map[string]int64{`<img src=x onerror='counter'>&`: 2},
	}

	w := httptest.NewRecorder()
	New(lister, slog.New(slog.DiscardHandler))(w, httptest.NewRequest(http.MethodGet, "/", nil))

	body := w.Body.String()

	for _, unsafe := range []string{"<script", "<img", `"gauge"`, "'counter'"} {
		if strings.Contains(body, unsafe) {
			t.Errorf("page contains unescaped %q:\n%s", unsafe, body)
		}
	}

	for _, want := range []string{"&lt;script&gt;", "&lt;img", "&amp; = 2"} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %q:\n%s", want, body)
		}
	}
}

// Неполный список — хуже отсутствия списка: если хранилище не ответило,
// страница не должна показывать пустоту как правду.
func TestNewReportsStorageFailure(t *testing.T) {
	lister := &mockMetricsLister{err: errors.New("storage is down")}
	handler := New(lister, slog.New(slog.DiscardHandler))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	res := w.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, res.StatusCode)
	}
}
