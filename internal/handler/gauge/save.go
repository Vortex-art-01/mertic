// Package gauge обновляет метрику типа gauge через URL:
// POST /update/gauge/{name}/{value}.
package gauge

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Vortex-art-01/mertic/internal/audit"
)

// GaugeSaver записывает значение gauge, заменяя прежнее.
type GaugeSaver interface {
	SaveGauge(ctx context.Context, name string, value float64) error
}

// Auditor узнаёт о принятых метриках; кому он передаст событие, хендлеру неважно.
// Enabled позволяет не собирать событие, когда его некому отправить.
type Auditor interface {
	Enabled() bool
	Notify(event audit.Event)
}

// New возвращает хендлер POST /update/gauge/{name}/{value}: он записывает value
// в метрику name. Имя и значение хендлер берёт из параметров пути name и value,
// поэтому маршрут должен их объявлять.
//
// Ответы:
//   - 200 без тела — значение сохранено;
//   - 400 — value не разбирается как float64;
//   - 404 — имя пустое;
//   - 500 — сбой хранилища, подробности уходят в лог l.
//
// О каждой принятой метрике хендлер сообщает auditor, если аудит включён.
func New(saver GaugeSaver, auditor Auditor, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			http.Error(w, "metric name is required", http.StatusNotFound)
			return
		}

		value, err := strconv.ParseFloat(r.PathValue("value"), 64)
		if err != nil {
			http.Error(w, "invalid gauge value", http.StatusBadRequest)
			return
		}

		if err := saver.SaveGauge(r.Context(), name, value); err != nil {
			l.Error("failed to save gauge",
				slog.String("metric", name), slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)

		// Событие собирается, только если есть кому его отправить: по умолчанию
		// аудит выключен.
		if auditor.Enabled() {
			auditor.Notify(audit.NewEvent(r, name))
		}
	}
}
