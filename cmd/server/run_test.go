package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Vortex-art-01/mertic/internal/model"
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
