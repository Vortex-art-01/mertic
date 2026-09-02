package ping

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

const timeout = 3 * time.Second

type Pinger interface {
	PingContext(ctx context.Context) error
}

func New(pinger Pinger, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if pinger == nil {
			w.WriteHeader(http.StatusOK)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		if err := pinger.PingContext(ctx); err != nil {
			l.Error("ping: database is unavailable", slog.Any("error", err))
			http.Error(w, "database is unavailable", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
