package buildinfo_test

import (
	"testing"

	"github.com/Vortex-art-01/mertic/internal/buildinfo"
)

// Каждое поле заменяется на N/A независимо от остальных: если при сборке
// задали только часть значений, строки всё равно печатаются все три и в
// прежнем порядке.
func TestInfoString(t *testing.T) {
	tests := []struct {
		name string
		info buildinfo.Info
		want string
	}{
		{
			name: "ничего не задано",
			want: "Build version: N/A\nBuild date: N/A\nBuild commit: N/A",
		},
		{
			name: "задано всё",
			info: buildinfo.Info{Version: "v1.0.0", Date: "2026-11-23T12:00:00Z", Commit: "8c6ab07"},
			want: "Build version: v1.0.0\nBuild date: 2026-11-23T12:00:00Z\nBuild commit: 8c6ab07",
		},
		{
			name: "задана только версия",
			info: buildinfo.Info{Version: "v1.0.0"},
			want: "Build version: v1.0.0\nBuild date: N/A\nBuild commit: N/A",
		},
		{
			name: "задана только дата",
			info: buildinfo.Info{Date: "2026-11-23T12:00:00Z"},
			want: "Build version: N/A\nBuild date: 2026-11-23T12:00:00Z\nBuild commit: N/A",
		},
		{
			name: "задан только коммит",
			info: buildinfo.Info{Commit: "8c6ab07"},
			want: "Build version: N/A\nBuild date: N/A\nBuild commit: 8c6ab07",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.info.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}
