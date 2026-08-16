package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/Vortex-art-01/mertic/internal/agent"
	"github.com/Vortex-art-01/mertic/internal/logger"
)

func main() {
	parseFlags()

	pollInterval := time.Duration(flagPollInterval) * time.Second
	reportInterval := time.Duration(flagReportInterval) * time.Second

	l := logger.New(os.Stdout)
	l.Info("running agent",
		slog.String("server_address", flagRunAddr),
		slog.String("poll_interval", pollInterval.String()),
		slog.String("report_interval", reportInterval.String()),
	)

	client := agent.NewClient("http://" + flagRunAddr)
	a := agent.New(client, pollInterval, reportInterval, l)
	a.Run()
}
