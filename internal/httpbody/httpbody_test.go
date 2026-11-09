package httpbody

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
)

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		ID string `json:"id"`
	}

	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{name: "valid", body: `{"id":"Alloc"}`, want: "Alloc"},
		{name: "whitespace around the value", body: " {\"id\":\"Alloc\"}\n", want: "Alloc"},
		{name: "empty body", body: "", wantErr: true},
		{name: "broken json", body: `{"id":`, wantErr: true},
		{name: "garbage after the value", body: `{"id":"Alloc"} oops`, wantErr: true},
		{name: "second value", body: `{"id":"Alloc"}{"id":"Frees"}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got payload

			err := DecodeJSON(strings.NewReader(tt.body), &got)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DecodeJSON() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got.ID != tt.want {
				t.Errorf("id = %q, want %q", got.ID, tt.want)
			}
		})
	}
}

func TestDecodeJSONReportsReadError(t *testing.T) {
	wantErr := errors.New("connection lost")

	var got map[string]string

	if err := DecodeJSON(iotest.ErrReader(wantErr), &got); !errors.Is(err, wantErr) {
		t.Errorf("DecodeJSON() error = %v, want %v", err, wantErr)
	}
}

// Результат разбора переживает возврат буфера в пул: следующий запрос затрёт буфер,
// но не то, что уже разобрано.
func TestDecodeJSONResultDoesNotReferToBuffer(t *testing.T) {
	var first struct {
		ID string `json:"id"`
	}

	if err := DecodeJSON(strings.NewReader(`{"id":"Alloc"}`), &first); err != nil {
		t.Fatalf("DecodeJSON() error = %v", err)
	}

	var second struct {
		ID string `json:"id"`
	}

	if err := DecodeJSON(strings.NewReader(`{"id":"XXXXX"}`), &second); err != nil {
		t.Fatalf("DecodeJSON() error = %v", err)
	}

	if first.ID != "Alloc" {
		t.Errorf("first id = %q, want %q", first.ID, "Alloc")
	}
}

// Тело больше предела — не то же самое, что битый JSON: клиенту нужен 413, чтобы
// он понял, что запрос надо уменьшить, а не исправить.
func TestErrorStatus(t *testing.T) {
	syntaxErr := DecodeJSON(strings.NewReader(`{"id":`), new(map[string]string))

	limited := http.MaxBytesReader(httptest.NewRecorder(), io.NopCloser(strings.NewReader(`{"id":"Alloc"}`)), 4)
	limitErr := DecodeJSON(limited, new(map[string]string))

	tests := []struct {
		name         string
		err          error
		wantCode     int
		wantReason   string
		wantTooLarge bool
	}{
		{
			name:         "body over the limit",
			err:          limitErr,
			wantCode:     http.StatusRequestEntityTooLarge,
			wantReason:   "request body too large",
			wantTooLarge: true,
		},
		{
			name:         "wrapped limit error",
			err:          fmt.Errorf("read body: %w", &http.MaxBytesError{Limit: 4}),
			wantCode:     http.StatusRequestEntityTooLarge,
			wantReason:   "request body too large",
			wantTooLarge: true,
		},
		{
			name:       "broken json",
			err:        syntaxErr,
			wantCode:   http.StatusBadRequest,
			wantReason: "invalid json body",
		},
		{
			name:       "read error",
			err:        errors.New("connection lost"),
			wantCode:   http.StatusBadRequest,
			wantReason: "invalid json body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatal("DecodeJSON() must have failed")
			}

			if got := TooLarge(tt.err); got != tt.wantTooLarge {
				t.Errorf("TooLarge(%v) = %t, want %t", tt.err, got, tt.wantTooLarge)
			}

			code, reason := ErrorStatus(tt.err)
			if code != tt.wantCode || reason != tt.wantReason {
				t.Errorf("ErrorStatus(%v) = %d %q, want %d %q",
					tt.err, code, reason, tt.wantCode, tt.wantReason)
			}
		})
	}
}

func TestPutBufferEmptiesBuffer(t *testing.T) {
	buf := GetBuffer()
	buf.WriteString("прошлый запрос")
	PutBuffer(buf)

	if got := GetBuffer(); got.Len() != 0 {
		t.Errorf("got buffer with %q, want an empty one", got.String())
	}
}

// Большой буфер в пул не возвращается: память должна освободиться вместе с запросом.
func TestPutBufferDropsOversizedBuffer(t *testing.T) {
	big := bytes.NewBuffer(make([]byte, 0, maxPooledSize+1))
	PutBuffer(big)

	for range 100 {
		buf := GetBuffer()
		if buf == big {
			t.Fatal("oversized buffer came back from the pool")
		}
		defer PutBuffer(buf)
	}
}
