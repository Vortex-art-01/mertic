package value

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Vortex-art-01/mertic/internal/model"
)

type ValueGetter interface {
	GetGauge(ctx context.Context, name string) (float64, bool, error)
	GetCounter(ctx context.Context, name string) (int64, bool, error)
}

func New(getter ValueGetter, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		mtype := r.PathValue("type")

		var (
			body  string
			found bool
			err   error
		)

		switch mtype {
		case model.Gauge:
			var v float64
			v, found, err = getter.GetGauge(r.Context(), name)
			body = strconv.FormatFloat(v, 'f', -1, 64)
		case model.Counter:
			var v int64
			v, found, err = getter.GetCounter(r.Context(), name)
			body = strconv.FormatInt(v, 10)
		default:
			http.Error(w, "unknown metric type", http.StatusNotFound)
			return
		}

		if err != nil {
			l.Error("failed to read metric",
				slog.String("metric", name), slog.String("type", mtype), slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		if !found {
			http.Error(w, "metric not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}
}
