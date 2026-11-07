// Package valuejson отдаёт значение метрики в JSON: POST /value/ с телом
// [model.Metrics].
package valuejson

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Vortex-art-01/mertic/internal/httpbody"
	"github.com/Vortex-art-01/mertic/internal/model"
)

// ValueGetter читает текущее значение метрики. Второй результат сообщает, есть
// ли метрика с таким именем: её отсутствие — не ошибка.
type ValueGetter interface {
	GetGauge(ctx context.Context, name string) (float64, bool, error)
	GetCounter(ctx context.Context, name string) (int64, bool, error)
}

// reject отвечает клиенту и пишет причину отказа в лог: middleware логирует
// только код ответа, а у /value один URI на все метрики — по нему причину
// не восстановить.
func reject(l *slog.Logger, w http.ResponseWriter, code int, reason string, attrs ...any) {
	l.Warn("value request rejected", append([]any{slog.String("reason", reason)}, attrs...)...)
	http.Error(w, reason, code)
}

// New возвращает хендлер POST /value/. В теле запроса — [model.Metrics] с
// именем и типом, в ответе — та же метрика с текущим значением:
//
//	запрос: {"id":"Alloc","type":"gauge"}
//	ответ:  {"id":"Alloc","type":"gauge","value":1024.5}
//
// Ответы:
//   - 200 с метрикой в JSON — метрика найдена;
//   - 400 — тело не JSON;
//   - 404 — тип неизвестен или метрики с таким именем нет;
//   - 500 — сбой хранилища, подробности уходят в лог l.
func New(getter ValueGetter, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var m model.Metrics
		if err := httpbody.DecodeJSON(r.Body, &m); err != nil {
			reject(l, w, http.StatusBadRequest, "invalid json body", slog.Any("error", err))
			return
		}

		var (
			found bool
			err   error
		)

		switch m.MType {
		case model.Gauge:
			var v float64
			v, found, err = getter.GetGauge(r.Context(), m.ID)
			m.Value, m.Delta = &v, nil
		case model.Counter:
			var v int64
			v, found, err = getter.GetCounter(r.Context(), m.ID)
			m.Delta, m.Value = &v, nil
		default:
			reject(l, w, http.StatusNotFound, "unknown metric type",
				slog.String("metric", m.ID), slog.String("type", m.MType))
			return
		}

		if err != nil {
			l.Error("value: failed to read metric",
				slog.String("metric", m.ID), slog.String("type", m.MType), slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		if !found {
			reject(l, w, http.StatusNotFound, "metric not found",
				slog.String("metric", m.ID), slog.String("type", m.MType))
			return
		}

		buf := httpbody.GetBuffer()
		defer httpbody.PutBuffer(buf)

		if err := json.NewEncoder(buf).Encode(m); err != nil {
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
