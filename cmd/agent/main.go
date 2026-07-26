package main

import (
	"time"

	"github.com/Vortex-art-01/mertic/internal/agent"
)

const (
	serverAddr     = "http://localhost:8080"
	pollInterval   = 2 * time.Second
	reportInterval = 10 * time.Second
)

func main() {
	client := agent.NewClient(serverAddr)
	a := agent.New(client, pollInterval, reportInterval)
	a.Run()
}
