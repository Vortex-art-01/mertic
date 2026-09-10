package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/dump"
	"github.com/Vortex-art-01/mertic/internal/model"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

// В синхронном режиме дамп должен появиться на диске сразу после запроса,
// без ожидания тика.
func TestSyncStorageWritesDumpOnUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics-db.json")
	l := slog.New(slog.DiscardHandler)

	storage, dumps := dump.Attach(t.Context(), repository.NewMemStorage(),
		dump.Config{Path: path}, l)
	defer dumps.Close()

	ts := httptest.NewServer(newRouter(routerDeps{storage: storage, log: l}))
	defer ts.Close()

	res, err := ts.Client().Post(ts.URL+"/update", "application/json",
		strings.NewReader(`{"id":"Alloc","type":"gauge","value":42.5}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d, want %d", res.StatusCode, http.StatusOK)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("dump was not written: %v", err)
	}

	var metrics []model.Metrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		t.Fatalf("dump is not valid json: %v\n%s", err, data)
	}
	if len(metrics) != 1 || metrics[0].ID != "Alloc" ||
		metrics[0].Value == nil || *metrics[0].Value != 42.5 {
		t.Errorf("dump = %s, want a single Alloc gauge with value 42.5", data)
	}
}

// Пакет тоже должен попасть на диск сразу — и одним сбросом на весь пакет,
// а не на каждую метрику.
func TestSyncStorageWritesDumpOnBatchUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics-db.json")
	l := slog.New(slog.DiscardHandler)

	storage, dumps := dump.Attach(t.Context(), repository.NewMemStorage(),
		dump.Config{Path: path}, l)
	defer dumps.Close()

	ts := httptest.NewServer(newRouter(routerDeps{storage: storage, log: l}))
	defer ts.Close()

	res, err := ts.Client().Post(ts.URL+"/updates/", "application/json",
		strings.NewReader(`[{"id":"Alloc","type":"gauge","value":42.5},
			{"id":"PollCount","type":"counter","delta":7}]`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("updates status = %d, want %d", res.StatusCode, http.StatusOK)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("dump was not written: %v", err)
	}

	var metrics []model.Metrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		t.Fatalf("dump is not valid json: %v\n%s", err, data)
	}
	if len(metrics) != 2 {
		t.Fatalf("dump has %d metrics, want 2:\n%s", len(metrics), data)
	}
	// Дамп упорядочен: сначала gauge, затем counter.
	if metrics[0].ID != "Alloc" || metrics[0].Value == nil || *metrics[0].Value != 42.5 {
		t.Errorf("dump[0] = %+v, want Alloc gauge with value 42.5", metrics[0])
	}
	if metrics[1].ID != "PollCount" || metrics[1].Delta == nil || *metrics[1].Delta != 7 {
		t.Errorf("dump[1] = %+v, want PollCount counter with delta 7", metrics[1])
	}
}
