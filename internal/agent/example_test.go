package agent_test

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/Vortex-art-01/mertic/internal/agent"
	"github.com/Vortex-art-01/mertic/internal/hash"
	"github.com/Vortex-art-01/mertic/internal/model"
)

// Клиент отправляет пакет одним запросом POST /updates/: JSON-массив метрик,
// сжатый gzip и подписанный ключом. Тестовый сервер печатает, что к нему пришло.
func ExampleClient_SendBatch() {
	const key = "secret"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.Method, r.URL.Path)
		fmt.Println("Content-Type:", r.Header.Get("Content-Type"))
		fmt.Println("Content-Encoding:", r.Header.Get("Content-Encoding"))

		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, "invalid gzip body", http.StatusBadRequest)
			return
		}

		body, err := io.ReadAll(zr)
		if err != nil {
			http.Error(w, "invalid gzip body", http.StatusBadRequest)
			return
		}

		fmt.Println(string(body))
		fmt.Println("signed:", hash.Valid(body, key, r.Header.Get(hash.Header)))
	}))
	defer srv.Close()

	alloc, pollCount := 1024.0, int64(5)

	client := agent.NewClient(srv.URL, key)

	err := client.SendBatch(context.Background(), []model.Metrics{
		{ID: "Alloc", MType: model.Gauge, Value: &alloc},
		{ID: "PollCount", MType: model.Counter, Delta: &pollCount},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Output:
	// POST /updates/
	// Content-Type: application/json
	// Content-Encoding: gzip
	// [{"id":"Alloc","type":"gauge","value":1024},{"id":"PollCount","type":"counter","delta":5}]
	// signed: true
}
