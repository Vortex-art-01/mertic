package valuejson

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Vortex-art-01/mertic/internal/model"
)

type ValueGetter interface {
	GetGauge(name string) (float64, bool)
	GetCounter(name string) (int64, bool)
}

// reject отвечает клиенту и пишет причину отказа в лог: middleware логирует
// только код ответа, а у /value один URI на все метрики — по нему причину
// не восстановить.
func reject(l *slog.Logger, w http.ResponseWriter, code int, reason string, attrs ...any) {
	l.Warn("value request rejected", append([]any{slog.String("reason", reason)}, attrs...)...)
	http.Error(w, reason, code)
}

func New(getter ValueGetter, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var m model.Metrics
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			reject(l, w, http.StatusBadRequest, "invalid json body", slog.Any("error", err))
			return
		}

		switch m.MType {
		case model.Gauge:
			v, ok := getter.GetGauge(m.ID)
			if !ok {
				reject(l, w, http.StatusNotFound, "metric not found",
					slog.String("metric", m.ID), slog.String("type", m.MType))
				return
			}
			m.Value, m.Delta = &v, nil
		case model.Counter:
			v, ok := getter.GetCounter(m.ID)
			if !ok {
				reject(l, w, http.StatusNotFound, "metric not found",
					slog.String("metric", m.ID), slog.String("type", m.MType))
				return
			}
			m.Delta, m.Value = &v, nil
		default:
			reject(l, w, http.StatusNotFound, "unknown metric type",
				slog.String("metric", m.ID), slog.String("type", m.MType))
			return
		}

		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(m); err != nil {
			l.Error("value: failed to encode response",
				slog.String("metric", m.ID), slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(w)
	}
}
