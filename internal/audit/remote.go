package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// Remote отправляет события на удалённый сервер методом POST.
type Remote struct {
	url        string
	httpClient *http.Client
}

// NewRemote сразу проверяет адрес: опечатка в нём должна останавливать сервер
// на старте, а не терять каждое следующее событие.
func NewRemote(rawURL string) (*Remote, error) {
	u, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse audit url: %w", withoutURL(err))
	}

	// ParseRequestURI примет и localhost:8080/audit — как URL со схемой
	// localhost, — поэтому схему и хост приходится проверять отдельно.
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("audit url must be a full http or https url")
	}

	return &Remote{url: rawURL, httpClient: &http.Client{}}, nil
}

// Update отправляет событие в теле POST-запроса с Content-Type: application/json.
// Ответ с кодом вне 2xx считается ошибкой. Адреса в ошибке нет: ошибки уходят в
// лог, а в URL может быть токен доступа.
func (r *Remote) Update(ctx context.Context, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", withoutURL(err))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send event: %w", withoutURL(err))
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("audit server responded with %s", resp.Status)
	}

	return nil
}

// withoutURL убирает адрес из ошибки net/url: ошибки уходят в лог, а в полном
// URL может быть токен доступа.
func withoutURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}

	return err
}
