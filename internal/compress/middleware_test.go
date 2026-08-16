package compress

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func gzipBytes(t *testing.T, data string) []byte {
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
