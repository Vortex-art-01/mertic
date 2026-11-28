package pool_test

import (
	"bytes"
	"fmt"

	"github.com/Vortex-art-01/mertic/internal/pool"
)

// batch — пакет метрик, который разбирают снова и снова. Метод Reset для
// структуры с меткой generate:reset создаёт утилита cmd/reset; здесь он
// написан так же, как его написала бы она.
type batch struct {
	names  []string
	values map[string]float64
}

func (b *batch) Reset() {
	if b == nil {
		return
	}

	b.names = b.names[:0]
	clear(b.values)
}

func Example() {
	batches := pool.New(func() *batch {
		return &batch{values: make(map[string]float64)}
	})

	b := batches.Get()
	b.names = append(b.names, "Alloc", "HeapAlloc")
	b.values["Alloc"] = 1024
	fmt.Println(len(b.names), len(b.values))

	// Put сбрасывает пакет, поэтому следующий Get получит пустой — неважно,
	// этот же или новый.
	batches.Put(b)

	b = batches.Get()
	fmt.Println(len(b.names), len(b.values))

	// Output:
	// 2 1
	// 0 0
}

// Метод Reset есть и у многих типов стандартной библиотеки. bytes.Buffer после
// него пуст, но сохраняет выделенную память.
func ExampleNew() {
	buffers := pool.New(func() *bytes.Buffer { return new(bytes.Buffer) })

	buf := buffers.Get()
	defer buffers.Put(buf)

	fmt.Fprintf(buf, "%s=%d", "PollCount", 5)
	fmt.Println(buf.String())

	// Output:
	// PollCount=5
}

// Функции создания может и не быть: тогда пустой пул, как sync.Pool без New,
// отдаёт нулевое значение — у указателя это nil. Объект создаёт вызывающий,
// например с ёмкостью, которую знает только он.
func ExampleNew_withoutNewFn() {
	buffers := pool.New[*bytes.Buffer](nil)

	buf := buffers.Get()
	if buf == nil {
		buf = bytes.NewBuffer(make([]byte, 0, 512))
	}
	defer buffers.Put(buf)

	fmt.Println(buf.Cap())

	// Output:
	// 512
}
