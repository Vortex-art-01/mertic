package ping

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// timeout ограничивает проверку: недоступная база не должна держать запрос
// до истечения собственных таймаутов драйвера.
const timeout = 3 * time.Second

type Pinger interface {
	PingContext(ctx context.Context) error
}

// New возвращает хендлер проверки соединения с базой. Если база не
// сконфигурирована, pinger равен nil и проверка считается неуспешной.
func New(pinger Pinger, l *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if pinger == nil {
			l.Error("ping: database is not configured")
			http.Error(w, "database is not configured", http.StatusInternalServerError)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		if err := pinger.PingContext(ctx); err != nil {
			l.Error("ping: database is unavailable", slog.Any("error", err))
			http.Error(w, "database is unavailable", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}
