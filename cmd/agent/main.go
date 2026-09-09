package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Vortex-art-01/mertic/internal/agent"
	"github.com/Vortex-art-01/mertic/internal/logger"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	parseFlags()

	pollInterval := time.Duration(flagPollInterval) * time.Second
	reportInterval := time.Duration(flagReportInterval) * time.Second

	l := logger.New(os.Stdout)
	l.Info("running agent",
		slog.String("server_address", flagRunAddr),
		slog.String("poll_interval", pollInterval.String()),
		slog.String("report_interval", reportInterval.String()),
		slog.Int("rate_limit", flagRateLimit),
		slog.Bool("signed", flagKey != ""),
	)

	client := agent.NewClient("http://"+flagRunAddr, flagKey)
	a := agent.New(client, pollInterval, reportInterval, flagRateLimit, l)
	a.Run(ctx)

	l.Info("agent stopped")
}
