// Package updatesjson принимает метрики пакетами: POST /updates/ с телом
// []Metrics. Одиночный POST /update никуда не делся — пакетная отправка
// просто экономит запросы, когда метрик много.
package updatesjson

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Vortex-art-01/mertic/internal/model"
)

// MetricsStorage сохраняет весь пакет разом: и хранилище в памяти, и база
// применяют его целиком, поэтому частично принятых пакетов не бывает.
type MetricsStorage interface {
	SaveBatch(ctx context.Context, metrics []model.Metrics) error
}

// reject отвечает клиенту и пишет причину отказа в лог: middleware логирует
// только код ответа, а у /updates/ один URI на все метрики — по нему причину
// не восстановить.
func reject(l *slog.Logger, w http.ResponseWriter, code int, reason string, attrs ...any) {
	l.Warn("batch update rejected", append([]any{slog.String("reason", reason)}, attrs...)...)
	http.Error(w, reason, code)
}

// validate повторяет проверки одиночного /update: пакет — это те же метрики,
// и требования к ним не меняются от способа доставки.
func validate(m model.Metrics) (int, string) {
	if m.ID == "" {
		return http.StatusNotFound, "metric name is required"
	}

	switch m.MType {
	case model.Gauge:
		if m.Value == nil {
			return http.StatusBadRequest, "gauge value is required"
		}
	case model.Counter:
		if m.Delta == nil {
			return http.StatusBadRequest, "counter delta is required"
		}
	default:
		return http.StatusBadRequest, "unknown metric type"
	}

	return 0, ""
}

func New(storage MetricsStorage, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var metrics []model.Metrics
		if err := json.NewDecoder(r.Body).Decode(&metrics); err != nil {
			reject(l, w, http.StatusBadRequest, "invalid json body", slog.Any("error", err))
			return
		}

		// Пакет проверяется целиком до записи: принять половину и отказать
		// на второй значило бы оставить хранилище в состоянии, о котором
		// клиент ничего не знает.
		for _, m := range metrics {
			if code, reason := validate(m); code != 0 {
				reject(l, w, code, reason,
					slog.String("metric", m.ID), slog.String("type", m.MType))
				return
			}
		}

		// Пустой пакет — не ошибка, но и хранилище тревожить незачем.
		if len(metrics) > 0 {
			if err := storage.SaveBatch(r.Context(), metrics); err != nil {
				l.Error("batch update failed",
					slog.Int("count", len(metrics)), slog.Any("error", err))
				http.Error(w, http.StatusText(http.StatusInternalServerError),
					http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}
}
