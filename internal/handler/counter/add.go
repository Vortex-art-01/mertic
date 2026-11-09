// Package counter обновляет счётчик через URL: POST /update/counter/{name}/{value}.
package counter

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Vortex-art-01/mertic/internal/audit"
)

// CounterSaver прибавляет приращение к счётчику: сервер хранит сумму всех
// присланных значений.
type CounterSaver interface {
	AddCounter(ctx context.Context, name string, value int64) error
}

// Auditor узнаёт о принятых метриках; кому он передаст событие, хендлеру неважно.
// Enabled позволяет не собирать событие, когда его некому отправить.
type Auditor interface {
	Enabled() bool
	Notify(event audit.Event)
}

// New возвращает хендлер POST /update/counter/{name}/{value}: он прибавляет
// value к счётчику name. Имя и приращение хендлер берёт из параметров пути name
// и value, поэтому маршрут должен их объявлять.
//
// Ответы:
//   - 200 без тела — приращение учтено;
//   - 400 — value не разбирается как int64;
//   - 404 — имя пустое;
//   - 500 — сбой хранилища, подробности уходят в лог l.
//
// О каждой принятой метрике хендлер сообщает auditor, если аудит включён.
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

		// Событие собирается, только если есть кому его отправить: по умолчанию
		// аудит выключен.
		if auditor.Enabled() {
			auditor.Notify(audit.NewEvent(r, name))
		}
	}
}
