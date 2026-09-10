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
		reportInterval int64
		pollInterval   int64
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
