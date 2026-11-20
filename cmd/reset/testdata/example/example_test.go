package example

import "testing"

func TestReset(t *testing.T) {
	childStr := "child"
	child := &ResetableStruct{i: 2, strP: &childStr, s: make([]int, 3, 8)}

	str := "value"
	m := map[string]string{"key": "value"}
	rs := &ResetableStruct{
		i:     1,
		str:   "text",
		strP:  &str,
		s:     []int{1, 2, 3},
		m:     m,
		child: child,
	}

	rs.Reset()

	if rs.i != 0 || rs.str != "" {
		t.Errorf("primitives: i = %d, str = %q, want zero values", rs.i, rs.str)
	}

	// Указатель остаётся, сбрасывается значение по нему.
	if rs.strP != &str || str != "" {
		t.Errorf("strP: pointer changed or *strP = %q, want the same pointer to \"\"", str)
	}

	if len(rs.s) != 0 || cap(rs.s) != 3 {
		t.Errorf("s: len %d, cap %d, want len 0 and the old capacity 3", len(rs.s), cap(rs.s))
	}

	if len(m) != 0 || rs.m == nil {
		t.Errorf("m: %v, want the same map, empty", rs.m)
	}

	// Вложенная структура сбрасывается своим методом Reset.
	if rs.child != child || child.i != 0 || childStr != "" || len(child.s) != 0 || cap(child.s) != 8 {
		t.Errorf("child = %+v, *child.strP = %q, want it reset in place", child, childStr)
	}

	var nilStruct *ResetableStruct
	nilStruct.Reset() // у nil метод ничего не делает и не паникует
}
