package updatesjson_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Vortex-art-01/mertic/internal/audit"
	"github.com/Vortex-art-01/mertic/internal/handler/updatesjson"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

// Пакет метрик приходит одним запросом. Повторы имени внутри пакета
// обрабатываются как отдельные запросы: приращения PollCount складываются.
func ExampleNew() {
	ctx := context.Background()
	l := slog.New(slog.DiscardHandler)
	storage := repository.NewMemStorage()

	auditor := audit.NewSubject(l) // без приёмников аудит выключен
	defer auditor.Close()

	router := chi.NewRouter()
	router.Post("/updates/", updatesjson.New(storage, auditor, l))

	body := `[
		{"id":"Alloc","type":"gauge","value":1024},
		{"id":"PollCount","type":"counter","delta":1},
		{"id":"PollCount","type":"counter","delta":2}
	]`

	req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	fmt.Println(rec.Code, rec.Header().Get("Content-Type"))
	fmt.Println(rec.Body.String())

	alloc, _, _ := storage.GetGauge(ctx, "Alloc")
	pollCount, _, _ := storage.GetCounter(ctx, "PollCount")
	fmt.Println("Alloc =", alloc)
	fmt.Println("PollCount =", pollCount)

	// Output:
	// 200 application/json
	// {"status":"ok"}
	// Alloc = 1024
	// PollCount = 3
}

// Пакет проверяется целиком до записи: одна некорректная метрика отклоняет весь
// пакет, и в хранилище не попадают даже правильные метрики.
func ExampleNew_rejected() {
	l := slog.New(slog.DiscardHandler)
	storage := repository.NewMemStorage()

	auditor := audit.NewSubject(l)
	defer auditor.Close()

	router := chi.NewRouter()
	router.Post("/updates/", updatesjson.New(storage, auditor, l))

	body := `[
		{"id":"Alloc","type":"gauge","value":1024},
		{"id":"PollCount","type":"counter"}
	]`

	req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	fmt.Printf("%d %s", rec.Code, rec.Body.String())

	_, saved, _ := storage.GetGauge(context.Background(), "Alloc")
	fmt.Println("Alloc saved:", saved)

	// Output:
	// 400 counter delta is required
	// Alloc saved: false
}
