package agent

import (
	"bytes"
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

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/update", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("send %s %s: %w", m.MType, m.ID, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send %s %s: %w", m.MType, m.ID, err)
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("send %s %s: unexpected status %s", m.MType, m.ID, resp.Status)
	}
	return nil
}
