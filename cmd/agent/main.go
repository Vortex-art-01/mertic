package main

import (
	"log"
	"time"

	"github.com/Vortex-art-01/mertic/internal/agent"
)

func main() {
	parseFlags()

	log.Println("Running server on", flagRunAddr)
	log.Println("ReportInterval is", flagReportInterval)
	log.Println("PollInterval is", flagPollInterval)
	client := agent.NewClient("http://" + flagRunAddr)
	a := agent.New(client,
		time.Duration(flagPollInterval)*time.Second,
		time.Duration(flagReportInterval)*time.Second,
	)
	a.Run()
}
