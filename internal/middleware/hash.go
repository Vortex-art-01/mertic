package middleware

import (
	"bytes"
	"io"
	"net/http"

	"github.com/Vortex-art-01/mertic/internal/hash"
	"github.com/Vortex-art-01/mertic/internal/httpbody"
)

// WithHash проверяет подписи запросов и подписывает ответы ключом key. Подпись —
// HMAC-SHA256 от тела в hex — передаётся в заголовке [hash.Header].
//
// Подпись запроса проверяется, только если клиент её прислал: запрос без
// заголовка или со значением [hash.None] проходит без проверки, а с неверной
// подписью отклоняется с кодом 400. Почему так, объяснено у [hash.None].
//
// Подписывается каждый ответ. Для этого мидлварь накапливает ответ хендлера в
// памяти и отдаёт клиенту только целиком, так что потоковых ответов под ней нет.
//
// С пустым key подписи выключены: мидлварь возвращает next как есть.
func WithHash(key string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if key == "" {
			return next
		}

		signer := hash.NewSigner(key)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Подпись проверяется, только если клиент её прислал: политика
			// «кто подписал, того и проверяем» описана у hash.None
			if got := r.Header.Get(hash.Header); got != "" && got != hash.None {
				body := httpbody.GetBuffer()
				defer httpbody.PutBuffer(body)

				if _, err := body.ReadFrom(r.Body); err != nil {
					http.Error(w, "failed to read request body", http.StatusBadRequest)
					return
				}

				r.Body = io.NopCloser(bytes.NewReader(body.Bytes()))

				if !signer.Valid(body.Bytes(), got) {
					http.Error(w, "invalid hash", http.StatusBadRequest)
					return
				}
			}

			hw := &hashWriter{ResponseWriter: w, signer: signer, body: httpbody.GetBuffer()}
			defer httpbody.PutBuffer(hw.body)

			next.ServeHTTP(hw, r)
			hw.flush()
		})
	}
}

// hashWriter копит ответ целиком в памяти: подпись считается по всему телу,
// поэтому отдать его раньше, чем хендлер отработает, нельзя. Цена решения —
// нет стриминга (клиент не увидит ни байта до конца обработки, см.
// FlushError), а расход памяти растёт вместе с размером ответа. Для ответов
// сервиса — JSON с метриками в единицы килобайт — это осознанный компромисс;
// для большого или потокового ответа такую подпись придётся пересматривать.
type hashWriter struct {
	http.ResponseWriter
	signer *hash.Signer
	status int
	body   *bytes.Buffer
}

func (w *hashWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *hashWriter) FlushError() error {
	return http.ErrNotSupported
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
	w.Header().Set(hash.Header, w.signer.Sign(w.body.Bytes()))

	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.ResponseWriter.WriteHeader(w.status)

	if w.body.Len() > 0 {
		_, _ = w.ResponseWriter.Write(w.body.Bytes())
	}
}
