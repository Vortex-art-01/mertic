package agent

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
)

func TestRetriableStatuses(t *testing.T) {
	tests := []struct {
		name string
		code int
		want bool
	}{
		{"сервер сломался", http.StatusInternalServerError, true},
		{"сервер не отвечает", http.StatusBadGateway, true},
		{"сервер занят", http.StatusServiceUnavailable, true},
		{"слишком много запросов", http.StatusTooManyRequests, true},
		{"неверный запрос", http.StatusBadRequest, false},
		{"нет такого адреса", http.StatusNotFound, false},
		{"метрика не найдена", http.StatusNotFound, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &StatusError{Code: tt.code, Status: http.StatusText(tt.code)}

			if got := retriable(err); got != tt.want {
				t.Errorf("retriable(%d) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

// Ошибка доезжает до классификатора обёрнутой в контекст отправки.
func TestRetriableUnwrapsError(t *testing.T) {
	transport := &TransportError{
		URL: "http://localhost:8080/updates/",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
	}
	err := fmt.Errorf("send batch of 3: %w", transport)

	if !retriable(err) {
		t.Errorf("retriable(%v) = false, want true", err)
	}

	// Обёртка не прячет исходную ошибку.
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		t.Errorf("errors.As(%v, *net.OpError) = false, want true", err)
	}
}

// Собственные промахи повторять бессмысленно: тело не сожмётся, а запрос
// не соберётся и со второй попытки.
func TestRetriableIgnoresOtherErrors(t *testing.T) {
	for _, err := range []error{nil, errors.New("compress: unexpected EOF")} {
		if retriable(err) {
			t.Errorf("retriable(%v) = true, want false", err)
		}
	}
}
