package middleware

import (
	"bytes"
	"io"
	"net/http"

	"github.com/Vortex-art-01/mertic/internal/hash"
)

func WithHash(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if key == "" {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "failed to read request body", http.StatusBadRequest)
				return
			}

			r.Body = io.NopCloser(bytes.NewReader(body))

			if got := r.Header.Get(hash.Header); got != "" && got != hash.None {
				if !hash.Valid(body, key, got) {
					http.Error(w, "invalid hash", http.StatusBadRequest)
					return
				}
			}

			hw := &hashWriter{ResponseWriter: w, key: key}
			next.ServeHTTP(hw, r)
			hw.flush()
		})
	}
}

// hashWriter копит ответ целиком в памяти: подпись считается по всему телу,
// поэтому отдать его раньше, чем хендлер отработает, нельзя. Цена решения —
// нет стриминга (клиент не увидит ни байта до конца обработки), нет
// http.Flusher, а расход памяти растёт вместе с размером ответа. Для ответов
// сервиса — JSON с метриками в единицы килобайт — это осознанный компромисс;
// для большого или потокового ответа такую подпись придётся пересматривать.
type hashWriter struct {
	http.ResponseWriter
	key    string
	status int
	body   bytes.Buffer
}

func (w *hashWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *hashWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *hashWriter) flush() {
	w.Header().Set(hash.Header, hash.Sign(w.body.Bytes(), w.key))

	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.ResponseWriter.WriteHeader(w.status)

	if w.body.Len() > 0 {
		_, _ = w.ResponseWriter.Write(w.body.Bytes())
	}
}
