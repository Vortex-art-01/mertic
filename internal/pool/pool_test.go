package pool_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Vortex-art-01/mertic/internal/pool"
)

// object — объект для тестов. Reset очищает данные, но оставляет память под
// них и считает, сколько раз его вызвали.
type object struct {
	data   []byte
	resets int

	// busy отмечает, что объект сейчас у кого-то на руках.
	busy atomic.Bool
}

func (o *object) Reset() {
	o.data = o.data[:0]
	o.resets++
}

func newObject() *object {
	return &object{}
}

func TestGetCreatesObjectsWhenPoolIsEmpty(t *testing.T) {
	created := 0
	p := pool.New(func() *object {
		created++
		return newObject()
	})

	first, second := p.Get(), p.Get()

	if first == nil || second == nil {
		t.Fatalf("Get() returned nil: %p, %p", first, second)
	}
	if first == second {
		t.Error("Get() returned the same object twice without Put")
	}
	if created != 2 {
		t.Errorf("newFn called %d times, want 2", created)
	}
}

func TestPutResetsObject(t *testing.T) {
	p := pool.New(newObject)

	o := p.Get()
	o.data = append(o.data, "данные прошлого владельца"...)
	p.Put(o)

	// Пользоваться объектом после Put нельзя, но тест проверяет именно его:
	// сбросить объект Put должен сам, ещё до того, как кто-то возьмёт его снова.
	if len(o.data) != 0 {
		t.Errorf("after Put data = %q, want empty", o.data)
	}
	if o.resets != 1 {
		t.Errorf("Put called Reset %d times, want 1", o.resets)
	}
}

// Объект, возвращённый через Put, Get выдаёт снова — сброшенным, но с прежней
// памятью. sync.Pool этого не обещает: с детектором гонок он выбрасывает часть
// объектов, а сборщик мусора — все свободные. Поэтому у теста много попыток.
func TestGetReusesReturnedObjects(t *testing.T) {
	p := pool.New(newObject)

	for range 100 {
		o := p.Get()
		o.data = append(o.data, "данные"...)
		p.Put(o)

		if got := p.Get(); got == o {
			if len(got.data) != 0 {
				t.Errorf("reused object has data %q, want empty", got.data)
			}
			if cap(got.data) == 0 {
				t.Error("reused object lost its memory")
			}

			return
		}
	}

	t.Error("Get() never returned an object passed to Put")
}

// Объект не должен достаться двум горутинам сразу, и каждая получает его
// чистым, кто бы ни пользовался им до неё.
func TestConcurrentGetAndPut(t *testing.T) {
	p := pool.New(newObject)

	var running sync.WaitGroup

	for g := range 16 {
		running.Go(func() {
			for i := range 500 {
				o := p.Get()

				if !o.busy.CompareAndSwap(false, true) {
					t.Error("Get() returned an object that is still in use")
				}
				if len(o.data) != 0 {
					t.Errorf("Get() returned an object with data %q", o.data)
				}

				o.data = fmt.Appendf(o.data, "горутина %d, итерация %d", g, i)

				o.busy.Store(false)
				p.Put(o)
			}
		})
	}

	running.Wait()
}

// set — ссылочный тип без указателя: мапу Reset очищает на месте, поэтому
// хватает получателя-значения.
type set map[string]struct{}

func (s set) Reset() {
	clear(s)
}

func TestPoolOfMaps(t *testing.T) {
	p := pool.New(func() set { return make(set) })

	s := p.Get()
	s["Alloc"] = struct{}{}
	p.Put(s)

	if len(s) != 0 {
		t.Errorf("after Put map has %d keys, want 0", len(s))
	}
}

// Без функции создания пул ведёт себя как sync.Pool без New: пустой отдаёт
// нулевое значение T, а объект, возвращённый через Put, — снова, уже
// сброшенным. Так же работает и нулевое значение Pool.
func TestPoolWithoutNew(t *testing.T) {
	pools := map[string]*pool.Pool[*object]{
		"New(nil)":         pool.New[*object](nil),
		"нулевое значение": new(pool.Pool[*object]),
	}

	for name, p := range pools {
		t.Run(name, func(t *testing.T) {
			if got := p.Get(); got != nil {
				t.Fatalf("Get() on empty pool = %p, want nil", got)
			}

			// sync.Pool не обещает вернуть объект: с детектором гонок он
			// выбрасывает часть из них. Поэтому у теста много попыток.
			for range 100 {
				p.Put(newObject())

				if got := p.Get(); got != nil {
					if got.resets != 1 {
						t.Errorf("returned object was reset %d times, want 1", got.resets)
					}

					return
				}
			}

			t.Error("Get() never returned an object passed to Put")
		})
	}
}

// Пул интерфейсов, как и sync.Pool, nil не хранит: вызвать Reset у него
// нельзя, а следующий Get отдал бы nil вместо нового объекта.
func TestPutSkipsNilInterface(t *testing.T) {
	p := pool.New(func() pool.Resetter { return newObject() })

	p.Put(nil)

	if got := p.Get(); got == nil {
		t.Error("Get() returned nil passed to Put, want a new object")
	}
}

// У пула интерфейсов без функции создания нулевое значение — nil, и его
// можно вернуть в пул, ничего не проверяя.
func TestPoolOfInterfacesWithoutNew(t *testing.T) {
	p := pool.New[pool.Resetter](nil)

	r := p.Get()
	if r != nil {
		t.Fatalf("Get() on empty pool = %v, want nil", r)
	}

	p.Put(r)

	if got := p.Get(); got != nil {
		t.Errorf("Get() after Put(nil) = %v, want nil", got)
	}
}
