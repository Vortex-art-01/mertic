// Package updatesjson принимает метрики пакетами: POST /updates/ с телом
// []Metrics. Одиночный POST /update никуда не делся — пакетная отправка
// просто экономит запросы, когда метрик много.
package updatesjson

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/Vortex-art-01/mertic/internal/audit"
	"github.com/Vortex-art-01/mertic/internal/httpbody"
	"github.com/Vortex-art-01/mertic/internal/model"
)

const (
	// batchCapacity — запас в слайсе под пакет: агент присылает несколько десятков
	// метрик, и с таким запасом разбор JSON обходится без наращивания слайса.
	batchCapacity = 64

	// maxPooledBatch ограничивает слайсы, которые возвращаются в пул: из-за одного
	// огромного пакета память не должна оставаться занятой.
	maxPooledBatch = 1024
)

// okResponse — ответ на принятый пакет.
var okResponse = []byte(`{"status":"ok"}`)

// batches — пул слайсов под пакеты метрик: сервер разбирает их постоянно.
var batches = sync.Pool{New: func() any {
	batch := make([]model.Metrics, 0, batchCapacity)
	return &batch
}}

// putBatch обнуляет элементы слайса: JSON заполняет существующие элементы на месте,
// и поля прошлого запроса иначе достались бы следующему, а записанное число попало
// бы по старому указателю.
func putBatch(batch *[]model.Metrics) {
	if cap(*batch) > maxPooledBatch {
		return
	}

	clear((*batch)[:cap(*batch)])
	*batch = (*batch)[:0]

	batches.Put(batch)
}

// MetricsStorage сохраняет весь пакет разом: и хранилище в памяти, и база
// применяют его целиком, поэтому частично принятых пакетов не бывает. Слайс после
// возврата из SaveBatch использовать нельзя: он вернётся в пул.
type MetricsStorage interface {
	SaveBatch(ctx context.Context, metrics []model.Metrics) error
}

// Auditor узнаёт о принятых метриках; кому он передаст событие, хендлеру неважно.
// Enabled позволяет не собирать событие, когда его некому отправить.
type Auditor interface {
	Enabled() bool
	Notify(event audit.Event)
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

// New возвращает хендлер POST /updates/. Тело запроса — JSON-массив
// [model.Metrics], и к каждой метрике те же требования, что у POST /update/.
// Пакет проверяется целиком до записи и сохраняется одним вызовом
// [MetricsStorage.SaveBatch], поэтому принимается либо весь, либо никак. Пустой
// массив — не ошибка: хендлер ответит 200, ничего не записав.
//
// Ответы:
//   - 200 с телом {"status":"ok"} — пакет сохранён;
//   - 400 — тело не JSON-массив или в пакете есть метрика неизвестного типа
//     либо без значения;
//   - 404 — в пакете есть метрика с пустым именем;
//   - 500 — сбой хранилища, подробности уходят в лог l.
//
// О принятом пакете хендлер сообщает auditor одним событием со всеми именами.
func New(storage MetricsStorage, auditor Auditor, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		batch := batches.Get().(*[]model.Metrics)
		defer putBatch(batch)

		if err := httpbody.DecodeJSON(r.Body, batch); err != nil {
			reject(l, w, http.StatusBadRequest, "invalid json body", slog.Any("error", err))
			return
		}

		metrics := *batch

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
		_, _ = w.Write(okResponse)

		// В пустом пакете не было метрик — значит, и сообщать аудиту не о чем. Имена
		// собираются, только если есть кому их отправить: по умолчанию аудит выключен.
		if len(metrics) > 0 && auditor.Enabled() {
			auditor.Notify(audit.NewEvent(r, metricNames(metrics)...))
		}
	}
}

func metricNames(metrics []model.Metrics) []string {
	names := make([]string, len(metrics))
	for i, m := range metrics {
		names[i] = m.ID
	}

	return names
}
