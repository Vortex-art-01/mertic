package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Vortex-art-01/mertic/internal/agent"
	"github.com/Vortex-art-01/mertic/internal/buildinfo"
	"github.com/Vortex-art-01/mertic/internal/logger"
)

// Сведения о сборке подставляет линкер флагом -X, например
// -ldflags "-X main.buildVersion=v1.0.0". Без флага значение остаётся пустым и
// при старте печатается как N/A.
var (
	buildVersion string
	buildDate    string
	buildCommit  string
)

func main() {
	// Печатаем до разбора флагов: по этим строкам видно, какая сборка
	// запущена, даже если дальше старт сорвётся из-за конфигурации.
	fmt.Println(buildinfo.Info{Version: buildVersion, Date: buildDate, Commit: buildCommit})

	if err := parseFlags(); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
