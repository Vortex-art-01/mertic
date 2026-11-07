package counter_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"

	"github.com/Vortex-art-01/mertic/internal/audit"
	"github.com/Vortex-art-01/mertic/internal/handler/counter"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

// Хендлер ставится на маршрут с параметрами name и value, как в сервере.
// Приращения counter накапливаются: сервер хранит их сумму.
func ExampleNew() {
	l := slog.New(slog.DiscardHandler)
	storage := repository.NewMemStorage()

	auditor := audit.NewSubject(l) // без приёмников аудит выключен
	defer auditor.Close()

	router := chi.NewRouter()
	router.Post("/update/counter/{name}/{value}", counter.New(storage, auditor, l))

	for _, path := range []string{
		"/update/counter/Requests/5",
		"/update/counter/Requests/3",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))

		fmt.Println(rec.Code)
	}

	requests, _, _ := storage.GetCounter(context.Background(), "Requests")
	fmt.Println("Requests =", requests)

	// Output:
	// 200
	// 200
	// Requests = 8
}

// Приращение counter — целое число: дробное отклоняется с кодом 400.
func ExampleNew_invalidValue() {
	l := slog.New(slog.DiscardHandler)

	auditor := audit.NewSubject(l)
	defer auditor.Close()

	router := chi.NewRouter()
	router.Post("/update/counter/{name}/{value}", counter.New(repository.NewMemStorage(), auditor, l))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/update/counter/Requests/1.5", nil))

	fmt.Println(rec.Code)
	fmt.Print(rec.Body.String())

	// Output:
	// 400
	// invalid counter value
}
