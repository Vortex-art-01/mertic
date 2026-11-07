// Package ping проверяет доступность базы данных: GET /ping.
package ping

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

const timeout = 3 * time.Second

// Pinger проверяет соединение с базой. Его реализует *pgxpool.Pool.
type Pinger interface {
	Ping(ctx context.Context) error
}

// New возвращает хендлер GET /ping. С заданным pinger хендлер проверяет базу и
// отвечает 200, если она доступна, и 500, если нет или если проверка не
// уложилась в 3 секунды; причина уходит в лог l. С nil pinger — метрики
// хранятся в памяти — ответ всегда 200.
func New(pinger Pinger, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if pinger == nil {
			w.WriteHeader(http.StatusOK)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		if err := pinger.Ping(ctx); err != nil {
			l.Error("ping: database is unavailable", slog.Any("error", err))
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
