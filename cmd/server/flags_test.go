package main

import (
	"flag"
	"os"
	"testing"
)

// Без параметров профилировщик выключен: профили раскрывают внутренности процесса,
// поэтому он включается только явно. Переменная окружения перекрывает флаг.
func TestParseFlagsPprof(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  string
		want string
	}{
		{name: "по умолчанию выключен"},
		{name: "флаг", args: []string{"--pprof-addr", "localhost:6060"}, want: "localhost:6060"},
		{name: "переменная окружения", env: "localhost:7070", want: "localhost:7070"},
		{
			name: "переменная окружения важнее флага",
			args: []string{"--pprof-addr", "localhost:6060"},
			env:  "localhost:7070",
			want: "localhost:7070",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, flags := os.Args, flag.CommandLine
			t.Cleanup(func() { os.Args, flag.CommandLine = args, flags })

			os.Args = append([]string{"server"}, tt.args...)
			flag.CommandLine = flag.NewFlagSet("server", flag.ContinueOnError)

			t.Setenv("PPROF_ADDRESS", tt.env)
			if tt.env == "" {
				if err := os.Unsetenv("PPROF_ADDRESS"); err != nil {
					t.Fatalf("failed to unset PPROF_ADDRESS: %v", err)
				}
			}

			if got := parseFlags().pprofAddr; got != tt.want {
				t.Errorf("pprofAddr = %q, want %q", got, tt.want)
			}
		})
	}
}

// Без параметров аудит выключен, а переменная окружения, как и везде,
// перекрывает значение флага.
func TestParseFlagsAudit(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		env      map[string]string
		wantFile string
		wantURL  string
	}{
		{name: "по умолчанию аудит выключен"},
		{
			name:     "флаги",
			args:     []string{"--audit-file", "audit.log", "--audit-url", "http://localhost:9090/audit"},
			wantFile: "audit.log",
			wantURL:  "http://localhost:9090/audit",
		},
		{
			name:     "переменные окружения",
			env:      map[string]string{"AUDIT_FILE": "audit.log", "AUDIT_URL": "http://localhost:9090/audit"},
			wantFile: "audit.log",
			wantURL:  "http://localhost:9090/audit",
		},
		{
			name: "переменные окружения важнее флагов",
			args: []string{"--audit-file", "flag.log", "--audit-url", "http://localhost:9090/flag"},
			env: map[string]string{
				"AUDIT_FILE": "env.log",
				"AUDIT_URL":  "http://localhost:9090/env",
			},
			wantFile: "env.log",
			wantURL:  "http://localhost:9090/env",
		},
		{
			name:     "только файл",
			args:     []string{"--audit-file", "audit.log"},
			wantFile: "audit.log",
		},
		{
			name:    "только URL",
			args:    []string{"--audit-url", "http://localhost:9090/audit"},
			wantURL: "http://localhost:9090/audit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// parseFlags регистрирует флаги в общем наборе, а повторная
			// регистрация — это паника, поэтому у каждого случая он свой.
			args, flags := os.Args, flag.CommandLine
			t.Cleanup(func() { os.Args, flag.CommandLine = args, flags })

			os.Args = append([]string{"server"}, tt.args...)
			flag.CommandLine = flag.NewFlagSet("server", flag.ContinueOnError)

			// Setenv вернёт исходные значения после теста, а Unsetenv убирает
			// то, что могло достаться из окружения самого теста.
			for _, name := range []string{"AUDIT_FILE", "AUDIT_URL"} {
				t.Setenv(name, "")
				if err := os.Unsetenv(name); err != nil {
					t.Fatalf("failed to unset %s: %v", name, err)
				}
			}
			for name, value := range tt.env {
				t.Setenv(name, value)
			}

			cfg := parseFlags()

			if cfg.auditFile != tt.wantFile {
				t.Errorf("auditFile = %q, want %q", cfg.auditFile, tt.wantFile)
			}
			if cfg.auditURL != tt.wantURL {
				t.Errorf("auditURL = %q, want %q", cfg.auditURL, tt.wantURL)
			}
		})
	}
}
