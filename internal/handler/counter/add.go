package counter

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Vortex-art-01/mertic/internal/audit"
)

type CounterSaver interface {
	AddCounter(ctx context.Context, name string, value int64) error
}

// Auditor узнаёт о принятых метриках; кому он передаст событие, хендлеру неважно.
type Auditor interface {
	Notify(event audit.Event)
}

func New(saver CounterSaver, auditor Auditor, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			http.Error(w, "metric name is required", http.StatusNotFound)
			return
		}

		value, err := strconv.ParseInt(r.PathValue("value"), 10, 64)
		if err != nil {
			http.Error(w, "invalid counter value", http.StatusBadRequest)
			return
		}

		if err := saver.AddCounter(r.Context(), name, value); err != nil {
			l.Error("failed to add counter",
				slog.String("metric", name), slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		auditor.Notify(audit.NewEvent(r, name))
	}
}
