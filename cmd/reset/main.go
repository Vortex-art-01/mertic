package main

import (
	"bytes"
	"cmp"
	"errors"
	"flag"
	"fmt"
	"go/token"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("reset: ")

	flag.Usage = usage
	flag.Parse()

	if err := run(".", flag.Args(), os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}

func usage() {
	fmt.Fprint(flag.CommandLine.Output(), `Использование: reset [пакеты]

Создаёт методы Reset() для структур с комментарием // generate:reset и
записывает их в файл reset.gen.go того пакета, где объявлена структура.
Пакеты задаются шаблонами, как у go build; без аргументов — все пакеты от
текущего каталога вниз (./...).

Подробности: go doc github.com/Vortex-art-01/mertic/cmd/reset
`)
}

// run создаёт методы Reset для пакетов, которые шаблоны patterns находят из
// каталога dir; без шаблонов — для всех пакетов от dir вниз. Записанные и
// удалённые файлы run перечисляет в stdout, ошибки разметки — в stderr. Если
// найдена хоть одна ошибка, run не меняет ни одного файла.
func run(dir string, patterns []string, stdout, stderr io.Writer) error {
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	changes, problems, err := generate(dir, patterns)
	if err != nil {
		return err
	}

	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(stderr, p)
		}

		return fmt.Errorf("%d error(s) found, no files changed", len(problems))
	}

	return apply(dir, changes, stdout)
}

// change — файл reset.gen.go, который нужно записать или удалить.
type change struct {
	path string // абсолютный путь
	data []byte // новое содержимое; nil — удалить файл
}

// generate выполняет алгоритм утилиты и возвращает файлы, содержимое которых
// должно измениться: файлы, где всё и так актуально, в список не попадают.
// Ошибки разметки, например комментарий generate:reset над типом, который не
// структура, generate возвращает отдельно от ошибок загрузки пакетов, чтобы
// показать пользователю все сразу; если они есть, изменений нет.
func generate(dir string, patterns []string) ([]change, []problem, error) {
	// Шаг 1. Просканировать пакеты от корневого каталога вниз. go/packages
	// разбирает их исходники и проверяет типы: по типу поля генератор решает,
	// как его сбросить.
	fset := token.NewFileSet()

	pkgs, err := loadPackages(dir, patterns, fset)
	if err != nil {
		return nil, nil, err
	}

	g := newGenerator(dir, fset)

	// Шаг 2. Найти в каждом пакете структуры с комментарием generate:reset.
	// Сначала — во всех пакетах: структура может хранить поле с помеченным
	// типом из другого пакета, а у такого типа Reset появится только сейчас.
	units := make([]*unit, 0, len(pkgs))
	for _, pkg := range pkgs {
		if u := g.scan(pkg); u != nil {
			units = append(units, u)
		}
	}

	// Шаги 3 и 4. Создать метод Reset для каждой найденной структуры и собрать
	// все методы пакета в его файл reset.gen.go. Пакет, в котором помеченных
	// структур больше нет, лишается и файла, если его когда-то создала утилита.
	var changes []change
	for _, u := range units {
		if c, ok := g.change(u); ok {
			changes = append(changes, c)
		}
	}

	// Ошибка в одном пакете останавливает всю генерацию: иначе другой пакет
	// мог бы получить вызов Reset у структуры, метод которой так и не создан.
	if problems := g.sortedProblems(); len(problems) > 0 {
		return nil, problems, nil
	}

	return changes, nil, nil
}

// loadPackages загружает пакеты patterns вместе с синтаксическими деревьями и
// типами; позиции всех файлов попадают в fset. Зависимости go/packages читает
// из export data, а не из исходников, поэтому памяти нужно немного даже на всём
// проекте.
//
// Ошибки типов в загруженных пакетах не мешают: код, который вызывает Reset у
// помеченной структуры, не компилируется, пока метод не создан, а утилита как
// раз должна его создать. Тип поля, который определить не удалось, генератор
// встретит сам и сообщит о нём.
func loadPackages(dir string, patterns []string, fset *token.FileSet) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo,
		Dir:  dir,
		Fset: fset,
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("load packages: %w", err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages match %s", strings.Join(patterns, " "))
	}

	slices.SortFunc(pkgs, func(a, b *packages.Package) int {
		return cmp.Compare(a.PkgPath, b.PkgPath)
	})

	return pkgs, nil
}

// apply записывает и удаляет файлы changes и сообщает о каждом в w. Пути в
// сообщениях — относительно каталога dir.
func apply(dir string, changes []change, w io.Writer) error {
	var errs []error

	for _, c := range changes {
		name := displayPath(dir, c.path)

		if c.data == nil {
			if err := os.Remove(c.path); err != nil {
				errs = append(errs, err)
				continue
			}

			fmt.Fprintf(w, "removed %s\n", name)
			continue
		}

		if err := os.WriteFile(c.path, c.data, 0o644); err != nil {
			errs = append(errs, err)
			continue
		}

		fmt.Fprintf(w, "wrote %s\n", name)
	}

	return errors.Join(errs...)
}

// displayPath возвращает путь path относительно каталога dir, если path лежит
// внутри него, и абсолютный путь в остальных случаях.
func displayPath(dir, path string) string {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return path
	}

	rel, err := filepath.Rel(absDir, path)
	if err != nil || !filepath.IsLocal(rel) {
		return path
	}

	return rel
}

// sameContent сравнивает содержимое файлов без учёта окончаний строк: при
// core.autocrlf=true git выдаёт файлы в Windows с CRLF, и переписывать из-за
// этого актуальный файл незачем.
func sameContent(a, b []byte) bool {
	crlf, lf := []byte("\r\n"), []byte("\n")

	return bytes.Equal(bytes.ReplaceAll(a, crlf, lf), bytes.ReplaceAll(b, crlf, lf))
}
