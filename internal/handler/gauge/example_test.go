package gauge_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"

	"github.com/Vortex-art-01/mertic/internal/audit"
	"github.com/Vortex-art-01/mertic/internal/handler/gauge"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

// Хендлер ставится на маршрут с параметрами name и value, как в сервере. Каждое
// новое значение gauge заменяет прежнее.
func ExampleNew() {
	l := slog.New(slog.DiscardHandler)
	storage := repository.NewMemStorage()

	auditor := audit.NewSubject(l) // без приёмников аудит выключен
	defer auditor.Close()

	router := chi.NewRouter()
	router.Post("/update/gauge/{name}/{value}", gauge.New(storage, auditor, l))

	for _, path := range []string{
		"/update/gauge/Temperature/36.6",
		"/update/gauge/Temperature/37.2",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))

		fmt.Println(rec.Code)
	}

	temperature, _, _ := storage.GetGauge(context.Background(), "Temperature")
	fmt.Println("Temperature =", temperature)

	// Output:
	// 200
	// 200
	// Temperature = 37.2
}

// Значение, которое не разбирается как число, отклоняется с кодом 400.
func ExampleNew_invalidValue() {
	l := slog.New(slog.DiscardHandler)

	auditor := audit.NewSubject(l)
	defer auditor.Close()

	router := chi.NewRouter()
	router.Post("/update/gauge/{name}/{value}", gauge.New(repository.NewMemStorage(), auditor, l))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/update/gauge/Temperature/hot", nil))

	fmt.Println(rec.Code)
	fmt.Print(rec.Body.String())

	// Output:
	// 400
	// invalid gauge value
}
