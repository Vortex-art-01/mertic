package gzippool

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"sync"
	"testing"
)

// drain убирает свободные компрессоры, оставшиеся от других тестов: список общий.
func drain() {
	for {
		select {
		case <-idle:
		default:
			return
		}
	}
}

func compress(t testing.TB, buf *bytes.Buffer, data string) {
	t.Helper()

	zw := Get(buf)
	defer Put(zw)

	if _, err := zw.Write([]byte(data)); err != nil {
		t.Errorf("failed to write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Errorf("failed to close: %v", err)
	}
}

func decompress(t testing.TB, data []byte) string {
	t.Helper()

	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Errorf("failed to open gzip stream: %v", err)
		return ""
	}
	defer zr.Close()

	got, err := io.ReadAll(zr)
	if err != nil {
		t.Errorf("failed to read gzip stream: %v", err)
	}

	return string(got)
}

// Второй поток не должен содержать ничего от первого: компрессор тот же самый.
func TestReusedWriterProducesCleanStream(t *testing.T) {
	drain()

	for _, want := range []string{"первый поток", "второй поток, уже другой"} {
		var buf bytes.Buffer

		compress(t, &buf, want)

		if got := decompress(t, buf.Bytes()); got != want {
			t.Errorf("decompressed %q, want %q", got, want)
		}
	}

	if len(idle) != 1 {
		t.Errorf("got %d idle writers, want the same one reused", len(idle))
	}
}

// Сколько бы компрессоров ни вернули, в памяти остаётся не больше maxIdle.
func TestPutKeepsAtMostMaxIdle(t *testing.T) {
	drain()

	writers := make([]*gzip.Writer, 0, maxIdle+2)
	for range cap(writers) {
		writers = append(writers, Get(io.Discard))
	}

	for _, zw := range writers {
		Put(zw)
	}

	if len(idle) != maxIdle {
		t.Errorf("got %d idle writers, want %d", len(idle), maxIdle)
	}
}

// Свободный компрессор не должен удерживать прошлого получателя.
func TestPutDetachesWriter(t *testing.T) {
	drain()

	var buf bytes.Buffer

	zw := Get(&buf)
	Put(zw)

	if _, err := zw.Write([]byte("уже никому")); err != nil {
		t.Fatalf("failed to write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	if buf.Len() != 0 {
		t.Errorf("released writer wrote %d bytes to its former destination", buf.Len())
	}
}

// Один компрессор не должен достаться двум потокам одновременно.
func TestConcurrentStreamsDoNotMix(t *testing.T) {
	var running sync.WaitGroup

	for g := range 16 {
		running.Go(func() {
			for i := range 50 {
				var buf bytes.Buffer

				want := fmt.Sprintf("горутина %d, поток %d", g, i)
				compress(t, &buf, want)

				if got := decompress(t, buf.Bytes()); got != want {
					t.Errorf("decompressed %q, want %q", got, want)
				}
			}
		})
	}

	running.Wait()
}
