package unknowntype_test

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"

	"github.com/Vortex-art-01/mertic/internal/audit"
	"github.com/Vortex-art-01/mertic/internal/handler/gauge"
	"github.com/Vortex-art-01/mertic/internal/handler/unknowntype"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

// Общий маршрут стоит рядом с маршрутами известных типов, как в сервере. Запрос
// с типом gauge уходит своему хендлеру, а с неизвестным типом — сюда.
func ExampleNew() {
	l := slog.New(slog.DiscardHandler)

	auditor := audit.NewSubject(l)
	defer auditor.Close()

	router := chi.NewRouter()
	router.Post("/update/gauge/{name}/{value}", gauge.New(repository.NewMemStorage(), auditor, l))
	router.Post("/update/{type}/{name}/{value}", unknowntype.New())

	for _, path := range []string{
		"/update/gauge/Latency/1.5",
		"/update/histogram/Latency/1.5",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))

		fmt.Println(path, rec.Code)
	}

	// Output:
	// /update/gauge/Latency/1.5 200
	// /update/histogram/Latency/1.5 400
}
