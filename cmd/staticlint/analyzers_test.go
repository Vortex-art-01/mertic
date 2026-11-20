package main

import (
	"go/parser"
	"go/token"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

// Набор проверяет и сама multichecker.Main, но только при запуске: с ошибкой
// в наборе анализатор не стартует вовсе. Тест находит её раньше.
func TestAnalyzersAreValid(t *testing.T) {
	all := analyzers()

	if err := analysis.Validate(all); err != nil {
		t.Fatalf("analysis.Validate() error = %v", err)
	}

	// Имя анализатора становится флагом -ИМЯ, а объявить один флаг дважды
	// нельзя: пакет flag паникует.
	seen := make(map[string]bool, len(all))
	for _, a := range all {
		if seen[a.Name] {
			t.Errorf("analyzer name %q is used more than once", a.Name)
		}

		seen[a.Name] = true
	}
}

// docEntry — строка таблицы анализаторов в документации пакета: строка блока
// кода, в которой за именем идут хотя бы два пробела и описание. Команды и
// флаги в других блоках под этот вид не подходят.
var docEntry = regexp.MustCompile(`^\t([A-Za-z][A-Za-z0-9]*) {2,}\S`)

// Документация обязана описывать каждый анализатор, а состав набора меняется
// и без правок в этом пакете: достаточно обновить staticcheck или x/tools.
// Тест не даёт описанию разойтись с набором ни в одну из сторон.
func TestDocDescribesEveryAnalyzer(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "doc.go", nil, parser.ParseComments|parser.PackageClauseOnly)
	if err != nil {
		t.Fatalf("failed to parse doc.go: %v", err)
	}

	described := make(map[string]bool)

	for line := range strings.Lines(file.Doc.Text()) {
		match := docEntry.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		name := match[1]
		if described[name] {
			t.Errorf("doc.go describes %s more than once", name)
		}

		described[name] = true
	}

	for _, a := range analyzers() {
		if !described[a.Name] {
			t.Errorf("analyzer %s is not described in doc.go", a.Name)
		}

		delete(described, a.Name)
	}

	for _, name := range slices.Sorted(maps.Keys(described)) {
		t.Errorf("doc.go describes %s, but there is no such analyzer", name)
	}
}
