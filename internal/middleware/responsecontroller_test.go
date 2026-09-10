package middleware

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Vortex-art-01/mertic/internal/hash"
)

// Обёртки не должны прятать настоящий ResponseWriter: http.NewResponseController
// обязан дотянуться до него сквозь всю цепочку мидлварей. Исключение —
// сброс буфера: при включённой подписи он невозможен и обязан честно об этом
// сказать, а не отправить клиенту неподписанный ответ.
func TestResponseControllerReachesRealWriter(t *testing.T) {
	var deadlineErr, flushErr error

	handler := WithLogging(slog.New(slog.DiscardHandler))(WithGzip(WithHash(testKey)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			rc := http.NewResponseController(w)

			deadlineErr = rc.SetWriteDeadline(time.Now().Add(time.Minute))
			flushErr = rc.Flush()

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(testBody))
		}))))

	srv := httptest.NewServer(handler)
	defer srv.Close()

	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}

	if deadlineErr != nil {
		t.Errorf("SetWriteDeadline() = %v, обёртки прячут настоящий writer", deadlineErr)
	}
	if !errors.Is(flushErr, http.ErrNotSupported) {
		t.Errorf("Flush() = %v, want %v", flushErr, http.ErrNotSupported)
	}
	if sign := res.Header.Get(hash.Header); !hash.Valid(body, testKey, sign) {
		t.Errorf("%s = %q, ответ не подписан", hash.Header, sign)
	}
	if string(body) != testBody {
		t.Errorf("body = %q, want %q", body, testBody)
	}
}
