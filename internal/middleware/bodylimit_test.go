package middleware

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/hash"
)

// Тело до предела включительно доходит до хендлера целиком, а чтение сверх
// предела обрывается ошибкой *http.MaxBytesError.
func TestWithBodyLimit(t *testing.T) {
	const limit = 8

	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "under the limit", body: "1234567"},
		{name: "at the limit", body: "12345678"},
		{name: "over the limit", body: "123456789", wantErr: true},
		{name: "far over the limit", body: strings.Repeat("x", 1<<16), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				got     []byte
				readErr error
			)

			handler := WithBodyLimit(limit)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got, readErr = io.ReadAll(r.Body)
			}))

			req := httptest.NewRequest(http.MethodPost, "/updates/", strings.NewReader(tt.body))
			handler.ServeHTTP(httptest.NewRecorder(), req)

			var tooLarge *http.MaxBytesError
			if errors.As(readErr, &tooLarge) != tt.wantErr {
				t.Fatalf("read error = %v, want *http.MaxBytesError: %t", readErr, tt.wantErr)
			}
			if tt.wantErr {
				// Сверх предела хендлер не получает ни байта.
				if len(got) > limit {
					t.Errorf("handler read %d bytes, want at most %d", len(got), limit)
				}
				return
			}
			if string(got) != tt.body {
				t.Errorf("body = %q, want %q", got, tt.body)
			}
		})
	}
}

// Запрос без тела оборачивать незачем: читать в нём нечего, а обёртка стоила
// бы аллокации на каждый GET.
func TestWithBodyLimitSkipsRequestWithoutBody(t *testing.T) {
	handler := WithBodyLimit(8)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r.Body != http.NoBody {
			t.Errorf("body = %T, want http.NoBody", r.Body)
		}
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

// Предел после WithGzip ограничивает распакованное тело: сжатый поток сам
// укладывается в предел, но развернуться в памяти дальше него не может.
func TestWithBodyLimitAfterGzipLimitsUnpackedBody(t *testing.T) {
	const limit = 1 << 10

	unpacked := strings.Repeat(" ", 64*limit)
	compressed := gzipBytes(t, unpacked)

	if len(compressed) > limit {
		t.Fatalf("compressed body is %d bytes, want it to fit the limit of %d", len(compressed), limit)
	}

	var readErr error

	handler := WithGzip(WithBodyLimit(limit)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, readErr = io.Copy(io.Discard, r.Body)
	})))

	req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader(compressed))
	req.Header.Set("Content-Encoding", "gzip")

	handler.ServeHTTP(httptest.NewRecorder(), req)

	var tooLarge *http.MaxBytesError
	if !errors.As(readErr, &tooLarge) {
		t.Errorf("read error = %v, want *http.MaxBytesError", readErr)
	}
}

// WithHash читает тело до проверки подписи, то есть от любого, кто прислал
// заголовок. Тело больше предела отклоняется до хендлера, даже с верной подписью.
func TestWithHashRejectsTooLargeBody(t *testing.T) {
	body := strings.Repeat(" ", 1<<10) + testBody

	called := false
	handler := WithBodyLimit(1 << 10)(WithHash(testKey)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	})))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, signedRequest(body, hash.Sign([]byte(body), testKey)))

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusRequestEntityTooLarge)
	}
	if called {
		t.Error("too large body reached the handler")
	}
}
