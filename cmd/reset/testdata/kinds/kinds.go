// Package kinds — поля всех видов, которые различает генератор.
package kinds

import (
	"bufio"
	"bytes"
	"hash"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type (
	ID      string
	Celsius float64
	Flag    bool
	Names   []string
	Index   map[string]int
	Block   [4]byte
	IntPtr  *int
)

// Plain — структура без Reset: по значению обнуляется, по указателю не меняется.
type Plain struct {
	N    int
	Tags []string
}

// Tracker считает вызовы своего Reset: генератор должен вызвать его, а не
// обнулить структуру.
type Tracker struct {
	Calls int
	Data  []int
}

func (t *Tracker) Reset() {
	t.Calls++
	t.Data = t.Data[:0]
}

// Counter — не структура, но со своим Reset: он тоже вызывается.
type Counter int

func (c *Counter) Reset() { *c = -1 }

// Set — мапа с Reset на значении получателя.
type Set map[string]struct{}

func (s Set) Reset() { clear(s) }

// Child — помеченная структура, которую хранят другие.
//
// generate:reset
type Child struct {
	Name  string
	Items []int
}

// ChildPtr — определённый тип-указатель: метод через него вызывается только
// после явного разыменования.
type ChildPtr *Child

// generate:reset
type Primitives struct {
	i    int
	i8   int8
	u64  uint64
	up   uintptr
	f    float64
	c    complex128
	b    bool
	s    string
	r    rune
	by   byte
	ptr  unsafe.Pointer
	id   ID
	temp Celsius
	flag Flag
	d    time.Duration
}

// generate:reset
type Collections struct {
	ints  []int
	names Names
	m     map[string]int
	index Index
}

// generate:reset
type Arrays struct {
	sum      [4]int
	block    Block
	times    [2]time.Time
	plains   [2]Plain
	bufs     [2][]byte
	grid     [2][3][]int
	children [2]*Child
	hashes   [2]hash.Hash
	empty    [0]int
	units    [3]struct{}
}

// generate:reset
type Pointers struct {
	pi     *int
	pid    *ID
	ps     *[]string
	pm     *map[string]int
	pany   *any
	ppi    **int
	parr   *[2][]int
	pchild *Child
	ppc    **Child
	cp     ChildPtr
	ip     IntPtr
	pbuf   *bytes.Buffer
	phash  *hash.Hash
	popts  *struct{ n int }
	pplain *Plain
	ptime  *time.Time
	preadr *bufio.Reader
	pmu    *sync.Mutex
}

// generate:reset
type Nested struct {
	child   Child
	plain   Plain
	t       time.Time
	buf     bytes.Buffer
	sb      strings.Builder
	br      bufio.Reader
	tracker Tracker
	counter Counter
	set     Set
	mu      sync.Mutex
	n       atomic.Int64
	opts    struct {
		n    int
		tags []string
		keep []int `reset:"-"`
	}
	none struct{}
}

// generate:reset
type Embedded struct {
	Child
	*Plain
	*Tracker
	io.Reader
	bytes.Buffer
}

// generate:reset
type Refs struct {
	h   hash.Hash
	r   io.Reader
	err error
	v   any
	ch  chan int
	fn  func() error
}

// generate:reset
type Skipped struct {
	_    int
	keep []int  `reset:"-"`
	log  *Plain `reset:"-"`
	n    int
}

// generate:reset
type Empty struct{}

// generate:reset
type Box[T any, K comparable] struct {
	items []T
	index map[K]T
	value T
	ptr   *T
	pair  Pair[K, T]
	plain Holder[T]
}

// generate:reset
type Pair[K comparable, V any] struct {
	key K
	val V
}

// Holder — обобщённая структура без Reset: обнуляется целиком.
type Holder[T any] struct {
	v T
}

// ValueWrap встраивает Tracker по значению: Reset у неё — метод Tracker.
type ValueWrap struct {
	Tracker
	Extra int
}

// PointerWrap и IfaceWrap получают Reset через встроенные указатель и
// интерфейс. У нулевого значения такой вызов паникует, поэтому генератор их
// обнуляет.
type (
	PointerWrap struct{ *bytes.Buffer }
	IfaceWrap   struct{ hash.Hash }
)

// Ambiguous получает Reset от двух встроенных типов на одной глубине: x.Reset()
// не скомпилируется, поэтому генератор структуру обнуляет.
type Ambiguous struct {
	Tracker
	bytes.Buffer
}

// generate:reset
type Promoted struct {
	value ValueWrap
	ptr   PointerWrap
	iface IfaceWrap
	amb   Ambiguous
}

// Receiver — у типа уже есть метод, и Reset получит то же имя получателя.
//
// generate:reset
type Receiver struct {
	n int
}

func (it *Receiver) String() string { return "receiver" }
