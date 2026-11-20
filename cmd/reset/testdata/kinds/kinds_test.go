package kinds

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"hash"
	"strings"
	"testing"
	"time"
	"unsafe"
)

func TestPrimitives(t *testing.T) {
	x := 1
	p := Primitives{
		i: 1, i8: 2, u64: 3, up: 4, f: 5, c: 6i, b: true, s: "s", r: 'r', by: 7,
		ptr: unsafe.Pointer(&x), id: "id", temp: 36.6, flag: true, d: time.Second,
	}

	p.Reset()

	if p != (Primitives{}) {
		t.Errorf("after Reset = %+v, want zero value", p)
	}
}

func TestCollections(t *testing.T) {
	m := map[string]int{"a": 1}
	index := Index{"b": 2}
	c := Collections{ints: make([]int, 2, 5), names: Names{"x"}, m: m, index: index}

	c.Reset()

	if len(c.ints) != 0 || cap(c.ints) != 5 || len(c.names) != 0 || cap(c.names) != 1 {
		t.Errorf("slices: ints len %d cap %d, names len %d cap %d, want len 0 and old capacity",
			len(c.ints), cap(c.ints), len(c.names), cap(c.names))
	}

	if len(m) != 0 || len(index) != 0 || c.m == nil || c.index == nil {
		t.Errorf("maps = %v, %v, want the same maps, empty", c.m, c.index)
	}
}

func TestArrays(t *testing.T) {
	child := &Child{Name: "child", Items: []int{1}}
	h := sha256.New()
	h.Write([]byte("data"))
	empty := sha256.Sum256(nil)

	a := Arrays{
		sum:      [4]int{1, 2, 3, 4},
		block:    Block{1},
		times:    [2]time.Time{time.Now()},
		plains:   [2]Plain{{N: 1}},
		bufs:     [2][]byte{make([]byte, 1, 4)},
		grid:     [2][3][]int{1: {2: make([]int, 2, 3)}},
		children: [2]*Child{nil, child},
		hashes:   [2]hash.Hash{h},
	}

	a.Reset()

	if a.sum != ([4]int{}) || a.block != (Block{}) || a.times != ([2]time.Time{}) || a.plains[0].N != 0 {
		t.Errorf("zeroed arrays = %v %v %v %v", a.sum, a.block, a.times, a.plains)
	}

	if len(a.bufs[0]) != 0 || cap(a.bufs[0]) != 4 || len(a.grid[1][2]) != 0 || cap(a.grid[1][2]) != 3 {
		t.Errorf("slices in arrays were not truncated in place: %v %v", a.bufs, a.grid)
	}

	if a.children[1] != child || child.Name != "" || len(child.Items) != 0 {
		t.Errorf("children[1] = %+v, want the same child, reset", a.children[1])
	}

	if got := h.Sum(nil); !bytes.Equal(got, empty[:]) {
		t.Errorf("hashes[0] was not reset")
	}
}

func TestPointers(t *testing.T) {
	i, ii := 1, 2
	pii := &ii
	id := ID("id")
	s := []string{"a", "b"}
	m := map[string]int{"a": 1}
	var v any = 1
	arr := [2][]int{make([]int, 2)}
	child := &Child{Name: "child"}
	pc := &Child{Name: "pc"}
	cp := &Child{Name: "cp"}
	ipv := 3
	buf := bytes.NewBufferString("data")
	var h hash.Hash = sha256.New()
	h.Write([]byte("data"))
	opts := &struct{ n int }{n: 1}
	plain := &Plain{N: 1}
	now := time.Now()
	r := bufio.NewReader(strings.NewReader("data"))

	p := Pointers{
		pi: &i, pid: &id, ps: &s, pm: &m, pany: &v, ppi: &pii, parr: &arr,
		pchild: child, ppc: &pc, cp: cp, ip: &ipv, pbuf: buf, phash: &h,
		popts: opts, pplain: plain, ptime: &now, preadr: r,
	}

	p.Reset()

	if i != 0 || id != "" || len(s) != 0 || cap(s) != 2 || len(m) != 0 || v != nil || ii != 0 || ipv != 0 {
		t.Errorf("values behind pointers were not reset: %d %q %v %v %v %d %d", i, id, s, m, v, ii, ipv)
	}

	if p.pi != &i || p.ppi != &pii || pii != &ii {
		t.Errorf("pointers themselves must not change")
	}

	if len(arr[0]) != 0 || cap(arr[0]) != 2 {
		t.Errorf("parr: %v, want slices truncated in place", arr)
	}

	if child.Name != "" || pc.Name != "" || cp.Name != "" || buf.Len() != 0 || opts.n != 0 {
		t.Errorf("Reset was not called through pointers: %q %q %q %d %d", child.Name, pc.Name, cp.Name, buf.Len(), opts.n)
	}

	if empty := sha256.Sum256(nil); !bytes.Equal(h.Sum(nil), empty[:]) {
		t.Errorf("phash: hash was not reset")
	}

	// Структуры без Reset по указателю не трогаются.
	if plain.N != 1 || now.IsZero() || r.Size() == 0 {
		t.Errorf("structs without Reset behind pointers changed: %+v %v, reader size %d", plain, now, r.Size())
	}

	var nilPointers Pointers
	nilPointers.Reset() // nil-указатели пропускаются
}

func TestNested(t *testing.T) {
	var n Nested
	n.child = Child{Name: "child", Items: make([]int, 1, 2)}
	n.plain = Plain{N: 1}
	n.t = time.Now()
	n.buf.WriteString("data")
	n.sb.WriteString("data")
	n.br.Reset(strings.NewReader("data"))
	n.tracker = Tracker{Data: make([]int, 1, 3)}
	n.counter = 5
	n.set = Set{"a": {}}
	n.mu.Lock()
	n.n.Store(1)
	n.opts.n = 1
	n.opts.tags = make([]string, 1, 2)
	n.opts.keep = []int{1}

	n.Reset()

	if n.child.Name != "" || len(n.child.Items) != 0 || cap(n.child.Items) != 2 {
		t.Errorf("child = %+v, want reset by its own method", n.child)
	}

	if n.plain.N != 0 || !n.t.IsZero() || n.buf.Len() != 0 || n.sb.Len() != 0 || n.br.Size() != 0 {
		t.Errorf("value structs were not reset: %+v %v %d %d %d", n.plain, n.t, n.buf.Len(), n.sb.Len(), n.br.Size())
	}

	if n.tracker.Calls != 1 || len(n.tracker.Data) != 0 || cap(n.tracker.Data) != 3 {
		t.Errorf("tracker = %+v, want its Reset called once", n.tracker)
	}

	if n.counter != -1 || len(n.set) != 0 || n.set == nil {
		t.Errorf("counter = %d, set = %v, want their own Reset called", n.counter, n.set)
	}

	if !n.mu.TryLock() || n.n.Load() != 0 {
		t.Errorf("sync values were not zeroed")
	}

	if n.opts.n != 0 || len(n.opts.tags) != 0 || cap(n.opts.tags) != 2 || len(n.opts.keep) != 1 {
		t.Errorf("opts = %+v, want fields reset one by one and keep untouched", n.opts)
	}
}

func TestEmbedded(t *testing.T) {
	plain := &Plain{N: 1}
	tracker := &Tracker{}

	e := Embedded{Child: Child{Name: "child"}, Plain: plain, Tracker: tracker, Reader: strings.NewReader("data")}
	e.Buffer.WriteString("data")

	e.Reset()

	if e.Child.Name != "" || e.Plain != plain || plain.N != 1 || tracker.Calls != 1 || e.Reader != nil || e.Buffer.Len() != 0 {
		t.Errorf("embedded fields: %+v, %+v, %+v, reader %v, buffer %d", e.Child, plain, tracker, e.Reader, e.Buffer.Len())
	}
}

func TestRefs(t *testing.T) {
	h := sha256.New()
	h.Write([]byte("data"))

	r := Refs{
		h: h, r: strings.NewReader("data"), err: errors.New("err"), v: 1,
		ch: make(chan int), fn: func() error { return nil },
	}

	r.Reset()

	if r.h != h {
		t.Errorf("h: the interface with Reset must keep its value")
	}

	if empty := sha256.Sum256(nil); !bytes.Equal(h.Sum(nil), empty[:]) {
		t.Errorf("h: hash was not reset")
	}

	if r.r != nil || r.err != nil || r.v != nil || r.ch != nil || r.fn != nil {
		t.Errorf("refs = %+v, want nil", r)
	}

	var nilRefs Refs
	nilRefs.Reset() // nil-интерфейс с Reset пропускается
}

func TestSkipped(t *testing.T) {
	plain := &Plain{N: 1}
	s := Skipped{keep: []int{1}, log: plain, n: 1}

	s.Reset()

	if len(s.keep) != 1 || s.log != plain || s.n != 0 {
		t.Errorf("skipped = %+v, want keep and log untouched, n reset", s)
	}
}

func TestEmpty(t *testing.T) {
	var e *Empty
	e.Reset()
}

func TestBox(t *testing.T) {
	v := "value"
	b := Box[string, int]{
		items: make([]string, 2, 4),
		index: map[int]string{1: "a"},
		value: "v",
		ptr:   &v,
		pair:  Pair[int, string]{key: 1, val: "a"},
		plain: Holder[string]{v: "h"},
	}

	b.Reset()

	if len(b.items) != 0 || cap(b.items) != 4 || len(b.index) != 0 || b.value != "" || v != "" || b.ptr != &v {
		t.Errorf("box = %+v, *ptr = %q", b, v)
	}

	if b.pair != (Pair[int, string]{}) || b.plain != (Holder[string]{}) {
		t.Errorf("pair = %+v, plain = %+v, want zero values", b.pair, b.plain)
	}
}

func TestPromoted(t *testing.T) {
	var zero Promoted
	zero.Reset() // встроенные nil-указатель и nil-интерфейс не должны вызвать панику

	p := Promoted{
		value: ValueWrap{Tracker: Tracker{Data: []int{1}}, Extra: 1},
		ptr:   PointerWrap{bytes.NewBufferString("data")},
		iface: IfaceWrap{sha256.New()},
		amb:   Ambiguous{Tracker: Tracker{Calls: 5}},
	}

	p.Reset()

	if p.value.Calls != 1 || len(p.value.Data) != 0 || p.value.Extra != 1 {
		t.Errorf("value = %+v, want Tracker.Reset called and Extra kept", p.value)
	}

	if p.ptr.Buffer != nil || p.iface.Hash != nil || p.amb.Calls != 0 {
		t.Errorf("ptr = %+v, iface = %+v, amb.Calls = %d, want zero values", p.ptr, p.iface, p.amb.Calls)
	}
}

func TestReceiver(t *testing.T) {
	r := Receiver{n: 1}
	r.Reset()

	if r.n != 0 {
		t.Errorf("n = %d, want 0", r.n)
	}
}
