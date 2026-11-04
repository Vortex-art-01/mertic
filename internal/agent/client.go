package agent

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Vortex-art-01/mertic/internal/hash"
	"github.com/Vortex-art-01/mertic/internal/model"
	"github.com/Vortex-art-01/mertic/internal/retry"
)

type Client struct {
	baseURL    string
	key        string
	httpClient *http.Client
	delays     []time.Duration
}

func NewClient(baseURL, key string) *Client {
	return &Client{
		baseURL:    baseURL,
		key:        key,
		httpClient: &http.Client{},
		delays:     retry.DefaultDelays(),
	}
}

func (c *Client) SendBatch(ctx context.Context, metrics []model.Metrics) error {
	if len(metrics) == 0 {
		return nil
	}

	body, err := json.Marshal(metrics)
	if err != nil {
		return fmt.Errorf("marshal batch of %d: %w", len(metrics), err)
	}

	if err := c.post(ctx, "/updates/", body); err != nil {
		return fmt.Errorf("send batch of %d: %w", len(metrics), err)
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

func (c *Client) post(ctx context.Context, path string, body []byte) error {
	var sign string
	if c.key != "" {
		sign = hash.Sign(body, c.key)
	}

	compressed, err := gzipped(body)
	if err != nil {
		return fmt.Errorf("compress: %w", err)
	}

	return retry.DoWith(ctx, c.delays, retriable, func() error {
		return c.do(ctx, path, compressed, sign)
	})
}

func (c *Client) do(ctx context.Context, path string, body []byte, sign string) error {
	url := c.baseURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request to %s: %w", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")

	if sign != "" {
		req.Header.Set(hash.Header, sign)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &TransportError{URL: url, Err: err}
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return &StatusError{Code: resp.StatusCode, Status: resp.Status}
	}

	return nil
}
