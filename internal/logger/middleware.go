package logger

import (
	"log/slog"
	"net/http"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.size += n
	return n, err
}

func WithLogging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			lw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(lw, r)

			log.Info("request handled",
				slog.String("uri", r.RequestURI),
				slog.String("method", r.Method),
				slog.Duration("duration", time.Since(start)),
				slog.Int("status", lw.status),
				slog.Int("size", lw.size),
			)
		})
	}
}
