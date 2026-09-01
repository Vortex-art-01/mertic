package agent

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/Vortex-art-01/mertic/internal/model"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

// SendBatch отправляет весь пакет одним запросом на POST /updates/.
// Пустой пакет до сервера не доходит: слать нечего.
func (c *Client) SendBatch(metrics []model.Metrics) error {
	if len(metrics) == 0 {
		return nil
	}

	body, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("marshal batch of %d: %w", len(metrics), err)
	}

	if err := c.post("/updates/", body); err != nil {
		return fmt.Errorf("send batch of %d: %w", len(metrics), err)
	}

	return nil
}

// SendGauge и SendCounter отправляют метрику по одной на POST /update.
// Агент ими больше не пользуется, но одиночный API сервера никуда не делся,
// и клиент его по-прежнему умеет.
func (c *Client) SendGauge(name string, value float64) error {
	return c.send(model.Metrics{ID: name, MType: model.Gauge, Value: &value})
}

func (c *Client) SendCounter(name string, delta int64) error {
	return c.send(model.Metrics{ID: name, MType: model.Counter, Delta: &delta})
}

func (c *Client) send(m model.Metrics) error {
	body, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal %s %s: %w", m.MType, m.ID, err)
	}

	if err := c.post("/update", body); err != nil {
		return fmt.Errorf("send %s %s: %w", m.MType, m.ID, err)
	}

	return nil
}

func gzipped(data []byte) ([]byte, error) {
	var buf bytes.Buffer

	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// post кладёт тело в запрос сжатым: сервер понимает gzip, а метрики жмутся
// хорошо — пакетом тем более.
func (c *Client) post(path string, body []byte) error {
	compressed, err := gzipped(body)
	if err != nil {
		return fmt.Errorf("compress: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(compressed))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %s", resp.Status)
	}

	return nil
}
