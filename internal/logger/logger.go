// Package logger создаёт логгер, которым пишут сервер и агент.
package logger

import (
	"io"
	"log/slog"
)

// New возвращает логгер, который пишет в w записи уровня Info и выше, по одному
// JSON-объекту на строку.
func New(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
