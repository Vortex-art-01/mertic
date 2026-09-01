package agent

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/model"
)

type recordedRequest struct {
	method          string
	path            string
	contentType     string
	contentEncoding string
	body            []byte
}

// decode разбирает записанное тело в переданную структуру: у одиночной
// отправки это Metrics, у пакетной — []Metrics.
func (r recordedRequest) decode(t *testing.T, v any) {
	t.Helper()

	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("failed to decode request body %q: %v", r.body, err)
	}
}

func newTestServer(t *testing.T, status int) (*Client, *[]recordedRequest) {
	t.Helper()

	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recordedRequest{
			method:          r.Method,
			path:            r.URL.Path,
			contentType:     r.Header.Get("Content-Type"),
			contentEncoding: r.Header.Get("Content-Encoding"),
		}

		// Агент всегда сжимает тело, поэтому читаем через gzip.
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Errorf("failed to open gzip body: %v", err)
			w.WriteHeader(status)
			return
		}
		defer zr.Close()

		if rec.body, err = io.ReadAll(zr); err != nil {
			t.Errorf("failed to read request body: %v", err)
		}

		requests = append(requests, rec)
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)

	return NewClient(srv.URL), &requests
}

func TestClientSendBatch(t *testing.T) {
	client, requests := newTestServer(t, http.StatusOK)

	value, delta := 123.45, int64(5)
	batch := []model.Metrics{
		{ID: "Alloc", MType: model.Gauge, Value: &value},
		{ID: "PollCount", MType: model.Counter, Delta: &delta},
	}

	if err := client.SendBatch(batch); err != nil {
		t.Fatalf("SendBatch: %v", err)
	}

	// Весь пакет уходит одним запросом — в этом и смысл /updates/.
	if len(*requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(*requests))
	}
	req := (*requests)[0]
	if req.method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.method)
	}
	if want := "/updates/"; req.path != want {
		t.Errorf("path = %s, want %s", req.path, want)
	}
	if req.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", req.contentType, "application/json")
	}
	if req.contentEncoding != "gzip" {
		t.Errorf("Content-Encoding = %q, want %q", req.contentEncoding, "gzip")
	}

	var got []model.Metrics
	req.decode(t, &got)

	if len(got) != 2 {
		t.Fatalf("body has %d metrics, want 2", len(got))
	}
	if got[0].ID != "Alloc" || got[0].Value == nil || *got[0].Value != value {
		t.Errorf("body[0] = %+v, want Alloc gauge with value %v", got[0], value)
	}
	if got[1].ID != "PollCount" || got[1].Delta == nil || *got[1].Delta != delta {
		t.Errorf("body[1] = %+v, want PollCount counter with delta %d", got[1], delta)
	}
}

// Пустой пакет не повод беспокоить сервер.
func TestClientSendBatchSkipsEmpty(t *testing.T) {
	client, requests := newTestServer(t, http.StatusOK)

	if err := client.SendBatch(nil); err != nil {
		t.Fatalf("SendBatch: %v", err)
	}

	if len(*requests) != 0 {
		t.Errorf("got %d requests, want none", len(*requests))
	}
}

func TestClientSendBatchErrorStatus(t *testing.T) {
	client, _ := newTestServer(t, http.StatusInternalServerError)

	value := 1.0
	batch := []model.Metrics{{ID: "Alloc", MType: model.Gauge, Value: &value}}

	if err := client.SendBatch(batch); err == nil {
		t.Error("expected error on non-200 response, got nil")
	}
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
	if req.contentEncoding != "gzip" {
		t.Errorf("Content-Encoding = %q, want %q", req.contentEncoding, "gzip")
	}

	var got model.Metrics
	req.decode(t, &got)

	if got.ID != "Alloc" || got.MType != model.Gauge {
		t.Errorf("body = %+v, want id Alloc of type gauge", got)
	}
	if got.Value == nil || *got.Value != 123.45 {
		t.Errorf("body value = %v, want 123.45", got.Value)
	}
	if got.Delta != nil {
		t.Errorf("body delta = %v, want nil for gauge", *got.Delta)
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

	var got model.Metrics
	req.decode(t, &got)

	if got.ID != "PollCount" || got.MType != model.Counter {
		t.Errorf("body = %+v, want id PollCount of type counter", got)
	}
	if got.Delta == nil || *got.Delta != 5 {
		t.Errorf("body delta = %v, want 5", got.Delta)
	}
	if got.Value != nil {
		t.Errorf("body value = %v, want nil for counter", *got.Value)
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
