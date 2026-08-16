package updatejson

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Vortex-art-01/mertic/internal/model"
)

// MetricsStorage сохраняет метрику и отдаёт её актуальное значение:
// для counter сервер накапливает приращения, поэтому в ответе
// возвращается сумма, а не пришедшая в запросе delta.
type MetricsStorage interface {
	SaveGauge(name string, value float64)
	AddCounter(name string, value int64)
	GetGauge(name string) (float64, bool)
	GetCounter(name string) (int64, bool)
}

// reject отвечает клиенту и пишет причину отказа в лог: middleware логирует
// только код ответа, а у /update один URI на все метрики — по нему причину
// не восстановить.
func reject(l *slog.Logger, w http.ResponseWriter, code int, reason string, attrs ...any) {
	l.Warn("update rejected", append([]any{slog.String("reason", reason)}, attrs...)...)
	http.Error(w, reason, code)
}

func New(storage MetricsStorage, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var m model.Metrics
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			reject(l, w, http.StatusBadRequest, "invalid json body", slog.Any("error", err))
			return
		}

		if m.ID == "" {
			reject(l, w, http.StatusNotFound, "metric name is required")
			return
		}

		switch m.MType {
		case model.Gauge:
			if m.Value == nil {
				reject(l, w, http.StatusBadRequest, "gauge value is required", slog.String("metric", m.ID))
				return
			}
			storage.SaveGauge(m.ID, *m.Value)

			saved, _ := storage.GetGauge(m.ID)
			m.Value, m.Delta = &saved, nil
		case model.Counter:
			if m.Delta == nil {
				reject(l, w, http.StatusBadRequest, "counter delta is required", slog.String("metric", m.ID))
				return
			}
			storage.AddCounter(m.ID, *m.Delta)

			saved, _ := storage.GetCounter(m.ID)
			m.Delta, m.Value = &saved, nil
		default:
			reject(l, w, http.StatusBadRequest, "unknown metric type",
				slog.String("metric", m.ID), slog.String("type", m.MType))
			return
		}

		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(m); err != nil {
			l.Error("update: failed to encode response",
				slog.String("metric", m.ID), slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(w)
	}
}
