// Package buildinfotest проверяет, что приложение печатает при старте сведения
// о сборке. Им пользуются тесты сервера и агента.
package buildinfotest

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestBinary собирает пакет main pkg дважды, без флагов линкера и с ними, и
// проверяет, что бинарник первыми строками печатает в stdout сведения о
// сборке: значения из -ldflags, а если их не задали — N/A.
//
// Проверять приходится собранный бинарник: -X с опечаткой в имени переменной
// линкер молча пропускает, и такая ошибка видна только по выводу приложения.
// Запускается он с флагом -h: пакет flag печатает справку в stderr и сразу
// завершает процесс, так что в stdout остаётся лишь то, что приложение вывело
// до разбора флагов.
//
// Каждая сборка занимает секунду-другую, поэтому с -short проверка
// пропускается.
func TestBinary(t *testing.T, pkg string) {
	t.Helper()

	if testing.Short() {
		t.Skip("сборка бинарника пропускается в режиме -short")
	}

	tests := []struct {
		name    string
		ldflags string
		want    string
	}{
		{
			name: "без -ldflags",
			want: "Build version: N/A\nBuild date: N/A\nBuild commit: N/A\n",
		},
		{
			name: "с -ldflags",
			ldflags: "-X main.buildVersion=v1.2.3 " +
				"-X main.buildDate=2026-11-23T12:00:00Z " +
				"-X main.buildCommit=abc1234",
			want: "Build version: v1.2.3\nBuild date: 2026-11-23T12:00:00Z\nBuild commit: abc1234\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bin := build(t, pkg, tt.ldflags)

			out, err := exec.Command(bin, "-h").Output()
			if err != nil {
				t.Fatalf("%s -h: %v", pkg, err)
			}

			if string(out) != tt.want {
				t.Errorf("stdout = %q, want %q", out, tt.want)
			}
		})
	}
}

// build собирает пакет main pkg с флагами линкера ldflags и возвращает путь к
// бинарнику.
func build(t *testing.T, pkg, ldflags string) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "app")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	// По умолчанию go build вшивает в бинарник состояние git и падает, если
	// git нет или он не доверяет каталогу, как в контейнере CI.
	cmd := exec.Command("go", "build", "-buildvcs=false", "-ldflags", ldflags, "-o", bin, pkg)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pkg, err, out)
	}

	return bin
}
