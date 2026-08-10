package main

import (
	"flag"
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
	addr := flag.String("a", "localhost:8080", "адрес эндпоинта HTTP-сервера")
	flag.Parse()

	if args := flag.Args(); len(args) > 0 {
		log.Fatalf("неизвестные аргументы: %v", args)
	}

	if err := run(*addr); err != nil {
		log.Fatal(err)
	}
}

func run(addr string) error {
	repo := repository.NewMemStorage()

	return http.ListenAndServe(addr, newRouter(repo))
}

func newRouter(repo *repository.MemStorage) http.Handler {
	r := chi.NewRouter()

	r.Get("/", index.New(repo))
	r.Get("/value/{type}/{name}", value.New(repo))

	r.Post("/update/gauge/{name}/{value}", gauge.New(repo))
	r.Post("/update/counter/{name}/{value}", counter.New(repo))
	r.Post("/update/{type}/{name}/{value}", unknowntype.New())

	return r
}
