package agent

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/model"
)

type recordedRequest struct {
	method      string
	path        string
	contentType string
	body        model.Metrics
}

func newTestServer(t *testing.T, status int) (*Client, *[]recordedRequest) {
	t.Helper()

	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recordedRequest{
			method:      r.Method,
			path:        r.URL.Path,
			contentType: r.Header.Get("Content-Type"),
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		if err := json.Unmarshal(raw, &rec.body); err != nil {
			t.Errorf("failed to decode request body %q: %v", raw, err)
		}

		requests = append(requests, rec)
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
	if want := "/update"; req.path != want {
		t.Errorf("path = %s, want %s", req.path, want)
	}
	if req.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", req.contentType, "application/json")
	}
	if req.body.ID != "Alloc" || req.body.MType != model.Gauge {
		t.Errorf("body = %+v, want id Alloc of type gauge", req.body)
	}
	if req.body.Value == nil || *req.body.Value != 123.45 {
		t.Errorf("body value = %v, want 123.45", req.body.Value)
	}
	if req.body.Delta != nil {
		t.Errorf("body delta = %v, want nil for gauge", *req.body.Delta)
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
	req := (*requests)[0]
	if want := "/update"; req.path != want {
		t.Errorf("path = %s, want %s", req.path, want)
	}
	if req.body.ID != "PollCount" || req.body.MType != model.Counter {
		t.Errorf("body = %+v, want id PollCount of type counter", req.body)
	}
	if req.body.Delta == nil || *req.body.Delta != 5 {
		t.Errorf("body delta = %v, want 5", req.body.Delta)
	}
	if req.body.Value != nil {
		t.Errorf("body value = %v, want nil for counter", *req.body.Value)
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
