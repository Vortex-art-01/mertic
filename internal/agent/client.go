package agent

import (
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/Vortex-art-01/mertic/internal/model"
)

// Client отправляет метрики на сервер сбора метрик по HTTP.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient создаёт клиент для сервера по адресу baseURL,
// например "http://localhost:8080".
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{},
	}
}

func (c *Client) SendGauge(name string, value float64) error {
	return c.send(model.Gauge, name, strconv.FormatFloat(value, 'f', -1, 64))
}

func (c *Client) SendCounter(name string, delta int64) error {
	return c.send(model.Counter, name, strconv.FormatInt(delta, 10))
}

func (c *Client) send(mType, name, value string) error {
	url := fmt.Sprintf("%s/update/%s/%s/%s", c.baseURL, mType, name, value)

	resp, err := c.http.Post(url, "text/plain", http.NoBody)
	if err != nil {
		return fmt.Errorf("send %s %s: %w", mType, name, err)
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("send %s %s: unexpected status %s", mType, name, resp.Status)
	}
	return nil
}
