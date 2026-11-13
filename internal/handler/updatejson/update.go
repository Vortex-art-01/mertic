// Package updatejson обновляет одну метрику через JSON: POST /update/ с телом
// [model.Metrics].
package updatejson

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Vortex-art-01/mertic/internal/audit"
	"github.com/Vortex-art-01/mertic/internal/httpbody"
	"github.com/Vortex-art-01/mertic/internal/model"
)

// MetricsStorage сохраняет метрику и отдаёт её актуальное значение:
// для counter сервер накапливает приращения, поэтому в ответе
// возвращается сумма, а не пришедшая в запросе delta.
type MetricsStorage interface {
	SaveGauge(ctx context.Context, name string, value float64) error
	AddCounter(ctx context.Context, name string, value int64) error
	GetGauge(ctx context.Context, name string) (float64, bool, error)
	GetCounter(ctx context.Context, name string) (int64, bool, error)
}

// Auditor узнаёт о принятых метриках; кому он передаст событие, хендлеру неважно.
// Enabled позволяет не собирать событие, когда его некому отправить.
type Auditor interface {
	Enabled() bool
	Notify(event audit.Event)
}

// reject отвечает клиенту и пишет причину отказа в лог: middleware логирует
// только код ответа, а у /update один URI на все метрики — по нему причину
// не восстановить.
func reject(l *slog.Logger, w http.ResponseWriter, code int, reason string, attrs ...any) {
	l.Warn("update rejected", append([]any{slog.String("reason", reason)}, attrs...)...)
	http.Error(w, reason, code)
}

// fail сообщает о сбое хранилища: клиенту — 500 без подробностей,
// в лог — саму ошибку.
func fail(l *slog.Logger, w http.ResponseWriter, m model.Metrics, err error) {
	l.Error("update failed",
		slog.String("metric", m.ID), slog.String("type", m.MType), slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// New возвращает хендлер POST /update/. Тело запроса — [model.Metrics]: у gauge
// обязательно поле value, у counter — delta. В ответ приходит та же метрика с
// актуальным значением; для counter это накопленная сумма, а не присланная
// delta. Например, если счётчик Requests равен 5:
//
//	запрос: {"id":"Requests","type":"counter","delta":3}
//	ответ:  {"id":"Requests","type":"counter","delta":8}
//
// Ответы:
//   - 200 с метрикой в JSON — метрика сохранена;
//   - 400 — тело не JSON, тип неизвестен или нет значения;
//   - 404 — имя пустое;
//   - 413 — тело больше предела, который ставит middleware.WithBodyLimit;
//   - 500 — сбой хранилища, подробности уходят в лог l.
//
// Причину отказа хендлер тоже пишет в лог: у всех метрик один URI, и по записи о
// запросе её не восстановить. О каждой принятой метрике хендлер сообщает auditor,
// если аудит включён.
func New(storage MetricsStorage, auditor Auditor, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var m model.Metrics
		if err := httpbody.DecodeJSON(r.Body, &m); err != nil {
			code, reason := httpbody.ErrorStatus(err)
			reject(l, w, code, reason, slog.Any("error", err))
			return
		}

		if m.ID == "" {
			reject(l, w, http.StatusNotFound, "metric name is required")
			return
		}

		ctx := r.Context()

		switch m.MType {
		case model.Gauge:
			if m.Value == nil {
				reject(l, w, http.StatusBadRequest, "gauge value is required", slog.String("metric", m.ID))
				return
			}
			if err := storage.SaveGauge(ctx, m.ID, *m.Value); err != nil {
				fail(l, w, m, err)
				return
			}

			saved, _, err := storage.GetGauge(ctx, m.ID)
			if err != nil {
				fail(l, w, m, err)
				return
			}
			m.Value, m.Delta = &saved, nil
		case model.Counter:
			if m.Delta == nil {
				reject(l, w, http.StatusBadRequest, "counter delta is required", slog.String("metric", m.ID))
				return
			}
			if err := storage.AddCounter(ctx, m.ID, *m.Delta); err != nil {
				fail(l, w, m, err)
				return
			}

			saved, _, err := storage.GetCounter(ctx, m.ID)
			if err != nil {
				fail(l, w, m, err)
				return
			}
			m.Delta, m.Value = &saved, nil
		default:
			reject(l, w, http.StatusBadRequest, "unknown metric type",
				slog.String("metric", m.ID), slog.String("type", m.MType))
			return
		}

		buf := httpbody.GetBuffer()
		defer httpbody.PutBuffer(buf)

		if err := json.NewEncoder(buf).Encode(m); err != nil {
			l.Error("update: failed to encode response",
				slog.String("metric", m.ID), slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = buf.WriteTo(w)

		// Событие собирается, только если есть кому его отправить: по умолчанию
		// аудит выключен.
		if auditor.Enabled() {
			auditor.Notify(audit.NewEvent(r, m.ID))
		}
	}
}
