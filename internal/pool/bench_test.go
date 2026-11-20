package pool_test

import (
	"sync"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/pool"
)

// buffer — объект с памятью размером с тело запроса: на таком видно, во что
// обходится выделять её заново.
type buffer struct {
	data []byte
}

func (b *buffer) Reset() {
	b.data = b.data[:0]
}

func newBuffer() *buffer {
	return &buffer{data: make([]byte, 0, 4<<10)}
}

// last держит последний буфер, чтобы компилятор не разместил его на стеке:
// в настоящем коде объект из пула живёт дольше одного вызова.
var last *buffer

func use(b *buffer) {
	b.data = append(b.data, `{"id":"Alloc","type":"gauge","value":1024}`...)
	last = b
}

// Pool сравнивается с sync.Pool, которым пользовались бы без него, и с новым
// буфером на каждую операцию.
func BenchmarkPool(b *testing.B) {
	b.Run("Pool", func(b *testing.B) {
		p := pool.New(newBuffer)

		b.ReportAllocs()
		for b.Loop() {
			buf := p.Get()
			use(buf)
			p.Put(buf)
		}
	})

	b.Run("sync.Pool", func(b *testing.B) {
		p := sync.Pool{New: func() any { return newBuffer() }}

		b.ReportAllocs()
		for b.Loop() {
			buf := p.Get().(*buffer)
			use(buf)
			buf.Reset()
			p.Put(buf)
		}
	})

	b.Run("new", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			use(newBuffer())
		}
	})
}
