package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Vortex-art-01/mertic/internal/compress"
	"github.com/Vortex-art-01/mertic/internal/dump"
	"github.com/Vortex-art-01/mertic/internal/handler/counter"
	"github.com/Vortex-art-01/mertic/internal/handler/gauge"
	"github.com/Vortex-art-01/mertic/internal/handler/index"
	"github.com/Vortex-art-01/mertic/internal/handler/unknowntype"
	"github.com/Vortex-art-01/mertic/internal/handler/updatejson"
	"github.com/Vortex-art-01/mertic/internal/handler/value"
	"github.com/Vortex-art-01/mertic/internal/handler/valuejson"
	"github.com/Vortex-art-01/mertic/internal/logger"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

const shutdownTimeout = 5 * time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, parseFlags(), logger.New(os.Stdout)); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, cfg config, l *slog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	repo := repository.NewMemStorage()

	var storage metricsStorage = repo
	var d *dump.File

	if cfg.fileStorage != "" {
		d = dump.New(cfg.fileStorage, repo, cfg.restore, l)

		if cfg.storeInterval > 0 {
			go d.Run(ctx, cfg.storeInterval)
		} else {
			storage = &syncStorage{MemStorage: repo, dump: d, l: l}
		}
	}

	srv := &http.Server{Addr: cfg.runAddr, Handler: newRouter(storage, l)}

	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)

		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			l.Error("failed to shut down server gracefully", slog.Any("error", err))
		}
	}()

	l.Info("running server",
		slog.String("address", cfg.runAddr),
		slog.String("file", cfg.fileStorage),
		slog.Duration("store interval", cfg.storeInterval),
		slog.Bool("restore", cfg.restore))

	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	<-shutdownDone

	if d != nil {
		if err := d.Save(); err != nil {
			return err
		}
	}

	l.Info("server stopped")

	return nil
}

func newRouter(repo metricsStorage, l *slog.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(logger.WithLogging(l))
	r.Use(compress.WithGzip)

	updateJSON := updatejson.New(repo, l)
	valueJSON := valuejson.New(repo, l)

	r.Get("/", index.New(repo, l))
	r.Get("/value/{type}/{name}", value.New(repo))

	// Варианты с завершающим слешем регистрируются явно:
	// chi не сопоставляет "/update/" с маршрутом "/update".
	r.Post("/update", updateJSON)
	r.Post("/update/", updateJSON)
	r.Post("/value", valueJSON)
	r.Post("/value/", valueJSON)

	r.Post("/update/gauge/{name}/{value}", gauge.New(repo))
	r.Post("/update/counter/{name}/{value}", counter.New(repo))
	r.Post("/update/{type}/{name}/{value}", unknowntype.New())

	return r
}
