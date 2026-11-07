package value_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"

	"github.com/Vortex-art-01/mertic/internal/handler/value"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

// Значение метрики приходит текстом без лишних знаков: у gauge — десятичной
// записью, у counter — целым числом.
func ExampleNew() {
	ctx := context.Background()

	storage := repository.NewMemStorage()
	_ = storage.SaveGauge(ctx, "Alloc", 1024.5)
	_ = storage.AddCounter(ctx, "PollCount", 5)

	router := chi.NewRouter()
	router.Get("/value/{type}/{name}", value.New(storage, slog.New(slog.DiscardHandler)))

	for _, path := range []string{
		"/value/gauge/Alloc",
		"/value/counter/PollCount",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		fmt.Println(rec.Code, rec.Body.String())
	}

	// Output:
	// 200 1024.5
	// 200 5
}

// И неизвестная метрика, и неизвестный тип дают 404.
func ExampleNew_notFound() {
	storage := repository.NewMemStorage()
	_ = storage.SaveGauge(context.Background(), "Alloc", 1024.5)

	router := chi.NewRouter()
	router.Get("/value/{type}/{name}", value.New(storage, slog.New(slog.DiscardHandler)))

	for _, path := range []string{
		"/value/gauge/Missing",
		"/value/histogram/Alloc",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		fmt.Printf("%d %s", rec.Code, rec.Body.String())
	}

	// Output:
	// 404 metric not found
	// 404 unknown metric type
}
