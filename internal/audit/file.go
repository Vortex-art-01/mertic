package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// File дописывает события в конец файла, по одному JSON на строку.
type File struct {
	path string
}

// NewFile сразу проверяет, что в файл можно писать: ошибка в пути должна
// останавливать сервер на старте, а не терять каждое следующее событие.
func NewFile(path string) (*File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create directory %s: %w", dir, err)
	}

	file, err := openForAppend(path)
	if err != nil {
		return nil, err
	}

	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close %s: %w", path, err)
	}

	return &File{path: path}, nil
}

// Update открывает файл заново на каждое событие: агенты шлют пакеты раз в
// несколько секунд, так что это недорого, зато внешняя ротация лога работает
// без перезапуска сервера. Контекст не нужен: запись на диск быстрая, и
// события успеют попасть в файл, даже если при остановке доставку уже отменили.
func (f *File) Update(_ context.Context, event Event) error {
	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}

	file, err := openForAppend(f.path)
	if err != nil {
		return err
	}

	if _, err := file.Write(append(line, '\n')); err != nil {
		file.Close()
		return fmt.Errorf("write %s: %w", f.path, err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", f.path, err)
	}

	return nil
}

func openForAppend(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	return file, nil
}
