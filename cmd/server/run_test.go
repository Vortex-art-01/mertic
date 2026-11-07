package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Vortex-art-01/mertic/internal/audit"
	"github.com/Vortex-art-01/mertic/internal/hash"
	"github.com/Vortex-art-01/mertic/internal/model"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

// freeAddr занимает свободный порт и сразу отдаёт его: перехватить его между
// вызовом и стартом сервера в тестовом окружении практически невозможно.
func freeAddr(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to reserve a port: %v", err)
	}
	defer ln.Close()

	return ln.Addr().String()
}

func waitReady(t *testing.T, url string) *http.Response {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		res, err := http.Get(url)
		if err == nil {
			return res
		}
		if time.Now().After(deadline) {
			t.Fatalf("server did not start: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Сервер поднимается с уже сохранённым дампом и должен и восстановить его,
// и дописать при остановке метрики, до которых тикер дойти не успел.
func TestRunRestoresAndSavesOnShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics-db.json")
	saved := `[{"id":"Alloc","type":"gauge","value":1.5}]`

	if err := os.WriteFile(path, []byte(saved), 0o644); err != nil {
		t.Fatalf("failed to write dump: %v", err)
	}

	cfg := config{
		runAddr: freeAddr(t),
		// Интервал заведомо больше теста: дамп в конце может появиться
		// только благодаря сохранению при остановке.
		storeInterval: time.Hour,
		fileStorage:   path,
		restore:       true,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, cfg, slog.New(slog.DiscardHandler))
	}()

	base := "http://" + cfg.runAddr

	res := waitReady(t, base+"/value/gauge/Alloc")
	body, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	if string(body) != "1.5" {
		t.Errorf("restored gauge Alloc = %q, want %q", body, "1.5")
	}

	update, err := http.Post(base+"/update/counter/PollCount/5", "text/plain", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	update.Body.Close()

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run() did not return after the context was cancelled")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read dump: %v", err)
	}

	var metrics []model.Metrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		t.Fatalf("dump is not valid json: %v\n%s", err, data)
	}

	got := make(map[string]model.Metrics, len(metrics))
	for _, m := range metrics {
		got[m.ID] = m
	}

	if m, ok := got["Alloc"]; !ok || m.Value == nil || *m.Value != 1.5 {
		t.Errorf("dump has no Alloc gauge with value 1.5:\n%s", data)
	}
	if m, ok := got["PollCount"]; !ok || m.Delta == nil || *m.Delta != 5 {
		t.Errorf("dump has no PollCount counter with delta 5:\n%s", data)
	}
}

// Если сервер не смог занять адрес, дамп трогать нельзя: пустое хранилище
// затёрло бы уже сохранённые метрики.
func TestRunKeepsDumpWhenAddressIsBusy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics-db.json")
	saved := `[{"id":"Alloc","type":"gauge","value":1.5}]`

	if err := os.WriteFile(path, []byte(saved), 0o644); err != nil {
		t.Fatalf("failed to write dump: %v", err)
	}

	busy, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to occupy a port: %v", err)
	}
	defer busy.Close()

	cfg := config{
		runAddr:       busy.Addr().String(),
		storeInterval: time.Hour,
		fileStorage:   path,
		restore:       false,
	}

	if err := run(context.Background(), cfg, slog.New(slog.DiscardHandler)); err == nil {
		t.Fatal("run() error = nil, want an error for a busy address")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read dump: %v", err)
	}
	if string(data) != saved {
		t.Errorf("dump = %s, want it untouched:\n%s", data, saved)
	}
}

// Метрики, принятые любым из эндпоинтов, доходят до обоих приёмников, отказ
// не доходит ни до одного, а run не завершается, пока события не разосланы.
func TestRunSendsAuditEventsToEveryObserver(t *testing.T) {
	received := make(chan audit.Event, 10)

	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var event audit.Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			t.Errorf("audit server got invalid json: %v", err)
			return
		}

		// Приёмник отвечает не сразу: к остановке сервера часть событий ещё
		// будет в очереди, и run обязан их дождаться.
		time.Sleep(50 * time.Millisecond)

		received <- event
	}))
	defer receiver.Close()

	path := filepath.Join(t.TempDir(), "logs", "audit.log")

	cfg := config{
		runAddr:   freeAddr(t),
		auditFile: path,
		auditURL:  receiver.URL + "/audit",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, cfg, slog.New(slog.DiscardHandler))
	}()

	base := "http://" + cfg.runAddr

	waitReady(t, base+"/ping").Body.Close()

	requests := []struct {
		path       string
		body       string
		wantStatus int
	}{
		{
			path:       "/updates/",
			body:       `[{"id":"Alloc","type":"gauge","value":1},{"id":"PollCount","type":"counter","delta":2}]`,
			wantStatus: http.StatusOK,
		},
		{path: "/update/", body: `{"id":"HeapSys","type":"gauge","value":3}`, wantStatus: http.StatusOK},
		{path: "/update/gauge/Frees/4", wantStatus: http.StatusOK},
		{path: "/update/counter/Requests/5", wantStatus: http.StatusOK},
		{path: "/update/histogram/Rejected/6", wantStatus: http.StatusBadRequest},
		{path: "/update/gauge/Rejected/invalid", wantStatus: http.StatusBadRequest},
		{path: "/update/", body: `{"id":"Rejected","type":"histogram"}`, wantStatus: http.StatusBadRequest},
		{path: "/updates/", body: `[]`, wantStatus: http.StatusOK},
		{path: "/value/", body: `{"id":"Alloc","type":"gauge"}`, wantStatus: http.StatusOK},
	}

	for _, req := range requests {
		res, err := http.Post(base+req.path, "application/json", strings.NewReader(req.body))
		if err != nil {
			t.Fatalf("POST %s failed: %v", req.path, err)
		}
		res.Body.Close()

		if res.StatusCode != req.wantStatus {
			t.Errorf("POST %s status = %d, want %d", req.path, res.StatusCode, req.wantStatus)
		}
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run() did not return after the context was cancelled")
	}

	remote := make([]audit.Event, 0, len(received))
	for len(received) > 0 {
		remote = append(remote, <-received)
	}

	checkAuditEvents(t, "audit server", remote, cfg.runAddr)
	checkAuditEvents(t, "audit file", readAuditFile(t, path), cfg.runAddr)
}

func readAuditFile(t *testing.T, path string) []audit.Event {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open audit file: %v", err)
	}
	defer file.Close()

	var events []audit.Event

	lines := bufio.NewScanner(file)
	for lines.Scan() {
		var event audit.Event
		if err := json.Unmarshal(lines.Bytes(), &event); err != nil {
			t.Fatalf("audit file line is not valid json: %v\n%s", err, lines.Text())
		}

		events = append(events, event)
	}
	if err := lines.Err(); err != nil {
		t.Fatalf("failed to read audit file: %v", err)
	}

	return events
}

func checkAuditEvents(t *testing.T, sink string, events []audit.Event, serverAddr string) {
	t.Helper()

	want := [][]string{{"Alloc", "PollCount"}, {"HeapSys"}, {"Frees"}, {"Requests"}}

	if len(events) != len(want) {
		t.Fatalf("%s got %d events, want %d: %+v", sink, len(events), len(want), events)
	}

	// Клиент и сервер в тесте на одной машине, поэтому адрес клиента совпадает
	// с адресом, на котором слушает сервер.
	wantIP, _, err := net.SplitHostPort(serverAddr)
	if err != nil {
		t.Fatalf("failed to parse server address: %v", err)
	}

	for i, event := range events {
		if !slices.Equal(event.Metrics, want[i]) {
			t.Errorf("%s event %d metrics = %v, want %v", sink, i, event.Metrics, want[i])
		}
		if event.IPAddress != wantIP {
			t.Errorf("%s event %d ip_address = %q, want %q", sink, i, event.IPAddress, wantIP)
		}
		if time.Since(time.Unix(event.TS, 0)) > time.Minute {
			t.Errorf("%s event %d ts = %d, want the current time", sink, i, event.TS)
		}
	}
}

// Профили доступны только на отдельном адресе: на порту с API метрик им делать нечего.
func TestRunServesPprofOnDedicatedAddress(t *testing.T) {
	cfg := config{runAddr: freeAddr(t), pprofAddr: freeAddr(t)}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, cfg, slog.New(slog.DiscardHandler))
	}()

	waitReady(t, "http://"+cfg.runAddr+"/ping").Body.Close()

	profile, err := http.Get("http://" + cfg.pprofAddr + "/debug/pprof/heap")
	if err != nil {
		t.Fatalf("failed to fetch the profile: %v", err)
	}
	defer profile.Body.Close()

	if profile.StatusCode != http.StatusOK {
		t.Errorf("profile status = %d, want %d", profile.StatusCode, http.StatusOK)
	}

	data, err := io.ReadAll(profile.Body)
	if err != nil {
		t.Fatalf("failed to read the profile: %v", err)
	}
	if len(data) == 0 {
		t.Error("profile is empty")
	}

	api, err := http.Get("http://" + cfg.runAddr + "/debug/pprof/heap")
	if err != nil {
		t.Fatalf("request to the API port failed: %v", err)
	}
	defer api.Body.Close()

	if api.StatusCode != http.StatusNotFound {
		t.Errorf("API port status = %d, want %d", api.StatusCode, http.StatusNotFound)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run() error = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run() did not return after the context was cancelled")
	}

	// Вместе с сервером останавливается и профилировщик: порт снова свободен.
	ln, err := net.Listen("tcp", cfg.pprofAddr)
	if err != nil {
		t.Errorf("pprof port is still busy after shutdown: %v", err)
		return
	}
	ln.Close()
}

// Занятый адрес профилировщика — такая же ошибка конфигурации, как и остальные.
func TestRunFailsOnBusyPprofAddress(t *testing.T) {
	busy, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to occupy a port: %v", err)
	}
	defer busy.Close()

	cfg := config{runAddr: freeAddr(t), pprofAddr: busy.Addr().String()}

	// Таймаут не даст тесту зависнуть, если сервер всё-таки запустится.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	if err := run(ctx, cfg, slog.New(slog.DiscardHandler)); err == nil {
		t.Error("run() error = nil, want an error for a busy pprof address")
	}
}

// Весь путь запроса агента через настоящие соединения: подпись, gzip, переиспользуемые
// буферы и компрессоры. Детектор гонок запускается не везде, поэтому данные проверяются
// по содержимому: сумма счётчика должна сойтись, а каждый ответ — иметь верную подпись.
func TestRouterConcurrentAgents(t *testing.T) {
	const (
		key      = "secret"
		agents   = 16
		requests = 25
	)

	repo := repository.NewMemStorage()
	l := slog.New(slog.DiscardHandler)

	subject := audit.NewSubject(l)
	defer subject.Close()

	ts := httptest.NewServer(newRouter(routerDeps{storage: repo, auditor: subject, key: key, log: l}))
	defer ts.Close()

	var running sync.WaitGroup

	for a := range agents {
		running.Go(func() {
			for r := range requests {
				value := float64(a*1000 + r)
				delta := int64(1)

				body, err := json.Marshal([]model.Metrics{
					{ID: fmt.Sprintf("Agent%d", a), MType: model.Gauge, Value: &value},
					{ID: "PollCount", MType: model.Counter, Delta: &delta},
				})
				if err != nil {
					t.Errorf("failed to encode batch: %v", err)
					return
				}

				sendSignedBatch(t, ts, body, key)
			}
		})
	}

	running.Wait()

	if got, _, _ := repo.GetCounter(t.Context(), "PollCount"); got != agents*requests {
		t.Errorf("PollCount = %d, want %d", got, agents*requests)
	}

	for a := range agents {
		want := float64(a*1000 + requests - 1)
		if got, _, _ := repo.GetGauge(t.Context(), fmt.Sprintf("Agent%d", a)); got != want {
			t.Errorf("Agent%d = %v, want %v", a, got, want)
		}
	}
}

func sendSignedBatch(t *testing.T, ts *httptest.Server, body []byte, key string) {
	t.Helper()

	var compressed bytes.Buffer

	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write(body); err != nil {
		t.Errorf("failed to compress batch: %v", err)
		return
	}
	if err := zw.Close(); err != nil {
		t.Errorf("failed to close gzip writer: %v", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/updates/", &compressed)
	if err != nil {
		t.Errorf("failed to create request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set(hash.Header, hash.Sign(body, key))

	res, err := ts.Client().Do(req)
	if err != nil {
		t.Errorf("request failed: %v", err)
		return
	}
	defer res.Body.Close()

	// Transport сам добавил Accept-Encoding и распаковал ответ, проверив его CRC32.
	reply, err := io.ReadAll(res.Body)
	if err != nil {
		t.Errorf("failed to read response: %v", err)
		return
	}

	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d: %s", res.StatusCode, http.StatusOK, reply)
	}
	if want := `{"status":"ok"}`; string(reply) != want {
		t.Errorf("response = %q, want %q", reply, want)
	}
	if sign := res.Header.Get(hash.Header); !hash.Valid(reply, key, sign) {
		t.Errorf("response signature %q does not match %q", sign, reply)
	}
}

// Аудит включают ради записи событий, поэтому ошибка в его настройках должна
// останавливать сервер на старте, а не терять каждое следующее событие.
func TestRunFailsOnInvalidAuditConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  config
	}{
		{name: "audit file is a directory", cfg: config{auditFile: t.TempDir()}},
		{name: "audit url is not full", cfg: config{auditURL: "localhost:9090/audit"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.cfg.runAddr = freeAddr(t)

			// Таймаут не даст тесту зависнуть, если сервер всё-таки запустится.
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()

			if err := run(ctx, tt.cfg, slog.New(slog.DiscardHandler)); err == nil {
				t.Error("run() error = nil, want an error for invalid audit config")
			}
		})
	}
}
