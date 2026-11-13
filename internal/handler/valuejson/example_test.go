package valuejson_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Vortex-art-01/mertic/internal/handler/valuejson"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

// В запросе — имя и тип метрики, в ответе — та же метрика с текущим значением:
// у gauge в поле value, у counter в поле delta.
func ExampleNew() {
	ctx := context.Background()

	storage := repository.NewMemStorage()
	_ = storage.SaveGauge(ctx, "Alloc", 1024.5)
	_ = storage.AddCounter(ctx, "PollCount", 5)

	router := chi.NewRouter()
	router.Post("/value/", valuejson.New(storage, slog.New(slog.DiscardHandler)))

	for _, body := range []string{
		`{"id":"Alloc","type":"gauge"}`,
		`{"id":"PollCount","type":"counter"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/value/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		fmt.Println(rec.Code, rec.Header().Get("Content-Type"))
		fmt.Print(rec.Body.String())
	}

	// Output:
	// 200 application/json
	// {"id":"Alloc","type":"gauge","value":1024.5}
	// 200 application/json
	// {"id":"PollCount","type":"counter","delta":5}
}

// Метрики с таким именем нет — ответ 404.
func ExampleNew_notFound() {
	router := chi.NewRouter()
	router.Post("/value/", valuejson.New(repository.NewMemStorage(), slog.New(slog.DiscardHandler)))

	req := httptest.NewRequest(http.MethodPost, "/value/", strings.NewReader(`{"id":"Missing","type":"gauge"}`))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	fmt.Printf("%d %s", rec.Code, rec.Body.String())

	// Output:
	// 404 metric not found
}
