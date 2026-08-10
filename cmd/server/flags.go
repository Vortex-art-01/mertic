package main

import (
	"flag"
	"os"
)

var (
	flagRunAddr string
)

func parseFlags() {
	flag.StringVar(&flagRunAddr, "a", "localhost:8080", "адрес эндпоинта HTTP-сервера")

	flag.Parse()

	if env := os.Getenv("ADDRESS"); env != "" {
		flagRunAddr = env
	}
}
