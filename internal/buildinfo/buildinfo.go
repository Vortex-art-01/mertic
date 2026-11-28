// Package buildinfo формирует сведения о сборке, которые сервер и агент
// печатают при старте: версию, дату и коммит.
//
// Сами значения хранятся в переменных buildVersion, buildDate и buildCommit
// пакета main каждого приложения. Задаёт их линкер флагом -X, так что версию
// можно поменять, не трогая код:
//
//	go build -ldflags "-X main.buildVersion=v1.0.0 -X main.buildDate=2026-11-23T12:00:00Z -X main.buildCommit=8c6ab07"
//
// Без флага переменная остаётся пустой, например при go run, и вместо её
// значения печатается N/A.
package buildinfo

import (
	"cmp"
	"fmt"
)

// notAvailable печатается вместо значения, которое при сборке не задали.
const notAvailable = "N/A"

// Info — сведения о сборке. Пустое поле значит, что значение при сборке не
// задали.
type Info struct {
	Version string
	Date    string
	Commit  string
}

// String возвращает сведения о сборке так, как их печатает приложение, — по
// строке на поле, без перевода строки в конце:
//
//	Build version: v1.0.0
//	Build date: 2026-11-23T12:00:00Z
//	Build commit: 8c6ab07
//
// Пустое поле выводится как N/A.
func (i Info) String() string {
	return fmt.Sprintf("Build version: %s\nBuild date: %s\nBuild commit: %s",
		cmp.Or(i.Version, notAvailable),
		cmp.Or(i.Date, notAvailable),
		cmp.Or(i.Commit, notAvailable))
}
