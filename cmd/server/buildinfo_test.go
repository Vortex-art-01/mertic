package main

import (
	"testing"

	"github.com/Vortex-art-01/mertic/internal/buildinfo/buildinfotest"
)

// Сервер, собранный с -ldflags и без них, при старте печатает сведения о
// сборке раньше всего остального.
func TestBuildInfo(t *testing.T) {
	buildinfotest.TestBinary(t, ".")
}
