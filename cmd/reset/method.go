package main

import (
	"bytes"
	"fmt"
	"go/format"
	"go/scanner"
	"go/token"
	"go/types"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"unicode"
)

// fileTemplate — каркас файла reset.gen.go: заголовок, импорты и методы Reset с
// проверкой получателя на nil. Операторы тела шаблон получает готовыми: правила
// сброса ветвятся по видам типов, и в коде они читаются проще, чем читались бы
// в шаблоне.
//
// Отступы в шаблоне — для читателя: результат всё равно форматирует go/format.
// Переводы строк он сохраняет, поэтому пустая строка шаблона — это пустая
// строка файла, а у строк с if, else и end маркеры {{- и -}} срезают перевод
// строки, чтобы такая строка не оставила в файле пустую.
var fileTemplate = template.Must(template.New(fileName).Parse(`{{.Header}}

package {{.Package}}
{{if eq (len .Imports) 1}}
import {{index .Imports 0}}
{{else if .Imports}}
import (
{{range .Imports}}	{{.}}
{{end}})
{{end}}
{{- range .Methods}}
// Reset сбрасывает {{.Type}} для повторного использования.
{{if .Lines -}}
func ({{.Recv}} *{{.RecvType}}) Reset() {
	if {{.Recv}} == nil {
		return
	}

{{range .Lines}}	{{.}}
{{end -}}
}
{{else -}}
func ({{.Recv}} *{{.RecvType}}) Reset() {}
{{end}}
{{- end}}`))

// fileData — то, что fileTemplate подставляет в файл.
type fileData struct {
	Header  string
	Package string
	Imports []string     // спецификации импорта: "time" или template2 "html/template"
	Methods []methodData // в порядке объявления структур
}

// methodData — то, что fileTemplate подставляет в метод Reset одной структуры.
type methodData struct {
	Type     string   // имя структуры для doc-комментария
	Recv     string   // имя получателя
	RecvType string   // тип получателя без звёздочки, с параметрами типа: Box[T, K]
	Lines    []string // операторы после проверки на nil; пусто — тело пустое
}

// render собирает файл reset.gen.go пакета u: заголовок, импорты и методы Reset
// всех помеченных структур в порядке их объявления.
func (g *generator) render(u *unit) ([]byte, bool) {
	f := &fileGen{g: g, pkg: u.pkg.Types, imports: make(map[string]importedPkg)}

	data := fileData{Header: header, Package: u.name}
	for _, named := range u.targets {
		data.Methods = append(data.Methods, f.method(named))
	}

	// Импорты известны только после методов: пакет попадает в них, когда на
	// него впервые ссылается тело.
	data.Imports = f.importSpecs()

	src, err := renderSource(data)
	if err != nil {
		// Шаблон не выполнился или собрал код, который не разбирается, — это
		// ошибка в самой утилите, а не в разметке.
		g.problems = append(g.problems, problem{
			file: displayPath(g.dir, u.path),
			msg:  fmt.Sprintf("internal error: %v", err),
		})

		return nil, false
	}

	return src, true
}

// renderSource выполняет fileTemplate и форматирует результат так же, как gofmt.
func renderSource(data fileData) ([]byte, error) {
	var b bytes.Buffer
	if err := fileTemplate.Execute(&b, data); err != nil {
		return nil, fmt.Errorf("cannot execute template: %w", err)
	}

	src, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("generated code does not parse: %w", err)
	}

	return src, nil
}

// fileGen собирает один файл reset.gen.go: ведёт его импорты.
type fileGen struct {
	g       *generator
	pkg     *types.Package
	imports map[string]importedPkg // по пути импорта
}

type importedPkg struct {
	name    string // имя, под которым пакет импортирован в файле
	pkgName string // собственное имя пакета
}

// qualify — types.Qualifier файла: возвращает имя, под которым в файле виден
// пакет p, и при первом обращении добавляет p в импорты.
func (f *fileGen) qualify(p *types.Package) string {
	if p.Path() == f.pkg.Path() {
		return ""
	}

	if imp, ok := f.imports[p.Path()]; ok {
		return imp.name
	}

	// Имя импорта не должно совпадать с именами уровня пакета: файловая и
	// пакетная области видимости не могут объявлять одно имя. Встроенные имена
	// тоже заняты, чтобы импорт не скрыл nil или clear.
	taken := func(name string) bool {
		if f.pkg.Scope().Lookup(name) != nil || types.Universe.Lookup(name) != nil {
			return true
		}

		for _, imp := range f.imports {
			if imp.name == name {
				return true
			}
		}

		return false
	}

	imp := importedPkg{name: freeName(p.Name(), taken), pkgName: p.Name()}
	f.imports[p.Path()] = imp

	return imp.name
}

// importSpecs возвращает спецификации импорта файла по порядку путей. Имя пакета
// указывается явно, только если оно не то, что читатель ожидает увидеть по пути.
func (f *fileGen) importSpecs() []string {
	specs := make([]string, 0, len(f.imports))
	for _, path := range slices.Sorted(maps.Keys(f.imports)) {
		if imp := f.imports[path]; imp.name == defaultImportName(path, imp.pkgName) {
			specs = append(specs, strconv.Quote(path))
		} else {
			specs = append(specs, imp.name+" "+strconv.Quote(path))
		}
	}

	return specs
}

// defaultImportName возвращает имя, которое читатель ожидает увидеть у пакета
// path без явного имени в импорте: последний элемент пути, а для путей вида
// .../chi/v5 — элемент перед номером версии. Если настоящее имя другое, render
// указывает его в импорте явно.
func defaultImportName(path, pkgName string) string {
	elems := strings.Split(path, "/")
	last := elems[len(elems)-1]

	if len(elems) > 1 && len(last) > 1 && last[0] == 'v' && strings.Trim(last[1:], "0123456789") == "" {
		last = elems[len(elems)-2]
	}

	if last != pkgName {
		return ""
	}

	return pkgName
}

// accessible сообщает, видны ли из пакета файла все поля анонимной структуры
// st. Невидимые поля бывают у структуры, которая пришла из другого пакета через
// псевдоним: сбросить такие поля по отдельности нельзя.
func (f *fileGen) accessible(st *types.Struct) bool {
	for fld := range st.Fields() {
		if !fld.Exported() && fld.Pkg().Path() != f.pkg.Path() {
			return false
		}
	}

	return true
}

// method создаёт метод Reset структуры named. Код получается за два прохода.
// Первый узнаёт, на какие имена ссылается тело: пакеты, типы, встроенные nil и
// clear. Второй пишет тело заново с именами получателя и переменных циклов,
// которые ни одно из этих имён не перекрывают.
func (f *fileGen) method(named *types.Named) methodData {
	draft := &methodGen{file: f, recv: "x", free: make(map[string]bool)}
	draft.body(named)

	taken := maps.Clone(draft.free)
	taken["nil"] = true // проверка получателя на nil
	taken[named.Obj().Name()] = true

	for tp := range named.TypeParams().TypeParams() {
		taken[tp.Obj().Name()] = true
	}

	isTaken := func(name string) bool { return taken[name] }

	recv := f.receiverName(named, isTaken)
	taken[recv] = true

	loopVars := make([]string, draft.maxDepth)
	for i := range loopVars {
		loopVars[i] = freeName(loopVarName(i), isTaken)
		taken[loopVars[i]] = true
	}

	m := &methodGen{file: f, recv: recv, loopVars: loopVars, free: make(map[string]bool)}
	m.body(named)

	return m.data(named)
}

// receiverName выбирает имя получателя. Если у типа уже есть методы, берётся
// то же имя, что у них: staticcheck (ST1016) требует одинаковых имён. Иначе —
// первые буквы слов в имени типа: rs для ResetableStruct.
func (f *fileGen) receiverName(named *types.Named, taken func(string) bool) string {
	counts := make(map[string]int)
	var order []string

	for m := range named.Origin().Methods() {
		if f.g.isStale(m.Pos()) {
			continue
		}

		name := m.Signature().Recv().Name()
		if name == "" || name == "_" {
			continue
		}

		if counts[name] == 0 {
			order = append(order, name)
		}
		counts[name]++
	}

	// Самое частое имя; при равенстве — то, что встретилось раньше.
	var best string
	for _, name := range order {
		if counts[name] > counts[best] {
			best = name
		}
	}

	if best != "" && !taken(best) {
		return best
	}

	return freeName(initials(named.Obj().Name()), taken)
}

// initials возвращает первые буквы слов имени в нижнем регистре:
// ResetableStruct → rs, HTTPServer → hs, mac → m.
func initials(name string) string {
	runes := []rune(name)

	var b strings.Builder
	for i, r := range runes {
		if i == 0 || startsWord(runes, i) {
			b.WriteRune(unicode.ToLower(r))
		}
	}

	return b.String()
}

// startsWord сообщает, начинается ли с руны i новое слово: заглавная буква
// после строчной или цифры, а в аббревиатуре — последняя заглавная перед
// строчной, как S в HTTPServer.
func startsWord(runes []rune, i int) bool {
	if !unicode.IsUpper(runes[i]) {
		return false
	}

	prev := runes[i-1]
	if unicode.IsLower(prev) || unicode.IsDigit(prev) {
		return true
	}

	return unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1])
}

// loopVarName возвращает основу имени переменной цикла на глубине depth.
func loopVarName(depth int) string {
	if depth < 3 {
		return string(rune('i' + depth))
	}

	return "i" + strconv.Itoa(depth)
}

// freeName возвращает base или, если это имя занято или оказалось ключевым
// словом, — base с наименьшим числовым суффиксом, начиная с 2: if2 для
// IsFine.
func freeName(base string, taken func(string) bool) string {
	valid := func(name string) bool {
		return token.IsIdentifier(name) && name != "_" && !taken(name)
	}

	// Суффикс не исправит основу, которая не годится в идентификатор по другой
	// причине. У имён типов и пакетов такой не бывает, но цикл ниже без этой
	// проверки не закончился бы.
	if !token.IsIdentifier(base) && !token.IsKeyword(base) {
		base = "v"
	}

	if valid(base) {
		return base
	}

	for n := 2; ; n++ {
		if name := base + strconv.Itoa(n); valid(name) {
			return name
		}
	}
}

// action — что нужно сделать со значением, чтобы его сбросить.
type action int

const (
	keep   action = iota // ничего: значение и так начальное, или трогать его нельзя
	assign               // присвоить нулевое значение
	custom               // отдельные операторы: обрезать слайс, вызвать Reset и т. д.
)

// methodGen пишет тело метода Reset одной структуры.
type methodGen struct {
	file     *fileGen
	recv     string          // имя получателя
	loopVars []string        // имена переменных циклов по глубине вложенности
	depth    int             // текущая глубина вложенности циклов
	maxDepth int             // наибольшая глубина: сколько имён понадобится
	free     map[string]bool // имена, на которые ссылается тело
	lines    []string
}

// body пишет операторы, которые сбрасывают поля структуры named.
func (m *methodGen) body(named *types.Named) {
	st := named.Underlying().(*types.Struct)
	recv := operand{expr: m.recv}

	for i := range st.NumFields() {
		if !skipField(st, i) {
			m.reset(recv.field(st.Field(i).Name()), st.Field(i).Type(), false)
		}
	}
}

// data возвращает метод в том виде, в каком его подставляет fileTemplate.
func (m *methodGen) data(named *types.Named) methodData {
	recvType := named.Obj().Name()
	if tps := named.TypeParams(); tps.Len() > 0 {
		names := make([]string, 0, tps.Len())
		for tp := range tps.TypeParams() {
			names = append(names, tp.Obj().Name())
		}

		recvType += "[" + strings.Join(names, ", ") + "]"
	}

	return methodData{
		Type:     named.Obj().Name(),
		Recv:     m.recv,
		RecvType: recvType,
		Lines:    m.lines,
	}
}

// skipField сообщает, что i-е поле структуры st генератор не трогает: пустое
// поле _ недоступно по имени, а тег reset:"-" просит оставить поле как есть.
func skipField(st *types.Struct, i int) bool {
	_, tagged := reflect.StructTag(st.Tag(i)).Lookup(tagKey)

	return tagged || st.Field(i).Name() == "_"
}

// plan решает, как сбросить значение типа t. pointee сообщает, что значение
// лежит по указателю: тогда именованную структуру без метода Reset() трогать
// нельзя — указатель мог прийти извне, и обнулить, например, общий
// *slog.Logger значило бы сломать его для всех.
func (m *methodGen) plan(t types.Type, pointee bool) action {
	if _, ok := types.Unalias(t).(*types.TypeParam); ok {
		return assign
	}

	// Указатель проверяется раньше Reset: у *T есть методы T, но вызывать их
	// можно, только убедившись, что указатель не nil.
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		if m.plan(u.Elem(), true) == keep {
			return keep
		}
		return custom

	case *types.Basic:
		if u.Kind() == types.Invalid {
			return keep
		}
	}

	if m.file.g.hasReset(t) {
		return custom
	}

	switch u := t.Underlying().(type) {
	case *types.Basic, *types.Interface, *types.Chan, *types.Signature:
		return assign

	case *types.Slice, *types.Map:
		return custom

	case *types.Array:
		if u.Len() == 0 {
			return keep
		}
		return m.plan(u.Elem(), false)

	case *types.Struct:
		if _, named := types.Unalias(t).(*types.Named); named {
			if pointee || u.NumFields() == 0 {
				return keep
			}
			return assign
		}

		if !m.file.accessible(u) {
			return assign
		}

		for i := range u.NumFields() {
			if !skipField(u, i) && m.plan(u.Field(i).Type(), false) != keep {
				return custom
			}
		}
	}

	return keep
}

// reset пишет операторы, которые сбрасывают значение x типа t по правилам из
// документации пакета. pointee — как у plan.
func (m *methodGen) reset(x operand, t types.Type, pointee bool) {
	switch m.plan(t, pointee) {
	case keep:
		return
	case assign:
		m.printf("%s = %s", x.value(), m.zero(t))
		return
	}

	if ptr, ok := t.Underlying().(*types.Pointer); ok {
		m.use("nil")
		m.printf("if %s != nil {", x.value())
		_, namedPtr := types.Unalias(t).(*types.Named)
		m.reset(x.deref(namedPtr), ptr.Elem(), true)
		m.printf("}")

		return
	}

	if m.file.g.hasReset(t) {
		if !types.IsInterface(t) {
			m.printf("%s", x.call("Reset", false))
			return
		}

		m.use("nil")
		m.printf("if %s != nil {", x.value())
		m.printf("%s", x.call("Reset", true))
		m.printf("}")

		return
	}

	switch u := t.Underlying().(type) {
	case *types.Slice:
		m.printf("%s = %s[:0]", x.value(), x.primary())

	case *types.Map:
		m.use("clear")
		m.printf("clear(%s)", x.value())

	case *types.Array:
		v := m.enterLoop()
		m.printf("for %s := range %s {", v, x.value())
		m.reset(x.index(v), u.Elem(), false)
		m.printf("}")
		m.leaveLoop()

	case *types.Struct:
		for i := range u.NumFields() {
			if !skipField(u, i) {
				m.reset(x.field(u.Field(i).Name()), u.Field(i).Type(), false)
			}
		}
	}
}

// zero возвращает нулевое значение типа t в виде выражения Go.
func (m *methodGen) zero(t types.Type) string {
	if _, ok := types.Unalias(t).(*types.TypeParam); ok {
		m.use("new")
		return "*new(" + m.typeString(t) + ")"
	}

	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsBoolean != 0:
			m.use("false")
			return "false"
		case u.Info()&types.IsString != 0:
			return `""`
		case u.Kind() == types.UnsafePointer:
			m.use("nil")
			return "nil"
		default:
			return "0"
		}

	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Signature, *types.Interface:
		m.use("nil")
		return "nil"

	default: // структура или массив
		return m.typeString(t) + "{}"
	}
}

// typeString записывает тип t так, как он виден из файла, и запоминает имена,
// на которые ссылается запись.
func (m *methodGen) typeString(t types.Type) string {
	s := types.TypeString(t, m.file.qualify)

	// Имена после точки — селекторы: они не могут совпасть с локальными
	// именами метода. Остальные — пакеты, типы и параметры типа.
	var sc scanner.Scanner
	fset := token.NewFileSet()
	sc.Init(fset.AddFile("", fset.Base(), len(s)), []byte(s), nil, 0)

	afterPeriod := false
	for {
		_, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}

		if tok == token.IDENT && !afterPeriod {
			m.use(lit)
		}
		afterPeriod = tok == token.PERIOD
	}

	return s
}

// use запоминает, что тело метода ссылается на имя name.
func (m *methodGen) use(name string) {
	m.free[name] = true
}

func (m *methodGen) printf(format string, args ...any) {
	m.lines = append(m.lines, fmt.Sprintf(format, args...))
}

// enterLoop входит в цикл на следующей глубине и возвращает имя его переменной.
func (m *methodGen) enterLoop() string {
	m.depth++
	m.maxDepth = max(m.maxDepth, m.depth)

	if m.depth <= len(m.loopVars) {
		return m.loopVars[m.depth-1]
	}

	// Первый проход: имена ещё не выбраны.
	return loopVarName(m.depth - 1)
}

func (m *methodGen) leaveLoop() {
	m.depth--
}

// operand — выражение Go для сбрасываемого значения: поле получателя, элемент
// массива или значение по указателю.
type operand struct {
	expr string

	// Для значения по указателю: expr — это *p, ptr — сам p, namedPtr — у p
	// определённый тип-указатель вроде type P *T, для которого Go не
	// разыменовывает p при вызове метода.
	ptr      *operand
	namedPtr bool
}

// value — выражение целиком: для присваивания, сравнения и аргумента функции.
func (x operand) value() string {
	return x.expr
}

// primary — выражение, к которому можно приписать селектор, индекс или срез:
// *p для этого нужно взять в скобки.
func (x operand) primary() string {
	if x.ptr != nil {
		return "(" + x.expr + ")"
	}

	return x.expr
}

// field — поле name. Поле значения по указателю Go находит и через сам
// указатель: p.f вместо (*p).f.
func (x operand) field(name string) operand {
	if x.ptr != nil {
		return operand{expr: x.ptr.primary() + "." + name}
	}

	return operand{expr: x.expr + "." + name}
}

// call — вызов метода name без аргументов. Метод значения по указателю тоже
// вызывается через указатель, кроме двух случаев: у указателя определённый
// тип или по нему лежит интерфейс — у *I методов нет.
func (x operand) call(name string, iface bool) string {
	if x.ptr != nil && !x.namedPtr && !iface {
		return x.ptr.primary() + "." + name + "()"
	}

	return x.primary() + "." + name + "()"
}

// index — элемент i массива.
func (x operand) index(i string) operand {
	return operand{expr: x.primary() + "[" + i + "]"}
}

// deref — значение по указателю x.
func (x operand) deref(namedPtr bool) operand {
	return operand{expr: "*" + x.expr, ptr: &x, namedPtr: namedPtr}
}
