package middleware

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
)

func gzipBytes(t testing.TB, data string) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(data)); err != nil {
		t.Fatalf("failed to compress %q: %v", data, err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	return buf.Bytes()
}

// echoHandler отдаёт тело запроса обратно с заданным Content-Type.
func echoHandler(contentType string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	})
}

func TestWithGzipDecompressesRequest(t *testing.T) {
	const body = `{"id":"Alloc","type":"gauge","value":123.45}`

	var got string
	handler := WithGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
		}
		got = string(raw)
	}))

	req := httptest.NewRequest(http.MethodPost, "/update", bytes.NewReader(gzipBytes(t, body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got != body {
		t.Errorf("handler received %q, want %q", got, body)
	}
}

func TestWithGzipRejectsBrokenRequestBody(t *testing.T) {
	called := false
	handler := WithGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader("not a gzip stream"))
	req.Header.Set("Content-Encoding", "gzip")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if called {
		t.Error("handler was called for a broken gzip body")
	}
}

func TestWithGzipUncompressedRequestPassesThrough(t *testing.T) {
	const body = `{"id":"Alloc","type":"gauge"}`

	var got string
	handler := WithGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got = string(raw)
	}))

	req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(httptest.NewRecorder(), req)

	if got != body {
		t.Errorf("handler received %q, want %q", got, body)
	}
}

func TestWithGzipCompressesResponse(t *testing.T) {
	tests := []struct {
		name           string
		contentType    string
		acceptEncoding string
		wantCompressed bool
	}{
		{
			name:           "json for gzip-capable client",
			contentType:    "application/json",
			acceptEncoding: "gzip",
			wantCompressed: true,
		},
		{
			name:           "html for gzip-capable client",
			contentType:    "text/html; charset=utf-8",
			acceptEncoding: "gzip",
			wantCompressed: true,
		},
		{
			name:           "gzip among other encodings",
			contentType:    "application/json",
			acceptEncoding: "deflate, gzip;q=1.0",
			wantCompressed: true,
		},
		{
			name:           "client without gzip support",
			contentType:    "application/json",
			acceptEncoding: "",
			wantCompressed: false,
		},
		{
			name:           "plain text is not compressed",
			contentType:    "text/plain; charset=utf-8",
			acceptEncoding: "gzip",
			wantCompressed: false,
		},
		{
			name:           "response without content type",
			contentType:    "",
			acceptEncoding: "gzip",
			wantCompressed: false,
		},
	}

	const body = `{"id":"Alloc","type":"gauge","value":123.45}`

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := WithGzip(echoHandler(tt.contentType))

			req := httptest.NewRequest(http.MethodPost, "/update", strings.NewReader(body))
			if tt.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want %d", res.StatusCode, http.StatusOK)
			}

			encoding := res.Header.Get("Content-Encoding")
			if tt.wantCompressed && encoding != "gzip" {
				t.Fatalf("Content-Encoding = %q, want %q", encoding, "gzip")
			}
			if !tt.wantCompressed && encoding != "" {
				t.Fatalf("Content-Encoding = %q, want empty", encoding)
			}

			var reader io.Reader = res.Body
			if tt.wantCompressed {
				zr, err := gzip.NewReader(res.Body)
				if err != nil {
					t.Fatalf("failed to open gzip response: %v", err)
				}
				defer zr.Close()

				reader = zr
			}

			got, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("failed to read response: %v", err)
			}
			if string(got) != body {
				t.Errorf("body = %q, want %q", got, body)
			}
		})
	}
}

// Несжатый поток обязан быть настоящим gzip: автотесты и агент читают его обычным
// gzip.Reader, который проверяет и контрольную сумму, и длину.
func TestStoredGzipIsValidStream(t *testing.T) {
	for _, size := range []int{0, 1, 15, maxStoredSize} {
		data := strings.Repeat("x", size)

		got := readGzip(t, storedGzip([]byte(data)))
		if got != data {
			t.Errorf("size %d: decompressed %q, want %q", size, got, data)
		}
	}
}

func readGzip(t testing.TB, data []byte) string {
	t.Helper()

	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Errorf("failed to open gzip stream: %v", err)
		return ""
	}
	defer zr.Close()

	got, err := io.ReadAll(zr)
	if err != nil {
		t.Errorf("failed to read gzip stream: %v", err)
	}

	return string(got)
}

// Какого бы размера ни был ответ и какими бы кусками он ни писался, клиент получает
// его целиком, в gzip и с тем статусом, что выставил хендлер.
func TestWithGzipResponseAroundStoredLimit(t *testing.T) {
	tests := []struct {
		name   string
		writes []int
	}{
		{name: "empty body"},
		{name: "one byte", writes: []int{1}},
		{name: "exactly at the limit", writes: []int{maxStoredSize}},
		{name: "one byte over the limit", writes: []int{maxStoredSize + 1}},
		{name: "much larger than the limit", writes: []int{100 * maxStoredSize}},
		{name: "small pieces under the limit", writes: []int{10, 20, 30}},
		{name: "pieces that end at the limit", writes: []int{maxStoredSize - 1, 1}},
		{name: "pieces that cross the limit", writes: []int{maxStoredSize - 1, 2, 500}},
		// Больше 64 КБ мелкими кусками: столько не поместилось бы в один блок stored,
		// длина которого занимает 16 бит.
		{name: "many pieces beyond a stored block", writes: slices.Repeat([]int{maxStoredSize}, 300)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var want strings.Builder

			handler := WithGzip(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusAccepted)

				for i, size := range tt.writes {
					piece := bytes.Repeat([]byte{byte('a' + i%26)}, size)
					want.Write(piece)

					n, err := w.Write(piece)
					if err != nil || n != size {
						t.Errorf("Write() = %d, %v; want %d, nil", n, err, size)
					}
				}
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Accept-Encoding", "gzip")

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusAccepted {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusAccepted)
			}
			if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
				t.Errorf("Content-Encoding = %q, want gzip", got)
			}
			if got := readGzip(t, rec.Body.Bytes()); got != want.String() {
				t.Errorf("body has %d bytes, want %d", len(got), want.Len())
			}
		})
	}
}

func TestCompressWriterCloseIsIdempotent(t *testing.T) {
	for _, size := range []int{10, maxStoredSize + 1} {
		rec := httptest.NewRecorder()
		rec.Header().Set("Content-Type", "application/json")

		w := &compressWriter{ResponseWriter: rec}
		if _, err := w.Write(bytes.Repeat([]byte("x"), size)); err != nil {
			t.Fatalf("Write() error = %v", err)
		}

		for range 2 {
			if err := w.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		}

		if got := readGzip(t, rec.Body.Bytes()); len(got) != size {
			t.Errorf("size %d: decompressed %d bytes", size, len(got))
		}
	}
}

// Ради этого всё и затевалось: маленькому ответу компрессор на 800 КБ не нужен. Порог
// с большим запасом: важно не точное число, а что компрессор здесь не создаётся.
func TestWithGzipSmallResponseNeedsNoCompressor(t *testing.T) {
	const limit = 16 << 10

	handler := WithGzip(drainAndReply)
	req := httptest.NewRequest(http.MethodPost, "/updates/", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	w := &discardWriter{header: make(http.Header)}

	const runs = 100

	var before, after runtime.MemStats

	runtime.ReadMemStats(&before)
	for range runs {
		clear(w.header)
		req.Body = http.NoBody
		handler.ServeHTTP(w, req)
	}
	runtime.ReadMemStats(&after)

	if perRun := (after.TotalAlloc - before.TotalAlloc) / runs; perRun > limit {
		t.Errorf("request allocated %d bytes, want at most %d", perRun, limit)
	}
}

// Сжатие переиспользуется, поэтому главный риск — чужие данные в ответе. Каждая
// горутина ждёт в ответе именно своё тело; большие тела идут через компрессор,
// маленькие — без него.
func TestWithGzipConcurrentRequestsDoNotMix(t *testing.T) {
	handler := WithGzip(echoHandler("application/json"))

	var running sync.WaitGroup

	for g := range 16 {
		running.Go(func() {
			for i := range 30 {
				want := fmt.Sprintf(`{"goroutine":%d,"request":%d}`, g, i)
				if i%2 == 0 {
					want = strings.Repeat(want, 50)
				}

				req := httptest.NewRequest(http.MethodPost, "/update", bytes.NewReader(gzipBytes(t, want)))
				req.Header.Set("Content-Encoding", "gzip")
				req.Header.Set("Accept-Encoding", "gzip")

				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)

				if got := readGzip(t, rec.Body.Bytes()); got != want {
					t.Errorf("goroutine %d: body has %d bytes, want %d", g, len(got), len(want))
				}
			}
		})
	}

	running.Wait()
}

// Сломанный запрос не должен портить читателя, который вернётся в пул.
func TestWithGzipRecoversAfterBrokenRequests(t *testing.T) {
	const body = `{"id":"Alloc","type":"gauge","value":123.45}`

	var got string
	var readErr error

	handler := WithGzip(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		var raw []byte

		raw, readErr = io.ReadAll(r.Body)
		got = string(raw)
	}))

	send := func(data []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/update", bytes.NewReader(data))
		req.Header.Set("Content-Encoding", "gzip")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		return rec
	}

	valid := gzipBytes(t, body)

	for range 3 {
		if rec := send([]byte("not a gzip stream")); rec.Code != http.StatusBadRequest {
			t.Errorf("broken stream: status = %d, want %d", rec.Code, http.StatusBadRequest)
		}

		send(valid[:len(valid)/2])
		if readErr == nil {
			t.Error("truncated stream: read error = nil")
		}

		send(valid)
		if readErr != nil || got != body {
			t.Errorf("valid stream after broken ones: got %q, %v; want %q", got, readErr, body)
		}
	}
}

// Хендлер вправе закрыть тело сам, а читатель всё равно должен вернуться в пул один
// раз: иначе он достался бы двум запросам сразу.
func TestWithGzipReaderReturnsToPoolOnce(t *testing.T) {
	handler := WithGzip(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_ = r.Body.Close()
		_ = r.Body.Close()
	}))

	req := httptest.NewRequest(http.MethodPost, "/update", bytes.NewReader(gzipBytes(t, testBody)))
	req.Header.Set("Content-Encoding", "gzip")

	handler.ServeHTTP(httptest.NewRecorder(), req)

	first, second := gzipReaders.Get(), gzipReaders.Get()
	defer gzipReaders.Put(first)
	defer gzipReaders.Put(second)

	if first == second {
		t.Error("the same reader came from the pool twice")
	}
}

func TestWithGzipKeepsErrorResponseReadable(t *testing.T) {
	handler := WithGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unknown metric type", http.StatusBadRequest)
	}))

	req := httptest.NewRequest(http.MethodPost, "/update", nil)
	req.Header.Set("Accept-Encoding", "gzip")

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want empty", got)
	}
	if got := w.Body.String(); got != "unknown metric type\n" {
		t.Errorf("body = %q, want %q", got, "unknown metric type\n")
	}
}
