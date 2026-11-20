package main

import (
	"go/types"
	"slices"
)

// hasReset сообщает, можно ли у значения типа t вызвать метод Reset() — без
// параметров и результатов. Значение считается адресуемым: генератор всегда
// обращается к полям через указатель на структуру, поэтому методы с
// получателем-указателем ему тоже доступны.
//
// Поиск повторяет правила Go для селектора x.Reset, как их реализует
// types.LookupFieldOrMethod: метод ищется сначала у самого типа, затем у
// встроенных полей, уровень за уровнем; найденное на меньшей глубине скрывает
// то, что глубже, а два кандидата на одной глубине дают неоднозначность. Своя
// реализация нужна, потому что учитывать надо не код, как он есть, а каким он
// станет после запуска: помеченные структуры получат Reset, даже если файла с
// ним ещё нет, а методы из старых файлов утилиты исчезнут или перепишутся.
// Поэтому найти Reset у типа, встроенного в другой тип, удаётся и тогда, когда
// метод только предстоит создать.
//
// Метод, до которого Go добирается через встроенный указатель или интерфейс,
// не в счёт, хотя вызвать его компилятор позволит: у нулевого значения
// struct{ *bytes.Buffer } такой вызов разыменует nil и упадёт с паникой.
func (g *generator) hasReset(t types.Type) bool {
	type embedded struct {
		typ       types.Type
		multiples bool // тип встречается на этой глубине несколько раз
		indirect  bool // путь к типу идёт через встроенный указатель или интерфейс
	}

	current := []embedded{{typ: t}}
	var seen []*types.Named // именованные типы с меньших глубин

	for len(current) > 0 {
		var (
			next   []embedded
			found  bool // на этой глубине есть поле или метод с именем Reset
			method bool // и это подходящий метод
		)

		// collision отмечает находку; true — селектор неоднозначен.
		collision := func(e embedded, isMethod bool) bool {
			if found || e.multiples {
				return true
			}

			found, method = true, isMethod
			return false
		}

		for _, e := range current {
			typ := types.Unalias(e.typ)

			if named, ok := typ.(*types.Named); ok {
				if slices.ContainsFunc(seen, func(s *types.Named) bool { return types.Identical(s, named) }) {
					continue
				}
				seen = append(seen, named)

				if declared, niladic := g.declaredReset(named); declared {
					if collision(e, niladic && !e.indirect) {
						return false
					}
					continue
				}
			}

			switch u := typ.Underlying().(type) {
			case *types.Struct:
				for f := range u.Fields() {
					if f.Name() == "Reset" {
						if collision(e, false) {
							return false
						}
						continue
					}

					if !found && f.Embedded() {
						next = append(next, embedded{
							typ:       derefType(f.Type()),
							multiples: e.multiples,
							indirect:  e.indirect || isPointer(f.Type()) || types.IsInterface(f.Type()),
						})
					}
				}

			case *types.Interface:
				if m := interfaceMethod(u, "Reset"); m != nil {
					if collision(e, isNiladic(m) && !e.indirect) {
						return false
					}
				}
			}
		}

		if found {
			return method
		}

		// Тип, встроенный на следующей глубине несколько раз, проверяется
		// однажды, но найденный в нём Reset будет неоднозначен.
		var merged []embedded
		for _, e := range next {
			i := slices.IndexFunc(merged, func(m embedded) bool { return types.Identical(m.typ, e.typ) })
			if i < 0 {
				merged = append(merged, e)
				continue
			}
			merged[i].multiples = true
		}

		current = merged
	}

	return false
}

// declaredReset сообщает, объявлен ли у типа named метод Reset и подходит ли
// его сигнатура. Помеченная структура получит метод в этом запуске; метод из
// старого файла утилиты не в счёт.
func (g *generator) declaredReset(named *types.Named) (declared, niladic bool) {
	origin := named.Origin()

	if obj := origin.Obj(); obj.Pkg() != nil && g.marked[typeKey(obj)] {
		return true, true
	}

	for m := range origin.Methods() {
		if m.Name() == "Reset" && !g.isStale(m.Pos()) {
			return true, isNiladic(m)
		}
	}

	return false, false
}

// interfaceMethod возвращает метод name интерфейса iface, включая методы
// встроенных интерфейсов, или nil.
func interfaceMethod(iface *types.Interface, name string) *types.Func {
	for m := range iface.Methods() {
		if m.Name() == name {
			return m
		}
	}

	return nil
}

// isNiladic сообщает, что у метода нет ни параметров, ни результатов.
func isNiladic(m *types.Func) bool {
	sig := m.Signature()

	return sig.Params().Len() == 0 && sig.Results().Len() == 0
}

// derefType снимает с типа встроенного поля указатель: *T встраивается как T.
func derefType(t types.Type) types.Type {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		return p.Elem()
	}

	return t
}

// isPointer сообщает, что встроенное поле типа t — указатель *T.
func isPointer(t types.Type) bool {
	_, ok := types.Unalias(t).(*types.Pointer)
	return ok
}
