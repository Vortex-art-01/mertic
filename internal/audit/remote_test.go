package audit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestRemote(t *testing.T, url string) *Remote {
	t.Helper()

	remote, err := NewRemote(url)
	if err != nil {
		t.Fatalf("NewRemote() error = %v", err)
	}

	return remote
}

func TestRemotePostsEvent(t *testing.T) {
	type request struct {
		method      string
		path        string
		contentType string
		body        string
	}

	received := make(chan request, 1)

	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
		}

		received <- request{r.Method, r.URL.Path, r.Header.Get("Content-Type"), string(body)}
	}))
	defer ts.Close()

	event := Event{TS: 12345678, Metrics: []string{"Alloc", "Frees"}, IPAddress: "192.168.0.42"}

	if err := newTestRemote(t, ts.URL+"/audit").Update(t.Context(), event); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	want := request{
		method:      http.MethodPost,
		path:        "/audit",
		contentType: "application/json",
		body:        `{"ts":12345678,"metrics":["Alloc","Frees"],"ip_address":"192.168.0.42"}`,
	}
	if got := <-received; got != want {
		t.Errorf("request = %+v, want %+v", got, want)
	}
}

func TestRemoteChecksResponseStatus(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{"ok", http.StatusOK, false},
		{"accepted", http.StatusAccepted, false},
		{"no content", http.StatusNoContent, false},
		{"bad request", http.StatusBadRequest, true},
		{"server error", http.StatusInternalServerError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer ts.Close()

			err := newTestRemote(t, ts.URL).Update(t.Context(), Event{TS: 1})

			if tt.wantErr {
				if err == nil {
					t.Fatal("Update() error = nil, want an error")
				}
				if !strings.Contains(err.Error(), http.StatusText(tt.status)) {
					t.Errorf("error %q must contain the response status", err)
				}
				return
			}

			if err != nil {
				t.Errorf("Update() error = %v", err)
			}
		})
	}
}

func TestRemoteReportsUnavailableServer(t *testing.T) {
	ts := httptest.NewServer(http.NotFoundHandler())
	remote := newTestRemote(t, ts.URL)
	ts.Close()

	if err := remote.Update(t.Context(), Event{TS: 1}); err == nil {
		t.Error("Update() error = nil, want an error for an unavailable server")
	}
}

// Отмена доставки при остановке сервера должна доходить и до HTTP-запроса.
func TestRemoteHonoursContext(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("request must not be sent with a cancelled context")
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := newTestRemote(t, ts.URL).Update(ctx, Event{TS: 1})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Update() error = %v, want %v", err, context.Canceled)
	}
}

// Ошибки уходят в лог, а в полном URL может быть токен доступа.
func TestRemoteErrorsDoNotExposeURL(t *testing.T) {
	const token = "secret-token"

	t.Run("delivery failure", func(t *testing.T) {
		ts := httptest.NewServer(http.NotFoundHandler())
		remote := newTestRemote(t, ts.URL+"/audit?token="+token)
		ts.Close()

		err := remote.Update(t.Context(), Event{TS: 1})
		if err == nil {
			t.Fatal("Update() error = nil, want an error for an unavailable server")
		}
		if strings.Contains(err.Error(), token) {
			t.Errorf("error %q exposes the token", err)
		}
	})

	t.Run("invalid url", func(t *testing.T) {
		_, err := NewRemote("http://example.com:port/audit?token=" + token)
		if err == nil {
			t.Fatal("NewRemote() error = nil, want an error for an invalid port")
		}
		if strings.Contains(err.Error(), token) {
			t.Errorf("error %q exposes the token", err)
		}
	})
}

func TestNewRemoteValidatesURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"http", "http://localhost:9090/audit", false},
		{"https", "https://example.com/audit?token=1", false},
		{"empty", "", true},
		{"without scheme", "localhost:9090/audit", true},
		{"path only", "/audit", true},
		{"without host", "http://", true},
		{"other scheme", "ftp://example.com/audit", true},
		{"garbage", "не адрес", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewRemote(tt.url)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewRemote(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
