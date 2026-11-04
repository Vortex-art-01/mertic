package agent

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Vortex-art-01/mertic/internal/hash"
	"github.com/Vortex-art-01/mertic/internal/model"
)

type recordedRequest struct {
	method          string
	path            string
	contentType     string
	contentEncoding string
	sign            string
	body            []byte
}

func (r recordedRequest) decode(t *testing.T, v any) {
	t.Helper()

	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("failed to decode request body %q: %v", r.body, err)
	}
}

func newTestServer(t *testing.T, status int) (*Client, *[]recordedRequest) {
	return newTestServerWithKey(t, status, "")
}

func newTestServerWithKey(t *testing.T, status int, key string) (*Client, *[]recordedRequest) {
	t.Helper()

	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := recordedRequest{
			method:          r.Method,
			path:            r.URL.Path,
			contentType:     r.Header.Get("Content-Type"),
			contentEncoding: r.Header.Get("Content-Encoding"),
			sign:            r.Header.Get(hash.Header),
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

	return newTestClient(srv.URL, key), &requests
}

func TestClientSendBatch(t *testing.T) {
	client, requests := newTestServer(t, http.StatusOK)

	value, delta := 123.45, int64(5)
	batch := []model.Metrics{
		{ID: "Alloc", MType: model.Gauge, Value: &value},
		{ID: "PollCount", MType: model.Counter, Delta: &delta},
	}

	if err := client.SendBatch(t.Context(), batch); err != nil {
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

	if err := client.SendBatch(t.Context(), nil); err != nil {
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

	if err := client.SendBatch(t.Context(), batch); err == nil {
		t.Error("expected error on non-200 response, got nil")
	}
}

// newTestClient — клиент с теми же повторами, но без настоящих пауз между
// ними: проверять расписание задержек — дело тестов пакета retry.
func newTestClient(baseURL, key string) *Client {
	c := NewClient(baseURL, key)
	c.delays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}

	return c
}

// flakyServer отвечает ошибкой status первым failures запросам и успехом —
// всем последующим: так ведёт себя сервер, который вот-вот поднимется.
func flakyServer(t *testing.T, failures int, status int) (*Client, *int) {
	t.Helper()

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++

		if requests <= failures {
			w.WriteHeader(status)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	return newTestClient(srv.URL, ""), &requests
}

// Временная ошибка сервера — повод повторить, а не потерять метрики.
func TestClientRetriesUntilServerRecovers(t *testing.T) {
	client, requests := flakyServer(t, 2, http.StatusServiceUnavailable)

	value := 1.0
	batch := []model.Metrics{{ID: "Alloc", MType: model.Gauge, Value: &value}}

	if err := client.SendBatch(t.Context(), batch); err != nil {
		t.Fatalf("SendBatch: %v", err)
	}

	// Две неудачи и успех с третьей попытки.
	if *requests != 3 {
		t.Errorf("server got %d requests, want 3", *requests)
	}
}

// Повторов ровно три сверх первой попытки — дальше отправка сдаётся.
func TestClientGivesUpAfterThreeRetries(t *testing.T) {
	client, requests := flakyServer(t, 100, http.StatusServiceUnavailable)

	value := 1.0
	batch := []model.Metrics{{ID: "Alloc", MType: model.Gauge, Value: &value}}

	err := client.SendBatch(t.Context(), batch)
	if err == nil {
		t.Fatal("expected error after all attempts failed, got nil")
	}

	var status *StatusError
	if !errors.As(err, &status) {
		t.Fatalf("error = %v, want *StatusError", err)
	}

	if *requests != 4 {
		t.Errorf("server got %d requests, want 4 (1 attempt + 3 retries)", *requests)
	}
}

// Отказ по существу запроса повторять бессмысленно: ответ не изменится.
func TestClientDoesNotRetryBadRequest(t *testing.T) {
	client, requests := flakyServer(t, 100, http.StatusBadRequest)

	value := 1.0
	batch := []model.Metrics{{ID: "Alloc", MType: model.Gauge, Value: &value}}

	if err := client.SendBatch(t.Context(), batch); err == nil {
		t.Fatal("expected error on 400 response, got nil")
	}

	if *requests != 1 {
		t.Errorf("server got %d requests, want 1", *requests)
	}
}

// Недоступный сервер — тот самый случай, ради которого повторы и заведены:
// агент обязан пережить его, вернув ошибку, а не завершившись.
func TestClientRetriesUnreachableServer(t *testing.T) {
	client := newTestClient("http://127.0.0.1:1", "") // заведомо недоступный адрес

	value := 1.0
	batch := []model.Metrics{{ID: "Alloc", MType: model.Gauge, Value: &value}}

	err := client.SendBatch(t.Context(), batch)
	if err == nil {
		t.Fatal("expected error when server is unreachable, got nil")
	}

	var transport *TransportError
	if !errors.As(err, &transport) {
		t.Errorf("error = %v, want *TransportError", err)
	}
}

// Отмена контекста прекращает повторы сразу: досиживать паузы незачем.
func TestClientStopsRetryingOnCanceledContext(t *testing.T) {
	client, requests := flakyServer(t, 100, http.StatusServiceUnavailable)
	client.delays = []time.Duration{time.Hour, time.Hour, time.Hour}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	value := 1.0
	batch := []model.Metrics{{ID: "Alloc", MType: model.Gauge, Value: &value}}

	if err := client.SendBatch(ctx, batch); err == nil {
		t.Fatal("expected error on canceled context, got nil")
	}

	if *requests != 0 {
		t.Errorf("server got %d requests, want none", *requests)
	}
}

// С ключом агент подписывает то самое тело, которое потом сжимает: сервер
// проверяет подпись уже после распаковки.
func TestClientSignsBatch(t *testing.T) {
	const key = "secret"

	client, requests := newTestServerWithKey(t, http.StatusOK, key)

	value := 123.45
	batch := []model.Metrics{{ID: "Alloc", MType: model.Gauge, Value: &value}}

	if err := client.SendBatch(t.Context(), batch); err != nil {
		t.Fatalf("SendBatch: %v", err)
	}

	if len(*requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(*requests))
	}

	req := (*requests)[0]
	if req.sign == "" {
		t.Fatalf("request has no %s header", hash.Header)
	}
	if !hash.Valid(req.body, key, req.sign) {
		t.Errorf("signature %q does not match body %q", req.sign, req.body)
	}
}

// Без ключа заголовку подписи взяться неоткуда.
func TestClientDoesNotSignWithoutKey(t *testing.T) {
	client, requests := newTestServer(t, http.StatusOK)

	value := 123.45
	batch := []model.Metrics{{ID: "Alloc", MType: model.Gauge, Value: &value}}

	if err := client.SendBatch(t.Context(), batch); err != nil {
		t.Fatalf("SendBatch: %v", err)
	}

	if len(*requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(*requests))
	}
	if sign := (*requests)[0].sign; sign != "" {
		t.Errorf("%s = %q, want no signature without a key", hash.Header, sign)
	}
}
