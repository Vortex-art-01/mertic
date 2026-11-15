package main

import (
	"fmt"
	"slices"

	"github.com/gostaticanalysis/nilerr"
	"github.com/polyfloyd/go-errorlint/errorlint"
	"github.com/timakin/bodyclose/passes/bodyclose"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/appends"
	"golang.org/x/tools/go/analysis/passes/asmdecl"
	"golang.org/x/tools/go/analysis/passes/assign"
	"golang.org/x/tools/go/analysis/passes/atomic"
	"golang.org/x/tools/go/analysis/passes/atomicalign"
	"golang.org/x/tools/go/analysis/passes/bools"
	"golang.org/x/tools/go/analysis/passes/buildtag"
	"golang.org/x/tools/go/analysis/passes/cgocall"
	"golang.org/x/tools/go/analysis/passes/composite"
	"golang.org/x/tools/go/analysis/passes/copylock"
	"golang.org/x/tools/go/analysis/passes/deepequalerrors"
	"golang.org/x/tools/go/analysis/passes/defers"
	"golang.org/x/tools/go/analysis/passes/directive"
	"golang.org/x/tools/go/analysis/passes/errorsas"
	"golang.org/x/tools/go/analysis/passes/framepointer"
	"golang.org/x/tools/go/analysis/passes/gofix"
	"golang.org/x/tools/go/analysis/passes/hostport"
	"golang.org/x/tools/go/analysis/passes/httpmux"
	"golang.org/x/tools/go/analysis/passes/httpresponse"
	"golang.org/x/tools/go/analysis/passes/ifaceassert"
	"golang.org/x/tools/go/analysis/passes/inline"
	"golang.org/x/tools/go/analysis/passes/loopclosure"
	"golang.org/x/tools/go/analysis/passes/lostcancel"
	"golang.org/x/tools/go/analysis/passes/modernize"
	"golang.org/x/tools/go/analysis/passes/nilfunc"
	"golang.org/x/tools/go/analysis/passes/nilness"
	"golang.org/x/tools/go/analysis/passes/printf"
	"golang.org/x/tools/go/analysis/passes/reflectvaluecompare"
	"golang.org/x/tools/go/analysis/passes/scannererr"
	"golang.org/x/tools/go/analysis/passes/shadow"
	"golang.org/x/tools/go/analysis/passes/shift"
	"golang.org/x/tools/go/analysis/passes/sigchanyzer"
	"golang.org/x/tools/go/analysis/passes/slog"
	"golang.org/x/tools/go/analysis/passes/sortslice"
	"golang.org/x/tools/go/analysis/passes/sqlrowserr"
	"golang.org/x/tools/go/analysis/passes/stdmethods"
	"golang.org/x/tools/go/analysis/passes/stdversion"
	"golang.org/x/tools/go/analysis/passes/stringintconv"
	"golang.org/x/tools/go/analysis/passes/structtag"
	"golang.org/x/tools/go/analysis/passes/testinggoroutine"
	"golang.org/x/tools/go/analysis/passes/tests"
	"golang.org/x/tools/go/analysis/passes/timeformat"
	"golang.org/x/tools/go/analysis/passes/unmarshal"
	"golang.org/x/tools/go/analysis/passes/unreachable"
	"golang.org/x/tools/go/analysis/passes/unsafeptr"
	"golang.org/x/tools/go/analysis/passes/unusedresult"
	"golang.org/x/tools/go/analysis/passes/unusedwrite"
	"golang.org/x/tools/go/analysis/passes/waitgroup"
	"honnef.co/go/tools/analysis/lint"
	"honnef.co/go/tools/quickfix/qf1004"
	"honnef.co/go/tools/quickfix/qf1009"
	"honnef.co/go/tools/quickfix/qf1010"
	"honnef.co/go/tools/quickfix/qf1012"
	"honnef.co/go/tools/simple"
	"honnef.co/go/tools/staticcheck"
	"honnef.co/go/tools/stylecheck"

	"github.com/Vortex-art-01/mertic/cmd/staticlint/osexit"
)

// analyzers возвращает все анализаторы, из которых состоит staticlint. Что
// ищет каждый из них и почему набор именно такой, описано в документации
// пакета.
func analyzers() []*analysis.Analyzer {
	return slices.Concat(
		standardAnalyzers(),
		staticcheckAnalyzers(),
		publicAnalyzers(),
		[]*analysis.Analyzer{osexit.Analyzer},
	)
}

// standardAnalyzers возвращает анализаторы из golang.org/x/tools/go/analysis/passes.
// Какие из них не взяты и почему, сказано в документации пакета.
func standardAnalyzers() []*analysis.Analyzer {
	passes := []*analysis.Analyzer{
		appends.Analyzer,
		asmdecl.Analyzer,
		assign.Analyzer,
		atomic.Analyzer,
		atomicalign.Analyzer,
		bools.Analyzer,
		buildtag.Analyzer,
		cgocall.Analyzer,
		composite.Analyzer,
		copylock.Analyzer,
		deepequalerrors.Analyzer,
		defers.Analyzer,
		directive.Analyzer,
		errorsas.Analyzer,
		framepointer.Analyzer,
		gofix.Analyzer,
		hostport.Analyzer,
		httpmux.Analyzer,
		httpresponse.Analyzer,
		ifaceassert.Analyzer,
		inline.Analyzer,
		loopclosure.Analyzer,
		lostcancel.Analyzer,
		nilfunc.Analyzer,
		nilness.Analyzer,
		printf.Analyzer,
		reflectvaluecompare.Analyzer,
		scannererr.Analyzer,
		shadow.Analyzer,
		shift.Analyzer,
		sigchanyzer.Analyzer,
		slog.Analyzer,
		sortslice.Analyzer,
		sqlrowserr.Analyzer,
		stdmethods.Analyzer,
		stdversion.Analyzer,
		stringintconv.Analyzer,
		structtag.Analyzer,
		testinggoroutine.Analyzer,
		tests.Analyzer,
		timeformat.Analyzer,
		unmarshal.Analyzer,
		unreachable.Analyzer,
		unsafeptr.Analyzer,
		unusedresult.Analyzer,
		unusedwrite.Analyzer,
		waitgroup.Analyzer,
	}

	// Пакет modernize — не один анализатор, а набор: по анализатору на каждую
	// устаревшую конструкцию. Suite — те из них, что отобрали сами авторы.
	return slices.Concat(passes, modernize.Suite)
}

// staticcheckAnalyzers возвращает проверки staticcheck: классы SA, S и ST
// целиком и четыре проверки класса QF.
func staticcheckAnalyzers() []*analysis.Analyzer {
	var checks []*analysis.Analyzer

	for _, class := range [][]*lint.Analyzer{
		staticcheck.Analyzers, // SA: ошибки и неэффективный код
		simple.Analyzers,      // S: упрощения
		stylecheck.Analyzers,  // ST: стиль
	} {
		for _, check := range class {
			checks = append(checks, check.Analyzer)
		}
	}

	// Класс QF — подсказки для редактора, а не замечания: сам staticcheck их
	// не запускает. Взяты только те, что находят вероятную ошибку или лишнюю
	// работу; остальные предлагают равноценную запись того же кода.
	return append(checks,
		qf1004.Analyzer,
		qf1009.Analyzer,
		qf1010.Analyzer,
		qf1012.Analyzer,
	)
}

// publicAnalyzers возвращает сторонние анализаторы. Они выбраны под проект:
// агент и приёмник аудита ходят по HTTP, а повторять ли запрос, агент и
// хранилище решают по ошибке.
func publicAnalyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{
		bodyclose.Analyzer,
		errorlintAnalyzer(),
		nilerr.Analyzer,
	}
}

// errorlintAnalyzer возвращает errorlint с включённой проверкой fmt.Errorf.
// Без неё он следит только за сравнением ошибок, а проекту важно и то, как
// они оборачиваются: ошибка, вложенная не через %w, для errors.Is и errors.As
// потеряна.
func errorlintAnalyzer() *analysis.Analyzer {
	a := errorlint.NewAnalyzer()

	if err := a.Flags.Set("errorf", "true"); err != nil {
		// Такого флага нет, только если его переименовали в самом errorlint.
		panic(fmt.Sprintf("errorlint: enable the errorf check: %v", err))
	}

	return a
}
