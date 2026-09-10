package main

import (
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Vortex-art-01/mertic/internal/database"
	"github.com/Vortex-art-01/mertic/internal/dump"
	"github.com/Vortex-art-01/mertic/internal/handler/counter"
	"github.com/Vortex-art-01/mertic/internal/handler/gauge"
	"github.com/Vortex-art-01/mertic/internal/handler/index"
	"github.com/Vortex-art-01/mertic/internal/handler/ping"
	"github.com/Vortex-art-01/mertic/internal/handler/unknowntype"
	"github.com/Vortex-art-01/mertic/internal/handler/updatejson"
	"github.com/Vortex-art-01/mertic/internal/handler/updatesjson"
	"github.com/Vortex-art-01/mertic/internal/handler/value"
	"github.com/Vortex-art-01/mertic/internal/handler/valuejson"
	"github.com/Vortex-art-01/mertic/internal/logger"
	"github.com/Vortex-art-01/mertic/internal/middleware"
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

	var (
		storage metricsStorage
		pinger  ping.Pinger
		dumps   io.Closer
	)

	switch {
	case cfg.databaseDSN != "":
		pool, err := database.New(ctx, cfg.databaseDSN)
		if err != nil {
			return err
		}
		defer pool.Close()

		l.Info("database schema is up to date")

		storage, pinger = repository.NewPostgres(pool), pool
	default:
		storage, dumps = dump.Attach(ctx, repository.NewMemStorage(), dump.Config{
			Path:     cfg.fileStorage,
			Interval: cfg.storeInterval,
			Restore:  cfg.restore,
		}, l)
	}

	srv := &http.Server{Addr: cfg.runAddr, Handler: newRouter(routerDeps{
		storage: storage,
		pinger:  pinger,
		key:     cfg.key,
		log:     l,
	})}

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
		slog.String("storage", storageKind(cfg)),
		slog.String("file", cfg.fileStorage),
		slog.Duration("store interval", cfg.storeInterval),
		slog.Bool("restore", cfg.restore),
		slog.Bool("signed", cfg.key != ""))

	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	<-shutdownDone

	if dumps != nil {
		if err := dumps.Close(); err != nil {
			return err
		}
	}

	l.Info("server stopped")

	return nil
}

func storageKind(cfg config) string {
	switch {
	case cfg.databaseDSN != "":
		return "database"
	case cfg.fileStorage != "":
		return "file"
	default:
		return "memory"
	}
}

type routerDeps struct {
	storage metricsStorage
	pinger  ping.Pinger
	key     string
	log     *slog.Logger
}

func newRouter(deps routerDeps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.WithLogging(deps.log))
	r.Use(middleware.WithGzip)
	r.Use(middleware.WithHash(deps.key))

	updateJSON := updatejson.New(deps.storage, deps.log)
	updatesJSON := updatesjson.New(deps.storage, deps.log)
	valueJSON := valuejson.New(deps.storage, deps.log)

	r.Get("/", index.New(deps.storage, deps.log))
	r.Get("/ping", ping.New(deps.pinger, deps.log))
	r.Get("/value/{type}/{name}", value.New(deps.storage, deps.log))

	r.Post("/update", updateJSON)
	r.Post("/update/", updateJSON)
	r.Post("/updates", updatesJSON)
	r.Post("/updates/", updatesJSON)
	r.Post("/value", valueJSON)
	r.Post("/value/", valueJSON)

	r.Post("/update/gauge/{name}/{value}", gauge.New(deps.storage, deps.log))
	r.Post("/update/counter/{name}/{value}", counter.New(deps.storage, deps.log))
	r.Post("/update/{type}/{name}/{value}", unknowntype.New())

	return r
}
