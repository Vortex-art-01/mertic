// Package osexit содержит анализатор, который запрещает прямой вызов os.Exit
// в функции main пакета main.
//
// # Зачем
//
// os.Exit завершает процесс немедленно: отложенные вызовы не выполняются,
// поэтому файлы остаются несброшенными, а соединения — незакрытыми. Функцию,
// которая так завершается, к тому же нельзя вызвать из теста. Работу main
// стоит вынести в функцию, возвращающую ошибку или код возврата, а в самой
// main оставить только сообщение о результате:
//
//	func main() {
//		if err := run(); err != nil {
//			log.Fatal(err)
//		}
//	}
//
// # Что считается нарушением
//
// Любой вызов os.Exit, записанный в теле функции main пакета main: в том числе
// во вложенных блоках, в литералах функций и в операторах defer и go.
//
//	func main() {
//		if err := run(); err != nil {
//			fmt.Fprintln(os.Stderr, err)
//			os.Exit(1) // direct call to os.Exit in function main of package main
//		}
//	}
//
// Вызываемая функция определяется по информации о типах, а не по тексту.
// Поэтому вызов находится и когда пакет os импортирован под другим именем или
// через точку, а локальная переменная с именем os нарушением не считается.
//
// # Что нарушением не считается
//
//   - вызов os.Exit из любой другой функции, включая метод с именем main и
//     функцию main пакета, который называется не main;
//   - косвенный вызов: через переменную, в которую положили os.Exit, или
//     через функцию, которая вызывает os.Exit сама, — например, log.Fatal;
//   - сгенерированные файлы: автор проекта ими не управляет. К ним относится
//     и функция main, которую go test создаёт для запуска тестов пакета: она
//     заканчивается вызовом os.Exit.
package osexit

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

const doc = `forbid direct calls to os.Exit in function main of package main

os.Exit terminates the process immediately: deferred functions do not run,
so files are left unflushed and connections unclosed, and a function that
exits this way cannot be called from a test. Move the work of main into a
function that returns an error or an exit code, and let main only report it.

The analyzer reports every call to os.Exit written in the body of function
main of package main, including calls in nested blocks, function literals,
defer and go statements. The callee is resolved through type information:
renamed and dot imports of package os are recognized, and a local identifier
named os is not mistaken for the package.

Calls from other functions and indirect calls (through a variable or through
a helper such as log.Fatal) are not reported. Generated files are skipped,
including the main function that "go test" synthesizes to run the tests.`

// Analyzer сообщает о прямых вызовах os.Exit в функции main пакета main.
var Analyzer = &analysis.Analyzer{
	Name: "osexit",
	Doc:  doc,
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	if pass.Pkg.Name() != "main" {
		return nil, nil
	}

	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !isMain(fn) {
				continue
			}

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok && isOSExit(pass.TypesInfo, call) {
					pass.ReportRangef(call, "direct call to os.Exit in function main of package main")
				}

				return true
			})
		}
	}

	return nil, nil
}

// isMain сообщает, объявляет ли fn функцию main с телом. Метод с именем main
// точкой входа не считается, а тела нет у функции, реализованной на
// ассемблере: в ней вызовов искать негде.
func isMain(fn *ast.FuncDecl) bool {
	return fn.Recv == nil && fn.Name.Name == "main" && fn.Body != nil
}

// isOSExit сообщает, вызывает ли call функцию Exit стандартного пакета os.
// Косвенные вызовы — через переменную или значение интерфейса — статической
// цели не имеют и сюда не попадают.
func isOSExit(info *types.Info, call *ast.CallExpr) bool {
	fn := typeutil.StaticCallee(info, call)

	return fn != nil && fn.FullName() == "os.Exit"
}
