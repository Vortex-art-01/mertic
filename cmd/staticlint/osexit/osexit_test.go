package osexit_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/Vortex-art-01/mertic/cmd/staticlint/osexit"
)

// TestAnalyzer прогоняет анализатор по пакетам из testdata/src и сверяет
// диагностики с комментариями "// want" в их исходниках: каждая диагностика
// должна быть ожидаемой, а каждое ожидание — исполниться.
func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), osexit.Analyzer,
		"direct",    // os.Exit во всех положениях внутри main
		"renamed",   // пакет os импортирован под другим именем
		"dotimport", // пакет os импортирован через точку
		"indirect",  // косвенные вызовы и локальная переменная os
		"generated", // сгенерированный файл
		"library",   // функция main не в пакете main
		"withtests", // пакет main с тестами и main, созданная go test
	)
}
