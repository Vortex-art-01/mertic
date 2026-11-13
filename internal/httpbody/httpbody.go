// Package httpbody читает и накапливает тела запросов и ответов в буферах из пула,
// чтобы не выделять память под них заново на каждый запрос.
package httpbody

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
)

// maxPooledSize ограничивает буферы, которые возвращаются в пул: из-за одного большого
// тела память не должна оставаться занятой, пока буфер ходит по кругу.
const maxPooledSize = 64 << 10

var buffers = sync.Pool{New: func() any { return new(bytes.Buffer) }}

// GetBuffer возвращает пустой буфер.
func GetBuffer() *bytes.Buffer {
	return buffers.Get().(*bytes.Buffer)
}

// PutBuffer возвращает буфер в пул. После этого ни им, ни срезами его содержимого
// пользоваться нельзя: буфер достанется другому запросу.
func PutBuffer(buf *bytes.Buffer) {
	if buf.Cap() > maxPooledSize {
		return
	}

	buf.Reset()
	buffers.Put(buf)
}

// DecodeJSON читает тело целиком и разбирает его в v. Строки и числа encoding/json
// копирует, поэтому v не ссылается на буфер и может пережить вызов.
//
// В отличие от json.Decoder здесь не остаётся незамеченным мусор после JSON-значения:
// такое тело считается ошибкой.
//
// Размер тела DecodeJSON не ограничивает: предел ставит middleware.WithBodyLimit на
// весь роутер, потому что тело целиком читает и проверка подписи, ещё до хендлера.
// На ошибку DecodeJSON отвечайте кодом из [ErrorStatus].
func DecodeJSON(body io.Reader, v any) error {
	buf := GetBuffer()
	defer PutBuffer(buf)

	if _, err := buf.ReadFrom(body); err != nil {
		return err
	}

	return json.Unmarshal(buf.Bytes(), v)
}

// TooLarge сообщает, что чтение тела оборвалось на пределе размера: так отвечает
// [http.MaxBytesReader], которым тело оборачивает middleware.WithBodyLimit.
func TooLarge(err error) bool {
	var tooLarge *http.MaxBytesError
	return errors.As(err, &tooLarge)
}

// ErrorStatus подбирает ответ на ошибку [DecodeJSON]: 413, если тело больше
// предела, и 400 на всё остальное — тело оборвалось или это не JSON.
func ErrorStatus(err error) (code int, reason string) {
	if TooLarge(err) {
		return http.StatusRequestEntityTooLarge, "request body too large"
	}

	return http.StatusBadRequest, "invalid json body"
}
