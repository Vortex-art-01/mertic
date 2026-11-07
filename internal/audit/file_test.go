package audit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}

	return string(data)
}

func TestFileAppendsEventsOnNewLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	existing := `{"ts":1,"metrics":["Old"],"ip_address":"10.0.0.1"}` + "\n"

	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("failed to prepare file: %v", err)
	}

	file, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}

	events := []Event{
		{TS: 2, Metrics: []string{"Alloc", "Frees"}, IPAddress: "192.168.0.42"},
		{TS: 3, Metrics: []string{"PollCount"}, IPAddress: "192.168.0.43"},
	}
	for _, event := range events {
		if err := file.Update(t.Context(), event); err != nil {
			t.Fatalf("Update() error = %v", err)
		}
	}

	want := existing +
		`{"ts":2,"metrics":["Alloc","Frees"],"ip_address":"192.168.0.42"}` + "\n" +
		`{"ts":3,"metrics":["PollCount"],"ip_address":"192.168.0.43"}` + "\n"

	if got := readFile(t, path); got != want {
		t.Errorf("file content:\n%s\nwant:\n%s", got, want)
	}
}

func TestNewFileCreatesMissingDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "audit", "audit.log")

	file, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}

	if err := file.Update(t.Context(), Event{TS: 1}); err != nil {
		t.Errorf("Update() error = %v", err)
	}
}

// Каталог вместо файла — ошибка конфигурации, и видно её должно быть сразу.
func TestNewFileRejectsUnusablePath(t *testing.T) {
	if _, err := NewFile(t.TempDir()); err == nil {
		t.Error("NewFile() error = nil, want an error for a directory")
	}
}

// Ротация лога переносит файл, а следующее событие должно создать новый.
func TestFileUpdateFollowsRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")
	rotated := filepath.Join(dir, "audit.log.1")

	file, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}
	if err := file.Update(t.Context(), Event{TS: 1, Metrics: []string{"Old"}}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if err := os.Rename(path, rotated); err != nil {
		t.Fatalf("failed to rotate file: %v", err)
	}

	if err := file.Update(t.Context(), Event{TS: 2, Metrics: []string{"New"}}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	wantOld := `{"ts":1,"metrics":["Old"],"ip_address":""}` + "\n"
	if got := readFile(t, rotated); got != wantOld {
		t.Errorf("rotated file = %q, want %q", got, wantOld)
	}

	wantNew := `{"ts":2,"metrics":["New"],"ip_address":""}` + "\n"
	if got := readFile(t, path); got != wantNew {
		t.Errorf("new file = %q, want %q", got, wantNew)
	}
}

// Subject отменяет доставку при остановке сервера, но события из очереди
// файла к этому моменту ещё можно успеть записать.
func TestFileUpdateIgnoresCancelledContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")

	file, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := file.Update(ctx, Event{TS: 1}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if readFile(t, path) == "" {
		t.Error("event was not written after the context was cancelled")
	}
}

func TestFileUpdateReportsFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "logs")

	file, err := NewFile(filepath.Join(dir, "audit.log"))
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}

	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("failed to remove directory: %v", err)
	}

	if err := file.Update(t.Context(), Event{TS: 1}); err == nil {
		t.Error("Update() error = nil, want an error for a missing directory")
	}
}
