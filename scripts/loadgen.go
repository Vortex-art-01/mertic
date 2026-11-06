//go:build ignore

// Генератор нагрузки для снятия профиля памяти с запущенного сервера: шлёт смесь
// запросов, похожую на настоящую, и после заданного числа запросов снимает профиль,
// не останавливая нагрузку. Число запросов фиксировано, а не время, чтобы профили
// разных версий сервера можно было сравнивать: они описывают один и тот же объём
// работы, а не столько, сколько версия успела сделать.
//
//	go run scripts/loadgen.go -key secret -out profiles/base.pprof
//
// Файл исключён из сборки тегом ignore: это инструмент, а не часть сервиса.
package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Vortex-art-01/mertic/internal/hash"
	"github.com/Vortex-art-01/mertic/internal/model"
)

const (
	// batchVariants — сколько разных пакетов подготовить заранее. Значения в них
	// разные, как у живого агента, а сжимать тело на каждый запрос незачем:
	// генератор отнимал бы процессор у сервера.
	batchVariants = 64

	// customMetrics — метрики, которые приходят не от агента, а отдельными
	// запросами. Вместе с агентскими они образуют непустое хранилище, которое
	// отдают чтения, страница со списком и дамп.
	customMetrics = 256
)

// agentGauges — те же имена, что шлёт настоящий агент.
var agentGauges = []string{
	"Alloc", "BuckHashSys", "Frees", "GCCPUFraction", "GCSys",
	"HeapAlloc", "HeapIdle", "HeapInuse", "HeapObjects", "HeapReleased",
	"HeapSys", "LastGC", "Lookups", "MCacheInuse", "MCacheSys",
	"MSpanInuse", "MSpanSys", "Mallocs", "NextGC", "NumForcedGC",
	"NumGC", "OtherSys", "PauseTotalNs", "StackInuse", "StackSys",
	"Sys", "TotalAlloc", "RandomValue", "TotalMemory", "FreeMemory",
	"CPUutilization1", "CPUutilization2", "CPUutilization3", "CPUutilization4",
	"CPUutilization5", "CPUutilization6", "CPUutilization7", "CPUutilization8",
	"CPUutilization9", "CPUutilization10", "CPUutilization11", "CPUutilization12",
}

// payload — тело запроса агента, сжатое один раз, и подпись несжатого тела.
type payload struct {
	body []byte
	sign string
}

// makeRequest собирает очередной запрос сценария; rng у каждого воркера свой.
type makeRequest func(rng *rand.Rand) (*http.Request, error)

func main() {
	addr := flag.String("addr", "localhost:8080", "адрес сервера")
	pprofAddr := flag.String("pprof", "localhost:6060", "адрес профилировщика сервера")
	key := flag.String("key", "", "ключ подписи: тот же, что у сервера")
	total := flag.Int64("n", 100_000, "после скольких запросов снять профиль")
	workers := flag.Int("c", 32, "число одновременных клиентов")
	out := flag.String("out", "", "файл для профиля памяти")
	flag.Parse()

	if *out == "" {
		log.Fatal("укажите файл для профиля: -out profiles/base.pprof")
	}

	if err := run(*addr, *pprofAddr, *key, *out, *total, *workers); err != nil {
		log.Fatal(err)
	}
}

func run(addr, pprofAddr, key, out string, total int64, workers int) error {
	client := &http.Client{Transport: &http.Transport{
		MaxIdleConns:        workers,
		MaxIdleConnsPerHost: workers,
	}}
	base := "http://" + addr

	batches, err := prepareBatches(key)
	if err != nil {
		return err
	}

	if err := seed(client, base); err != nil {
		return fmt.Errorf("seed storage: %w", err)
	}

	scenarios := buildScenarios(base, batches)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	var (
		sent, failed atomic.Int64
		snapshotErr  error
		snapshotOnce sync.Once
		running      sync.WaitGroup
	)

	start := time.Now()

	for w := range workers {
		running.Go(func() {
			rng := rand.New(rand.NewPCG(uint64(w), 17))

			for ctx.Err() == nil {
				i := sent.Add(1)

				if err := send(client, scenarios[i%int64(len(scenarios))], rng); err != nil {
					failed.Add(1)
					log.Printf("request failed: %v", err)
				}

				// Нагрузка не останавливается, пока профиль снимается: остальные
				// воркеры продолжают слать запросы до возврата из saveProfile.
				if i == total {
					snapshotOnce.Do(func() {
						snapshotErr = saveProfile(pprofAddr, out)
						stop()
					})
				}
			}
		})
	}

	running.Wait()

	elapsed := time.Since(start)
	fmt.Printf("запросов: %d, ошибок: %d, время: %s, запросов в секунду: %.0f\n",
		sent.Load(), failed.Load(), elapsed.Round(time.Millisecond), float64(sent.Load())/elapsed.Seconds())

	if snapshotErr != nil {
		return fmt.Errorf("save profile: %w", snapshotErr)
	}
	if failed.Load() > 0 {
		return fmt.Errorf("%d запросов завершились ошибкой: профиль не годится для сравнения", failed.Load())
	}

	return nil
}

// buildScenarios раскладывает запросы в доли на сотню. Пакеты от агентов — основная
// работа сервера; чтения и одиночные обновления встречаются реже, страницу со
// списком открывает только человек.
func buildScenarios(base string, batches []payload) []makeRequest {
	weighted := []struct {
		share int
		make  makeRequest
	}{
		{share: 80, make: func(rng *rand.Rand) (*http.Request, error) {
			batch := batches[rng.IntN(len(batches))]

			req, err := http.NewRequest(http.MethodPost, base+"/updates/", bytes.NewReader(batch.body))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Content-Encoding", "gzip")
			if batch.sign != "" {
				req.Header.Set(hash.Header, batch.sign)
			}

			return req, nil
		}},
		{share: 6, make: func(rng *rand.Rand) (*http.Request, error) {
			value := rng.Float64() * 1000
			return jsonRequest(base+"/update/", model.Metrics{ID: customName(rng), MType: model.Gauge, Value: &value})
		}},
		{share: 2, make: func(rng *rand.Rand) (*http.Request, error) {
			delta := rng.Int64N(100)
			return jsonRequest(base+"/update/", model.Metrics{ID: customName(rng) + "_total", MType: model.Counter, Delta: &delta})
		}},
		{share: 2, make: func(rng *rand.Rand) (*http.Request, error) {
			return http.NewRequest(http.MethodPost,
				base+"/update/gauge/"+customName(rng)+"/"+strconv.FormatFloat(rng.Float64()*1000, 'f', 3, 64), nil)
		}},
		{share: 5, make: func(rng *rand.Rand) (*http.Request, error) {
			return jsonRequest(base+"/value/", model.Metrics{ID: customName(rng), MType: model.Gauge})
		}},
		{share: 4, make: func(rng *rand.Rand) (*http.Request, error) {
			return http.NewRequest(http.MethodGet, base+"/value/gauge/"+agentGauges[rng.IntN(len(agentGauges))], nil)
		}},
		{share: 1, make: func(*rand.Rand) (*http.Request, error) {
			return http.NewRequest(http.MethodGet, base+"/", nil)
		}},
	}

	scenarios := make([]makeRequest, 0, 100)
	for _, w := range weighted {
		for range w.share {
			scenarios = append(scenarios, w.make)
		}
	}

	// Иначе запросы одного вида шли бы подряд длинными сериями.
	rand.New(rand.NewPCG(1, 2)).Shuffle(len(scenarios), func(i, j int) {
		scenarios[i], scenarios[j] = scenarios[j], scenarios[i]
	})

	return scenarios
}

func customName(rng *rand.Rand) string {
	return "custom_" + strconv.Itoa(rng.IntN(customMetrics))
}

func jsonRequest(url string, m model.Metrics) (*http.Request, error) {
	body, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

// send отправляет запрос через стандартный Transport: как и настоящий агент, он сам
// добавляет Accept-Encoding: gzip и распаковывает ответ.
func send(client *http.Client, build makeRequest, rng *rand.Rand) error {
	req, err := build(rng)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s %s: %s", req.Method, req.URL.Path, resp.Status)
	}

	return nil
}

// prepareBatches готовит пакеты как у агента: gauge со случайными значениями и
// PollCount, тело сжато, подпись считается по несжатому.
func prepareBatches(key string) ([]payload, error) {
	rng := rand.New(rand.NewPCG(3, 4))
	batches := make([]payload, batchVariants)

	for i := range batches {
		metrics := make([]model.Metrics, 0, len(agentGauges)+1)

		for _, name := range agentGauges {
			value := rng.Float64() * 1e9
			metrics = append(metrics, model.Metrics{ID: name, MType: model.Gauge, Value: &value})
		}

		delta := rng.Int64N(10) + 1
		metrics = append(metrics, model.Metrics{ID: "PollCount", MType: model.Counter, Delta: &delta})

		data, err := json.Marshal(metrics)
		if err != nil {
			return nil, err
		}

		var buf bytes.Buffer

		zw := gzip.NewWriter(&buf)
		if _, err := zw.Write(data); err != nil {
			return nil, err
		}
		if err := zw.Close(); err != nil {
			return nil, err
		}

		batches[i].body = buf.Bytes()
		if key != "" {
			batches[i].sign = hash.Sign(data, key)
		}
	}

	return batches, nil
}

// seed наполняет хранилище до начала нагрузки: чтения не должны получать 404.
func seed(client *http.Client, base string) error {
	metrics := make([]model.Metrics, 0, len(agentGauges)+customMetrics)

	value := 1.0
	for _, name := range agentGauges {
		metrics = append(metrics, model.Metrics{ID: name, MType: model.Gauge, Value: &value})
	}
	for i := range customMetrics {
		metrics = append(metrics, model.Metrics{ID: "custom_" + strconv.Itoa(i), MType: model.Gauge, Value: &value})
	}

	body, err := json.Marshal(metrics)
	if err != nil {
		return err
	}

	resp, err := client.Post(base+"/updates/", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}

	return nil
}

// saveProfile снимает профиль кучи. Параметр gc=1 просит сборку мусора перед снятием:
// без него в профиле оказалось бы состояние на момент последней сборки, а она у
// разных версий сервера случается в разное время.
func saveProfile(pprofAddr, out string) error {
	resp, err := http.Get("http://" + pprofAddr + "/debug/pprof/heap?gc=1")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}

	file, err := os.Create(out)
	if err != nil {
		return err
	}

	if _, err := io.Copy(file, resp.Body); err != nil {
		file.Close()
		return err
	}

	return file.Close()
}
