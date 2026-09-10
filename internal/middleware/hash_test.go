package middleware

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/hash"
)

const (
	testKey  = "secret"
	testBody = `[{"id":"Alloc","type":"gauge","value":123.45}]`
)

// signedRequest — POST с телом body и подписью sign в заголовке; пустой sign
// означает запрос без подписи.
func signedRequest(body, sign string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")

	if sign != "" {
		req.Header.Set(hash.Header, sign)
	}

	return req
}

func TestWithHashPassesValidSignature(t *testing.T) {
	var got string
	handler := WithHash(testKey)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
		}
		got = string(raw)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, signedRequest(testBody, hash.Sign([]byte(testBody), testKey)))

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	// Хендлер должен получить тело целиком, хотя middleware уже прочитало его.
	if got != testBody {
		t.Errorf("handler got body %q, want %q", got, testBody)
	}

	body, _ := io.ReadAll(res.Body)
	if want := `{"ok":true}`; string(body) != want {
		t.Errorf("response body = %q, want %q", body, want)
	}
	if sign := res.Header.Get(hash.Header); !hash.Valid(body, testKey, sign) {
		t.Errorf("response signature %q does not match body %q", sign, body)
	}
}

// Подделанная подпись — повод отбросить данные, а не сохранить их.
func TestWithHashRejectsInvalidSignature(t *testing.T) {
	called := false
	handler := WithHash(testKey)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, signedRequest(testBody, hash.Sign([]byte(testBody), "another")))

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusBadRequest)
	}
	if called {
		t.Error("handler was called for a request with a broken signature")
	}
}

// Клиент без ключа подписи не присылает — такие запросы сервер принимает.
func TestWithHashAcceptsUnsignedRequests(t *testing.T) {
	for _, sign := range []string{"", hash.None} {
		called := false
		handler := WithHash(testKey)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, signedRequest(testBody, sign))

		res := rec.Result()
		res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("%s=%q: status = %d, want %d", hash.Header, sign, res.StatusCode, http.StatusOK)
		}
		if !called {
			t.Errorf("%s=%q: handler was not called", hash.Header, sign)
		}
		if res.Header.Get(hash.Header) == "" {
			t.Errorf("%s=%q: response is not signed", hash.Header, sign)
		}
	}
}

// Статус ответа переживает буферизацию: middleware отдаёт его как есть.
func TestWithHashKeepsStatus(t *testing.T) {
	handler := WithHash(testKey)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, signedRequest(testBody, ""))

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusNotFound)
	}

	body, _ := io.ReadAll(res.Body)
	if sign := res.Header.Get(hash.Header); !hash.Valid(body, testKey, sign) {
		t.Errorf("response signature %q does not match body %q", sign, body)
	}
}

// Без ключа middleware не вмешивается вовсе.
func TestWithHashWithoutKeyDoesNothing(t *testing.T) {
	handler := WithHash("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, signedRequest(testBody, "явно неверная подпись"))

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if sign := res.Header.Get(hash.Header); sign != "" {
		t.Errorf("response signed with %q, want no signature without a key", sign)
	}
}

// Связка с gzip — тот случай, ради которого middleware и стоит внутри него:
// подпись считается по несжатому телу с обеих сторон.
func TestWithHashWorksWithGzip(t *testing.T) {
	handler := WithGzip(WithHash(testKey)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	})))

	req := httptest.NewRequest(http.MethodPost, "/updates/", bytes.NewReader(gzipBytes(t, testBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set(hash.Header, hash.Sign([]byte(testBody), testKey))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if enc := res.Header.Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", enc)
	}

	zr, err := gzip.NewReader(res.Body)
	if err != nil {
		t.Fatalf("failed to open gzip response: %v", err)
	}
	defer zr.Close()

	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	if string(body) != testBody {
		t.Errorf("response body = %q, want %q", body, testBody)
	}
	if sign := res.Header.Get(hash.Header); !hash.Valid(body, testKey, sign) {
		t.Errorf("response signature %q does not match uncompressed body %q", sign, body)
	}
}
