package main

import (
	"bytes"
	"flag"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "переписать эталоны reset.gen.go.golden в testdata")

// TestGenerate прогоняет утилиту по модулям из testdata и сверяет каждый
// получившийся reset.gen.go с эталоном reset.gen.go.golden из того же
// каталога. Эталон есть ровно там, где после запуска должен лежать файл:
// удалённый файл эталона не имеет, а чужой, нетронутый, совпадает со своим.
//
// Затем утилита запускается ещё раз и не должна ничего менять, а в модуле
// запускаются его собственные тесты: сгенерированный код компилируется и
// сбрасывает поля так, как описано в документации.
//
// Эталоны переписывает go test ./cmd/reset -update.
func TestGenerate(t *testing.T) {
	cases := []string{
		"example", // структура из условия задачи
		"kinds",   // поля всех видов, которые различает генератор
		"multi",   // несколько пакетов, группы, старые и чужие файлы, конфликты имён
	}

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			src := filepath.Join("testdata", name)
			dir := copyModule(t, src)

			changes := mustGenerate(t, dir)
			if err := apply(dir, changes, &bytes.Buffer{}); err != nil {
				t.Fatalf("apply() error = %v", err)
			}

			got := findFiles(t, dir, fileName)
			if *update {
				updateGolden(t, src, got)
			}

			want := findFiles(t, src, fileName+".golden")

			dirs := make(map[string]bool)
			for rel := range got {
				dirs[rel] = true
			}
			for rel := range want {
				dirs[rel] = true
			}

			for _, rel := range slices.Sorted(maps.Keys(dirs)) {
				switch g, w := got[rel], want[rel]; {
				case w == nil:
					t.Errorf("%s: unexpected file:\n%s", rel, g)
				case g == nil:
					t.Errorf("%s: file was not generated", rel)
				case !sameContent(g, w):
					t.Errorf("%s differs from golden file:\n%s", rel, g)
				}
			}

			if again := mustGenerate(t, dir); len(again) > 0 {
				t.Errorf("second run changes %s, want nothing to change", again[0].path)
			}

			if testing.Short() {
				t.Skip("skipping go test of the generated code in short mode")
			}

			goCommand(t, dir, "vet", "./...")
			goCommand(t, dir, "test", "./...")
		})
	}
}

// TestProblems проверяет, что на ошибки разметки утилита отвечает сообщением с
// местом в исходниках и ничего не меняет.
func TestProblems(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "not a struct",
			files: map[string]string{"a.go": "package a\n\n// generate:reset\ntype IDs []string\n"},
			want:  "a.go:4:6: IDs is not a struct type: generate:reset applies only to structs",
		},
		{
			name:  "alias",
			files: map[string]string{"a.go": "package a\n\ntype Plain struct{ n int }\n\n// generate:reset\ntype Alias = Plain\n"},
			want:  "a.go:6:6: Alias is an alias: mark the type it refers to instead",
		},
		{
			name:  "function",
			files: map[string]string{"a.go": "package a\n\n// generate:reset\nfunc F() {}\n"},
			want:  "a.go:4:1: generate:reset applies only to struct types, not to functions",
		},
		{
			name:  "variable",
			files: map[string]string{"a.go": "package a\n\n// generate:reset\nvar v int\n"},
			want:  "a.go:4:1: generate:reset applies only to struct types, not to var declarations",
		},
		{
			name:  "constant in a group",
			files: map[string]string{"a.go": "package a\n\nconst (\n\t// generate:reset\n\tc = 1\n)\n"},
			want:  "a.go:5:2: generate:reset applies only to struct types, not to const declarations",
		},
		{
			name: "handwritten Reset",
			files: map[string]string{"a.go": "package a\n\n// generate:reset\ntype T struct{ n int }\n\n" +
				"func (t *T) Reset() { t.n = 0 }\n"},
			want: "a.go:6:1: T is marked generate:reset but already has method Reset",
		},
		{
			name:  "field named Reset",
			files: map[string]string{"a.go": "package a\n\n// generate:reset\ntype T struct {\n\tReset bool\n}\n"},
			want:  "a.go:5:2: field Reset of T conflicts with the generated method Reset",
		},
		{
			name:  "unsupported tag",
			files: map[string]string{"a.go": "package a\n\n// generate:reset\ntype T struct {\n\tn int `reset:\"skip\"`\n}\n"},
			want:  `a.go:5:2: field n: unsupported tag value reset:"skip", only reset:"-" is allowed`,
		},
		{
			name: "unsupported tag in an anonymous struct",
			files: map[string]string{"a.go": "package a\n\n// generate:reset\ntype T struct {\n\tin struct {\n" +
				"\t\tn int `reset:\"keep\"`\n\t}\n}\n"},
			want: `a.go:6:3: field n: unsupported tag value reset:"keep", only reset:"-" is allowed`,
		},
		{
			name:  "unknown field type",
			files: map[string]string{"a.go": "package a\n\n// generate:reset\ntype T struct {\n\tn Undefined\n}\n"},
			want:  "a.go:5:2: field n: cannot determine its type",
		},
		{
			name: "foreign reset.gen.go",
			files: map[string]string{
				"a.go":         "package a\n\n// generate:reset\ntype T struct{ n int }\n",
				"reset.gen.go": "package a\n\nfunc helper() {}\n",
			},
			want: "reset.gen.go:1:1: file was not generated by cmd/reset, refusing to overwrite it",
		},
		{
			name: "syntax error",
			files: map[string]string{
				"a.go": "package a\n\n// generate:reset\ntype T struct{ n int }\n",
				"b.go": "package a\n\nfunc f() {}}\n",
			},
			want: "b.go:3:12: expected declaration, found '}'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := writeModule(t, tt.files)

			changes, problems, err := generate(dir, []string{"./..."})
			if err != nil {
				t.Fatalf("generate() error = %v", err)
			}

			if len(changes) > 0 {
				t.Errorf("generate() changes %s, want no changes", changes[0].path)
			}

			if len(problems) != 1 || !strings.HasPrefix(problems[0].String(), tt.want) {
				t.Errorf("generate() problems = %q, want one starting with %q", problems, tt.want)
			}
		})
	}
}

// Шаблон, под который не попал ни один пакет, — ошибка: молча ничего не
// сделав, утилита скрыла бы опечатку в аргументах.
func TestUnknownPattern(t *testing.T) {
	dir := writeModule(t, map[string]string{"a.go": "package a\n"})

	changes, problems, err := generate(dir, []string{"./missing/..."})
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}

	if len(changes) > 0 || len(problems) != 1 || !strings.Contains(problems[0].String(), "missing") {
		t.Errorf("generate() = %v, %q, want one problem about the pattern", changes, problems)
	}
}

func TestSplitPos(t *testing.T) {
	tests := []struct {
		pos       string
		file      string
		line, col int
		ok        bool
	}{
		{`C:\mod\a.go:3:12`, `C:\mod\a.go`, 3, 12, true},
		{`C:\mod\a.go:3`, `C:\mod\a.go`, 3, 0, true},
		{"/mod/a.go:7:1", "/mod/a.go", 7, 1, true},
		{"-", "", 0, 0, false},
		{"", "", 0, 0, false},
	}

	for _, tt := range tests {
		file, line, col, ok := splitPos(tt.pos)
		if file != tt.file || line != tt.line || col != tt.col || ok != tt.ok {
			t.Errorf("splitPos(%q) = %q, %d, %d, %t, want %q, %d, %d, %t",
				tt.pos, file, line, col, ok, tt.file, tt.line, tt.col, tt.ok)
		}
	}
}

// TestRun проверяет работу утилиты целиком: что она пишет в stdout и stderr и
// что при ошибке в одном пакете не меняет файлы и в других.
func TestRun(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"a/a.go": "package a\n\n// generate:reset\ntype T struct{ n int }\n",
		"b/b.go": "package b\n\ntype U struct{ n int }\n",
	})

	var stdout, stderr bytes.Buffer
	if err := run(dir, nil, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v, stderr:\n%s", err, &stderr)
	}

	generated := filepath.Join("a", fileName)
	if got, want := stdout.String(), "wrote "+generated+"\n"; got != want {
		t.Errorf("first run printed %q, want %q", got, want)
	}

	stdout.Reset()
	if err := run(dir, nil, &stdout, &stderr); err != nil || stdout.Len() > 0 {
		t.Errorf("second run: error = %v, printed %q, want nothing to change", err, &stdout)
	}

	// Метка в b ошибочна: утилита не должна ни создать файл в b, ни удалить
	// файл в a, хотя метку из a убрали.
	writeFile(t, filepath.Join(dir, "a", "a.go"), "package a\n\ntype T struct{ n int }\n")
	writeFile(t, filepath.Join(dir, "b", "b.go"), "package b\n\n// generate:reset\ntype U []int\n")

	stdout.Reset()
	err := run(dir, nil, &stdout, &stderr)
	if err == nil || err.Error() != "1 error(s) found, no files changed" {
		t.Errorf("run() error = %v, want the error count", err)
	}

	if want := filepath.Join("b", "b.go") + ":4:6: U is not a struct type"; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want it to contain %q", &stderr, want)
	}

	if _, err := os.Stat(filepath.Join(dir, generated)); err != nil {
		t.Errorf("%s was removed despite the error: %v", generated, err)
	}

	if _, err := os.Stat(filepath.Join(dir, "b", fileName)); err == nil {
		t.Errorf("b/%s was written despite the error", fileName)
	}

	// Когда ошибку исправили, старый файл в a удаляется.
	writeFile(t, filepath.Join(dir, "b", "b.go"), "package b\n\ntype U []int\n")

	stdout.Reset()
	if err := run(dir, nil, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	if got, want := stdout.String(), "removed "+generated+"\n"; got != want {
		t.Errorf("run printed %q, want %q", got, want)
	}
}

// Файл, который отличается от нового только окончаниями строк, утилита не
// переписывает: так его выдаёт git в Windows с core.autocrlf=true.
func TestCRLFIsUpToDate(t *testing.T) {
	dir := writeModule(t, map[string]string{"a.go": "package a\n\n// generate:reset\ntype T struct{ s []int }\n"})

	if err := apply(dir, mustGenerate(t, dir), &bytes.Buffer{}); err != nil {
		t.Fatalf("apply() error = %v", err)
	}

	path := filepath.Join(dir, fileName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, strings.ReplaceAll(string(data), "\n", "\r\n"))

	if changes := mustGenerate(t, dir); len(changes) > 0 {
		t.Errorf("generate() wants to rewrite a file with CRLF line endings")
	}
}

// TestProjectIsUpToDate проверяет, что файлы reset.gen.go проекта совпадают с
// тем, что создала бы утилита: новое поле помеченной структуры без повторного
// запуска осталось бы без сброса, и компилятор этого не заметит.
func TestProjectIsUpToDate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping loading of the whole module in short mode")
	}

	changes, problems, err := generate(filepath.Join("..", ".."), []string{"./..."})
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}

	for _, p := range problems {
		t.Error(p)
	}

	for _, c := range changes {
		t.Errorf("%s is out of date: run go run ./cmd/reset from the repository root", c.path)
	}
}

func TestInitials(t *testing.T) {
	tests := map[string]string{
		"ResetableStruct": "rs",
		"HTTPServer":      "hs",
		"mac":             "m",
		"URL":             "u",
		"compressReader":  "cr",
		"Vec3D":           "vd",
	}

	for name, want := range tests {
		if got := initials(name); got != want {
			t.Errorf("initials(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestDefaultImportName(t *testing.T) {
	tests := []struct {
		path, pkgName, want string
	}{
		{"time", "time", "time"},
		{"sync/atomic", "atomic", "atomic"},
		{"github.com/go-chi/chi/v5", "chi", "chi"},
		{"gopkg.in/yaml.v3", "yaml", ""},
		{"example.com/go-lib", "lib", ""},
	}

	for _, tt := range tests {
		if got := defaultImportName(tt.path, tt.pkgName); got != tt.want {
			t.Errorf("defaultImportName(%q, %q) = %q, want %q", tt.path, tt.pkgName, got, tt.want)
		}
	}
}

func mustGenerate(t *testing.T, dir string) []change {
	t.Helper()

	changes, problems, err := generate(dir, []string{"./..."})
	if err != nil {
		t.Fatalf("generate() error = %v", err)
	}

	for _, p := range problems {
		t.Errorf("generate() problem: %s", p)
	}

	if t.Failed() {
		t.FailNow()
	}

	return changes
}

// copyModule копирует модуль из testdata во временный каталог: утилита
// записывает файлы рядом с исходниками.
func copyModule(t *testing.T, src string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
		t.Fatalf("copy %s: %v", src, err)
	}

	return dir
}

// writeModule создаёт во временном каталоге модуль из файлов files.
func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/m\n\ngo 1.26\n")

	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}

		writeFile(t, path, content)
	}

	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// findFiles возвращает содержимое всех файлов с именем name в дереве root по
// каталогу относительно root.
func findFiles(t *testing.T, root, name string) map[string][]byte {
	t.Helper()

	files := make(map[string][]byte)

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.Name() != name {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}

		files[rel] = data
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	return files
}

// updateGolden записывает в каталог src эталоны по файлам got и удаляет
// эталоны, которым файла не нашлось.
func updateGolden(t *testing.T, src string, got map[string][]byte) {
	t.Helper()

	for rel := range findFiles(t, src, fileName+".golden") {
		if got[rel] == nil {
			if err := os.Remove(filepath.Join(src, rel, fileName+".golden")); err != nil {
				t.Fatal(err)
			}
		}
	}

	for rel, data := range got {
		writeFile(t, filepath.Join(src, rel, fileName+".golden"), string(data))
	}
}

// goCommand запускает команду go в модуле dir и проваливает тест, если она
// завершилась ошибкой.
func goCommand(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off")

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("go %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}
