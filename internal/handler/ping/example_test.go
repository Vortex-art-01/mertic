package ping_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"

	"github.com/Vortex-art-01/mertic/internal/handler/ping"
)

// unavailableDB изображает базу, до которой не достучаться.
type unavailableDB struct{}

func (unavailableDB) Ping(context.Context) error {
	return errors.New("connection refused")
}

// Без базы метрики хранятся в памяти, проверять нечего, и ответ всегда 200. С
// базой ответ зависит от того, отвечает ли она.
func ExampleNew() {
	l := slog.New(slog.DiscardHandler)

	inMemory := chi.NewRouter()
	inMemory.Get("/ping", ping.New(nil, l))

	withDatabase := chi.NewRouter()
	withDatabase.Get("/ping", ping.New(unavailableDB{}, l))

	for _, router := range []http.Handler{inMemory, withDatabase} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))

		fmt.Println(rec.Code)
	}

	// Output:
	// 200
	// 500
}
