package index_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Vortex-art-01/mertic/internal/handler/index"
	"github.com/Vortex-art-01/mertic/internal/repository"
)

// На странице сначала идут gauge, затем counter, внутри группы — по имени. Здесь
// напечатаны только строки списка; вокруг них — обычная HTML-разметка.
func ExampleNew() {
	ctx := context.Background()

	storage := repository.NewMemStorage()
	_ = storage.SaveGauge(ctx, "Temperature", 36.6)
	_ = storage.SaveGauge(ctx, "Alloc", 1024.5)
	_ = storage.AddCounter(ctx, "PollCount", 5)

	router := chi.NewRouter()
	router.Get("/", index.New(storage, slog.New(slog.DiscardHandler)))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	fmt.Println(rec.Code, rec.Header().Get("Content-Type"))

	for line := range strings.Lines(rec.Body.String()) {
		if strings.Contains(line, "<li>") {
			fmt.Println(strings.TrimSpace(line))
		}
	}

	// Output:
	// 200 text/html; charset=utf-8
	// <li>Alloc = 1024.5</li>
	// <li>Temperature = 36.6</li>
	// <li>PollCount = 5</li>
}
