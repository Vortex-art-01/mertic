package counter

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
)

type CounterSaver interface {
	AddCounter(ctx context.Context, name string, value int64) error
}

func New(saver CounterSaver, l *slog.Logger) http.HandlerFunc {
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
	}
}
