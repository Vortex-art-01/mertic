package main

import (
	"flag"
	"log"
	"time"

	"github.com/Vortex-art-01/mertic/internal/agent"
)

func main() {
	addr := flag.String("a", "localhost:8080", "адрес эндпоинта HTTP-сервера")
	reportInterval := flag.Int64("r", 10, "частота отправки метрик на сервер, сек")
	pollInterval := flag.Int64("p", 2, "частота опроса метрик из пакета runtime, сек")
	flag.Parse()

	if args := flag.Args(); len(args) > 0 {
		log.Fatalf("неизвестные аргументы: %v", args)
	}

	client := agent.NewClient("http://" + *addr)
	a := agent.New(client,
		time.Duration(*pollInterval)*time.Second,
		time.Duration(*reportInterval)*time.Second,
	)
	a.Run()
}
