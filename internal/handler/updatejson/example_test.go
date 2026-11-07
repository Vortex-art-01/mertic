package updatejson_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Vortex-art-01/mertic/internal/audit"
	"github.com/Vortex-art-01/mertic/internal/handler/updatejson"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

// В ответ на обновление приходит метрика с актуальным значением. Для counter это
// накопленная сумма: счётчик Requests был равен 5, пришла delta 3 — в ответе 8.
func ExampleNew() {
	l := slog.New(slog.DiscardHandler)

	storage := repository.NewMemStorage()
	_ = storage.AddCounter(context.Background(), "Requests", 5)

	auditor := audit.NewSubject(l) // без приёмников аудит выключен
	defer auditor.Close()

	router := chi.NewRouter()
	router.Post("/update/", updatejson.New(storage, auditor, l))

	for _, body := range []string{
		`{"id":"Temperature","type":"gauge","value":36.6}`,
		`{"id":"Requests","type":"counter","delta":3}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/update/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		fmt.Println(rec.Code, rec.Header().Get("Content-Type"))
		fmt.Print(rec.Body.String())
	}

	// Output:
	// 200 application/json
	// {"id":"Temperature","type":"gauge","value":36.6}
	// 200 application/json
	// {"id":"Requests","type":"counter","delta":8}
}

// Метрику без имени, без значения или неизвестного типа хендлер отклоняет, не
// трогая хранилище. В теле ответа — причина отказа.
func ExampleNew_rejected() {
	l := slog.New(slog.DiscardHandler)

	auditor := audit.NewSubject(l)
	defer auditor.Close()

	router := chi.NewRouter()
	router.Post("/update/", updatejson.New(repository.NewMemStorage(), auditor, l))

	for _, body := range []string{
		`{"id":"Temperature","type":"gauge"}`,
		`{"type":"gauge","value":36.6}`,
		`{"id":"Latency","type":"histogram","value":1.5}`,
		`{"id":"Temperature",`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/update/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		fmt.Printf("%d %s", rec.Code, rec.Body.String())
	}

	// Output:
	// 400 gauge value is required
	// 404 metric name is required
	// 400 unknown metric type
	// 400 invalid json body
}
