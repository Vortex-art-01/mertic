package main

import (
	"strings"
	"testing"
)

// Опечатка в конфигурации должна останавливать агента на старте, а не
// превращаться в молчаливое значение по умолчанию.
func TestValidateFlags(t *testing.T) {
	tests := []struct {
		name           string
		reportInterval int
		pollInterval   int
		rateLimit      int
		wantErr        string
	}{
		{"значения по умолчанию", 10, 2, 1, ""},
		{"нулевая частота отправки", 0, 2, 1, "REPORT_INTERVAL"},
		{"нулевая частота опроса", 10, 0, 1, "POLL_INTERVAL"},
		{"нулевой лимит запросов", 10, 2, 0, "RATE_LIMIT"},
		{"отрицательный лимит запросов", 10, 2, -1, "RATE_LIMIT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagReportInterval, flagPollInterval, flagRateLimit = tt.reportInterval, tt.pollInterval, tt.rateLimit

			err := validateFlags()

			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("validateFlags() = %v, ошибки быть не должно", err)
			case tt.wantErr != "" && err == nil:
				t.Fatalf("validateFlags() = nil, ожидалась ошибка про %s", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Errorf("validateFlags() = %q, в сообщении нет %s", err, tt.wantErr)
			}
		})
	}
}

// Переменная окружения перекрывает значение флага, а мусор в ней
// возвращает ошибку с именем переменной.
func TestEnvHelpers(t *testing.T) {
	t.Run("строка", func(t *testing.T) {
		got := "localhost:8080"

		envString("TEST_ADDRESS", &got)
		if got != "localhost:8080" {
			t.Errorf("незаданная переменная изменила значение на %q", got)
		}

		t.Setenv("TEST_ADDRESS", "example.com:9090")

		envString("TEST_ADDRESS", &got)
		if got != "example.com:9090" {
			t.Errorf("envString() = %q, want %q", got, "example.com:9090")
		}
	})

	t.Run("число", func(t *testing.T) {
		got := 1

		if err := envInt("TEST_RATE_LIMIT", &got); err != nil || got != 1 {
			t.Errorf("незаданная переменная: got = %d, err = %v", got, err)
		}

		t.Setenv("TEST_RATE_LIMIT", "5")

		if err := envInt("TEST_RATE_LIMIT", &got); err != nil || got != 5 {
			t.Errorf("envInt() = %d, err = %v, want 5 без ошибки", got, err)
		}

		t.Setenv("TEST_RATE_LIMIT", "пять")

		err := envInt("TEST_RATE_LIMIT", &got)
		if err == nil {
			t.Fatal("envInt() = nil, ожидалась ошибка разбора")
		}
		if !strings.Contains(err.Error(), "TEST_RATE_LIMIT") {
			t.Errorf("envInt() = %q, в сообщении нет имени переменной", err)
		}
		if got != 5 {
			t.Errorf("после ошибки значение стало %d, должно остаться 5", got)
		}
	})
}
