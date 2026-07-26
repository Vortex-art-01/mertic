package main

import (
	"log"
	"net/http"

	"github.com/Vortex-art-01/mertic/internal/handler/counter"
	"github.com/Vortex-art-01/mertic/internal/handler/gauge"
	"github.com/Vortex-art-01/mertic/internal/handler/unknowntype"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	repo := repository.NewMemStorage()

	mux := http.NewServeMux()

	mux.HandleFunc("POST /update/gauge/{name}/{value}", gauge.New(repo))
	mux.HandleFunc("POST /update/counter/{name}/{value}", counter.New(repo))
	mux.HandleFunc("POST /update/{type}/{name}/{value}", unknowntype.New())

	return http.ListenAndServe(":8080", mux)
}
