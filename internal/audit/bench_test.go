package audit

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

// noopObserver не задерживает очередь: в замер Notify попадает только постановка
// события в неё.
type noopObserver struct{}

func (noopObserver) Update(context.Context, Event) error { return nil }

// benchEvent — событие о пакете метрик от агента.
func benchEvent() Event {
	names := make([]string, 44)
	for i := range names {
		names[i] = fmt.Sprintf("Metric%04d", i)
	}

	return Event{TS: 1793784813, Metrics: names, IPAddress: "192.168.0.42"}
}

func BenchmarkNewEvent(b *testing.B) {
	req := httptest.NewRequest(http.MethodPost, "/updates/", nil)
	names := benchEvent().Metrics

	b.ReportAllocs()
	for b.Loop() {
		_ = NewEvent(req, names...)
	}
}

func BenchmarkSubjectNotify(b *testing.B) {
	event := benchEvent()
	l := slog.New(slog.DiscardHandler)

	// Аудит выключен: уведомлять некого, и так сервер работает по умолчанию.
	b.Run("no_observers", func(b *testing.B) {
		subject := NewSubject(l)
		defer subject.Close()

		b.ReportAllocs()
		for b.Loop() {
			subject.Notify(event)
		}
	})

	b.Run("one_observer", func(b *testing.B) {
		subject := NewSubject(l)
		defer subject.Close()

		subject.Register(noopObserver{})

		b.ReportAllocs()
		for b.Loop() {
			subject.Notify(event)
		}
	})
}

func BenchmarkFileUpdate(b *testing.B) {
	ctx := context.Background()
	event := benchEvent()

	file, err := NewFile(filepath.Join(b.TempDir(), "audit.log"))
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if err := file.Update(ctx, event); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRemoteUpdate(b *testing.B) {
	ctx := context.Background()
	event := benchEvent()

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
	}))
	defer srv.Close()

	remote, err := NewRemote(srv.URL)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if err := remote.Update(ctx, event); err != nil {
			b.Fatal(err)
		}
	}
}
