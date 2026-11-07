package middleware_test

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"

	"github.com/Vortex-art-01/mertic/internal/hash"
	"github.com/Vortex-art-01/mertic/internal/middleware"
)

// Клиент сжимает тело запроса и сообщает об этом заголовком Content-Encoding, а
// заголовком Accept-Encoding просит сжатый ответ. Хендлер за мидлварью читает и
// пишет несжатые данные.
func ExampleWithGzip() {
	router := chi.NewRouter()
	router.Use(middleware.WithGzip)
	router.Post("/echo", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body) // уже распакованное тело
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})

	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, _ = zw.Write([]byte(`{"id":"Alloc","type":"gauge","value":1024}`))
	_ = zw.Close()

	req := httptest.NewRequest(http.MethodPost, "/echo", &compressed)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("Accept-Encoding", "gzip")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	fmt.Println(rec.Code, rec.Header().Get("Content-Encoding"))

	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		log.Fatal(err)
	}

	body, err := io.ReadAll(zr)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(body))

	// Output:
	// 200 gzip
	// {"id":"Alloc","type":"gauge","value":1024}
}

// Клиент с тем же ключом, что у сервера, подписывает тело запроса и передаёт
// подпись в заголовке HashSHA256, а сервер подписывает ответ. Запрос без подписи
// проходит без проверки, с неверной подписью — отклоняется.
func ExampleWithHash() {
	const key = "secret"

	router := chi.NewRouter()
	router.Use(middleware.WithHash(key))
	router.Post("/echo", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, r.Body)
	})

	body := []byte(`[{"id":"Alloc","type":"gauge","value":1024}]`)

	for _, request := range []struct {
		name string
		sign string
	}{
		{name: "signed", sign: hash.Sign(body, key)},
		{name: "unsigned", sign: ""},
		{name: "wrong key", sign: hash.Sign(body, "wrong")},
	} {
		req := httptest.NewRequest(http.MethodPost, "/echo", bytes.NewReader(body))
		if request.sign != "" {
			req.Header.Set(hash.Header, request.sign)
		}

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		signed := hash.Valid(rec.Body.Bytes(), key, rec.Header().Get(hash.Header))
		fmt.Printf("%s: %d, response signed: %t\n", request.name, rec.Code, signed)
	}

	// Output:
	// signed: 200, response signed: true
	// unsigned: 200, response signed: true
	// wrong key: 400, response signed: false
}
