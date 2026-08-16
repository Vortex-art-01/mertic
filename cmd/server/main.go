package main

import (
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"

	"github.com/Vortex-art-01/mertic/internal/handler/counter"
	"github.com/Vortex-art-01/mertic/internal/handler/gauge"
	"github.com/Vortex-art-01/mertic/internal/handler/index"
	"github.com/Vortex-art-01/mertic/internal/handler/unknowntype"
	"github.com/Vortex-art-01/mertic/internal/handler/updatejson"
	"github.com/Vortex-art-01/mertic/internal/handler/value"
	"github.com/Vortex-art-01/mertic/internal/handler/valuejson"
	"github.com/Vortex-art-01/mertic/internal/logger"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

func main() {
	parseFlags()

	if err := run(flagRunAddr, logger.New(os.Stdout)); err != nil {
		log.Fatal(err)
	}
}

func run(addr string, l *slog.Logger) error {
	repo := repository.NewMemStorage()

	l.Info("running server", slog.String("address", addr))
	return http.ListenAndServe(addr, newRouter(repo, l))
}

func newRouter(repo *repository.MemStorage, l *slog.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(logger.WithLogging(l))

	updateJSON := updatejson.New(repo, l)
	valueJSON := valuejson.New(repo, l)

	r.Get("/", index.New(repo, l))
	r.Get("/value/{type}/{name}", value.New(repo))

	// Варианты с завершающим слешем регистрируются явно:
	// chi не сопоставляет "/update/" с маршрутом "/update".
	r.Post("/update", updateJSON)
	r.Post("/update/", updateJSON)
	r.Post("/value", valueJSON)
	r.Post("/value/", valueJSON)

	r.Post("/update/gauge/{name}/{value}", gauge.New(repo))
	r.Post("/update/counter/{name}/{value}", counter.New(repo))
	r.Post("/update/{type}/{name}/{value}", unknowntype.New())

	return r
}
