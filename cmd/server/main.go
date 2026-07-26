package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/Vortex-art-01/mertic/internal/handler/counter"
	"github.com/Vortex-art-01/mertic/internal/handler/gauge"
	"github.com/Vortex-art-01/mertic/internal/handler/index"
	"github.com/Vortex-art-01/mertic/internal/handler/unknowntype"
	"github.com/Vortex-art-01/mertic/internal/handler/value"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	repo := repository.NewMemStorage()

	return http.ListenAndServe(":8080", newRouter(repo))
}

func newRouter(repo repository.Repository) http.Handler {
	r := chi.NewRouter()

	r.Get("/", index.New(repo))
	r.Get("/value/{type}/{name}", value.New(repo))

	r.Post("/update/gauge/{name}/{value}", gauge.New(repo))
	r.Post("/update/counter/{name}/{value}", counter.New(repo))
	r.Post("/update/{type}/{name}/{value}", unknowntype.New())

	return r
}
