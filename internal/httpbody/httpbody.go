// Package httpbody читает и накапливает тела запросов и ответов в буферах из пула,
// чтобы не выделять память под них заново на каждый запрос.
package httpbody

import (
	"bytes"
	"encoding/json"
	"io"
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
func DecodeJSON(body io.Reader, v any) error {
	buf := GetBuffer()
	defer PutBuffer(buf)

	if _, err := buf.ReadFrom(body); err != nil {
		return err
	}

	return json.Unmarshal(buf.Bytes(), v)
}
