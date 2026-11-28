package a

import (
	"testing"

	"example.com/multi/b"
	"example.com/multi/c"
)

func TestA(t *testing.T) {
	item := &b.Item{ID: 2, Tags: []string{"x"}}
	x := A{
		item:  b.Item{ID: 1},
		pitem: item,
		wrap:  b.Wrapper{Item: b.Item{ID: 3}, Extra: 4},
		plain: b.Plain{N: 5},
		old:   c.Old{N: 6},
	}

	x.Reset()

	if x.item.ID != 0 || x.pitem != item || item.ID != 0 || len(item.Tags) != 0 {
		t.Errorf("items were not reset by b.Item.Reset: %+v %+v", x.item, item)
	}

	// Reset у Wrapper — метод встроенной Item: Extra он не трогает.
	if x.wrap.ID != 0 || x.wrap.Extra != 4 {
		t.Errorf("wrap = %+v, want Item reset and Extra kept", x.wrap)
	}

	if x.plain.N != 0 || x.old.N != 0 {
		t.Errorf("plain = %+v, old = %+v, want zero values", x.plain, x.old)
	}
}

func TestGroups(t *testing.T) {
	g1, h1, h2, d := G1{s: "s"}, H1{n: 1}, H2{list: []string{"a"}}, Directive{n: 1}

	g1.Reset()
	h1.Reset()
	h2.Reset()
	d.Reset()

	if g1.s != "" || h1.n != 0 || len(h2.list) != 0 || d.n != 0 {
		t.Errorf("groups were not reset: %+v %+v %+v %+v", g1, h1, h2, d)
	}
}
