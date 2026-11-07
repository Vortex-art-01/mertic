package middleware

import (
	"bufio"
	"compress/gzip"
	"encoding/binary"
	"hash/crc32"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/Vortex-art-01/mertic/internal/gzippool"
)

const (
	// maxStoredSize — до этого размера ответ уходит gzip-потоком без сжатия. Ответы
	// сервиса почти всегда крошечные: {"status":"ok"} или одна метрика. Deflate на
	// таких размерах экономит считанные байты, а компрессор занимает около 800 КБ.
	// Клиент разницы не заметит: для него это обычный gzip. Настоящий компрессор
	// достаётся только большим ответам, например странице со списком метрик.
	maxStoredSize = 256

	// storedOverhead — заголовок gzip, заголовок блока deflate, CRC32 и длина.
	storedOverhead = 10 + 5 + 4 + 4
)

var compressibleTypes = []string{"application/json", "text/html"}

func compressible(contentType string) bool {
	mediaType, _, _ := strings.Cut(contentType, ";")
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))

	for _, t := range compressibleTypes {
		if mediaType == t {
			return true
		}
	}
	return false
}

func acceptsGzip(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Accept-Encoding")), "gzip")
}

func sentGzip(r *http.Request) bool {
	return strings.Contains(strings.ToLower(r.Header.Get("Content-Encoding")), "gzip")
}

// storedGzip оборачивает data в gzip-поток из одного несжатого блока: заголовок gzip
// (RFC 1952), блок deflate типа stored (RFC 1951), затем CRC32 и длина. Размер блока
// ограничен 64 КБ, поэтому data не должна быть больше.
func storedGzip(data []byte) []byte {
	size := uint16(len(data))

	out := make([]byte, 0, len(data)+storedOverhead)
	out = append(out, 0x1f, 0x8b, 8, 0, 0, 0, 0, 0, 0, 0xff) // deflate, без флагов и времени, ОС неизвестна
	out = append(out, 1)                                     // последний блок, без сжатия
	out = binary.LittleEndian.AppendUint16(out, size)
	out = binary.LittleEndian.AppendUint16(out, ^size)
	out = append(out, data...)
	out = binary.LittleEndian.AppendUint32(out, crc32.ChecksumIEEE(data))
	out = binary.LittleEndian.AppendUint32(out, uint32(size))

	return out
}

type compressWriter struct {
	http.ResponseWriter
	small    []byte       // начало ответа, пока он умещается в maxStoredSize
	zw       *gzip.Writer // настоящий компрессор, когда ответ перестал умещаться
	compress bool
	decided  bool
}

func (w *compressWriter) decide() {
	if w.decided {
		return
	}
	w.decided = true

	if !compressible(w.Header().Get("Content-Type")) {
		return
	}
	w.compress = true

	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Del("Content-Length")
}

func (w *compressWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *compressWriter) WriteHeader(status int) {
	w.decide()
	w.ResponseWriter.WriteHeader(status)
}

func (w *compressWriter) Write(b []byte) (int, error) {
	w.decide()

	switch {
	case !w.compress:
		return w.ResponseWriter.Write(b)
	case w.zw != nil:
		return w.zw.Write(b)
	case len(w.small)+len(b) <= maxStoredSize:
		w.small = append(w.small, b...)
		return len(b), nil
	}

	w.zw = gzippool.Get(w.ResponseWriter)

	if _, err := w.zw.Write(w.small); err != nil {
		return 0, err
	}
	return w.zw.Write(b)
}

// Close дописывает поток и отдаёт компрессор обратно. Повторный вызов ничего не
// делает: иначе один компрессор оказался бы свободным дважды и достался двум запросам.
func (w *compressWriter) Close() error {
	if !w.compress {
		return nil
	}
	w.compress = false

	if w.zw == nil {
		_, err := w.ResponseWriter.Write(storedGzip(w.small))
		return err
	}

	err := w.zw.Close()

	gzippool.Put(w.zw)
	w.zw = nil

	return err
}

// compressReader хранит вместе с gzip.Reader собственный bufio.Reader: получив обычный
// io.Reader, gzip.Reader.Reset каждый раз создавал бы новый буфер на 4 КБ.
type compressReader struct {
	body io.ReadCloser
	br   *bufio.Reader
	zr   *gzip.Reader
}

// gzipReaders — пул распаковщиков: у каждого окно на 40 КБ. В отличие от компрессоров
// они нужны почти каждому запросу, поэтому sync.Pool им как раз подходит.
var gzipReaders = sync.Pool{New: func() any {
	return &compressReader{br: bufio.NewReader(nil), zr: new(gzip.Reader)}
}}

func newCompressReader(body io.ReadCloser) (*compressReader, error) {
	r := gzipReaders.Get().(*compressReader)

	r.body = body
	r.br.Reset(body)

	if err := r.zr.Reset(r.br); err != nil {
		r.release()
		return nil, err
	}

	return r, nil
}

func (r *compressReader) Read(p []byte) (int, error) {
	return r.zr.Read(p)
}

func (r *compressReader) Close() error {
	err := r.zr.Close()

	if bodyErr := r.body.Close(); err == nil {
		err = bodyErr
	}

	return err
}

// release отвязывает читателя от запроса и возвращает его в пул. Это не часть Close:
// хендлер тоже может закрыть тело, и тогда читатель вернулся бы в пул дважды.
func (r *compressReader) release() {
	r.body = nil
	r.br.Reset(nil)

	gzipReaders.Put(r)
}

// WithGzip поддерживает gzip в обе стороны. Тело запроса с заголовком
// Content-Encoding: gzip мидлварь распаковывает, а запрос, тело которого не
// похоже на gzip, отклоняет с кодом 400. Ответ клиенту, приславшему
// Accept-Encoding: gzip, она сжимает, если его Content-Type — application/json
// или text/html; остальные ответы уходят как есть. Хендлер за мидлварью читает и
// пишет несжатые данные.
//
// Ответ до 256 байт уходит gzip-потоком без сжатия: для клиента это обычный gzip,
// а держать ради нескольких байт компрессор на 800 КБ незачем.
func WithGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sentGzip(r) {
			cr, err := newCompressReader(r.Body)
			if err != nil {
				http.Error(w, "invalid gzip body", http.StatusBadRequest)
				return
			}
			defer func() {
				_ = cr.Close()
				cr.release()
			}()

			r.Body = cr
		}

		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Add("Vary", "Accept-Encoding")

		cw := &compressWriter{ResponseWriter: w}
		defer cw.Close()

		next.ServeHTTP(cw, r)
	})
}
