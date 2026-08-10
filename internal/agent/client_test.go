package agent

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type recordedRequest struct {
	method      string
	path        string
	contentType string
}

func newTestServer(t *testing.T, status int) (*Client, *[]recordedRequest) {
	t.Helper()

	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, recordedRequest{
			method:      r.Method,
			path:        r.URL.Path,
			contentType: r.Header.Get("Content-Type"),
		})
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	return NewClient(srv.URL), &requests
}

func TestClientSendGauge(t *testing.T) {
	client, requests := newTestServer(t, http.StatusOK)

	if err := client.SendGauge("Alloc", 123.45); err != nil {
		t.Fatalf("SendGauge: %v", err)
	}

	if len(*requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(*requests))
	}
	req := (*requests)[0]
	if req.method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.method)
	}
	if want := "/update/gauge/Alloc/123.45"; req.path != want {
		t.Errorf("path = %s, want %s", req.path, want)
	}
	if req.contentType != "text/plain" {
		t.Errorf("Content-Type = %q, want %q", req.contentType, "text/plain")
	}
}

func TestClientSendCounter(t *testing.T) {
	client, requests := newTestServer(t, http.StatusOK)

	if err := client.SendCounter("PollCount", 5); err != nil {
		t.Fatalf("SendCounter: %v", err)
	}

	if len(*requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(*requests))
	}
	if want := "/update/counter/PollCount/5"; (*requests)[0].path != want {
		t.Errorf("path = %s, want %s", (*requests)[0].path, want)
	}
}

func TestClientSendErrorStatus(t *testing.T) {
	client, _ := newTestServer(t, http.StatusBadRequest)

	if err := client.SendGauge("Alloc", 1); err == nil {
		t.Error("expected error on non-200 response, got nil")
	}
}

func TestClientSendServerUnavailable(t *testing.T) {
	client := NewClient("http://127.0.0.1:1") // заведомо недоступный адрес

	if err := client.SendGauge("Alloc", 1); err == nil {
		t.Error("expected error when server is unreachable, got nil")
	}
}
