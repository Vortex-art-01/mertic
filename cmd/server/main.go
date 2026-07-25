package main

import (
	"log"
	"net/http"

	"github.com/Vortex-art-01/mertic/internal/handler"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	repo := repository.NewMemStorage()
	return http.ListenAndServe(":8080", handler.NewRouter(repo))
}
