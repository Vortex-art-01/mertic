package middleware

import "net/http"

// WithBodyLimit ограничивает тело запроса n байтами. Мидлвари и хендлеры за ней
// читают тело целиком в память: WithHash — чтобы проверить подпись, JSON-хендлеры —
// чтобы разобрать запрос. Без предела один запрос мог бы занять всю память сервера.
//
// Чтение сверх n возвращает ошибку [*http.MaxBytesError], и тот, кто читает,
// отвечает на неё кодом 413 (см. [httpbody.ErrorStatus]). Запрос без тела
// мидлварь пропускает как есть: ограничивать в нём нечего.
//
// На сервере мидлварь стоит после [WithGzip] и ограничивает уже распакованное
// тело. Сжатое тело ограничивать мало: deflate сжимает повторы примерно в тысячу
// раз, и мегабайт gzip распаковывается в гигабайт.
func WithBodyLimit(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// У сервера тело есть у каждого запроса, но у запроса без тела это
			// http.NoBody: обёртка над ним стоила бы аллокации и ничего не дала.
			if r.Body != http.NoBody {
				r.Body = http.MaxBytesReader(w, r.Body, n)
			}

			next.ServeHTTP(w, r)
		})
	}
}
