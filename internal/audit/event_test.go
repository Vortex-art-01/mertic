package audit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

// Формат события — контракт с приёмниками, поэтому сверяем его дословно.
func TestEventJSON(t *testing.T) {
	data, err := json.Marshal(Event{
		TS:        12345678,
		Metrics:   []string{"Alloc", "Frees"},
		IPAddress: "192.168.0.42",
	})
	if err != nil {
		t.Fatalf("failed to encode event: %v", err)
	}

	want := `{"ts":12345678,"metrics":["Alloc","Frees"],"ip_address":"192.168.0.42"}`
	if string(data) != want {
		t.Errorf("event = %s, want %s", data, want)
	}
}

func TestNewEvent(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/updates/", nil)
	req.RemoteAddr = "192.168.0.42:5555"

	before := time.Now().Unix()
	event := NewEvent(req, "Alloc", "Frees")
	after := time.Now().Unix()

	if event.TS < before || event.TS > after {
		t.Errorf("ts = %d, want between %d and %d", event.TS, before, after)
	}
	if !slices.Equal(event.Metrics, []string{"Alloc", "Frees"}) {
		t.Errorf("metrics = %v, want [Alloc Frees]", event.Metrics)
	}
	if event.IPAddress != "192.168.0.42" {
		t.Errorf("ip_address = %q, want %q", event.IPAddress, "192.168.0.42")
	}
}

func TestNewEventIPAddress(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{"ipv4", "192.168.0.42:5555", "192.168.0.42"},
		{"ipv6", "[2001:db8::1]:5555", "2001:db8::1"},
		{"address without port", "192.168.0.42", "192.168.0.42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/updates/", nil)
			req.RemoteAddr = tt.remoteAddr

			if got := NewEvent(req, "Alloc").IPAddress; got != tt.want {
				t.Errorf("ip_address = %q, want %q", got, tt.want)
			}
		})
	}
}

// Заголовки клиент подставляет сам: подделать через них адрес в аудите нельзя.
func TestNewEventIgnoresForwardedHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/updates/", nil)
	req.RemoteAddr = "192.168.0.42:5555"
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	req.Header.Set("X-Real-IP", "10.0.0.2")

	if got := NewEvent(req, "Alloc").IPAddress; got != "192.168.0.42" {
		t.Errorf("ip_address = %q, want %q", got, "192.168.0.42")
	}
}
