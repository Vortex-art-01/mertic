package audit

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// recorder запоминает события и отвечает заданной ошибкой.
type recorder struct {
	mu     sync.Mutex
	events []Event
	err    error
}

func (r *recorder) Update(_ context.Context, event Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, event)

	return r.err
}

// timestamps возвращает TS полученных событий: в тестах они служат номерами.
func (r *recorder) timestamps() []int64 {
	r.mu.Lock()
	defer r.mu.Unlock()

	got := make([]int64, 0, len(r.events))
	for _, event := range r.events {
		got = append(got, event.TS)
	}

	return got
}

// blocked держит каждую доставку, пока её не отпустят или не отменят.
type blocked struct {
	recorder
	release chan struct{}
}

func (b *blocked) Update(ctx context.Context, event Event) error {
	select {
	case <-b.release:
		return b.recorder.Update(ctx, event)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Хендлер не собирает событие, пока его некому отправить.
func TestSubjectEnabledOnlyWithObservers(t *testing.T) {
	subject := NewSubject(slog.New(slog.DiscardHandler))

	if subject.Enabled() {
		t.Error("Enabled() = true without observers")
	}

	subject.Register(&recorder{})

	if !subject.Enabled() {
		t.Error("Enabled() = false with an observer")
	}

	subject.Close()

	if subject.Enabled() {
		t.Error("Enabled() = true after Close")
	}
}

func TestSubjectNotifiesEveryObserverInOrder(t *testing.T) {
	first, second := &recorder{}, &recorder{}

	subject := NewSubject(slog.New(slog.DiscardHandler))
	subject.Register(first)
	subject.Register(second)

	for ts := range int64(3) {
		subject.Notify(Event{TS: ts})
	}

	subject.Close()

	want := []int64{0, 1, 2}
	if got := first.timestamps(); !slices.Equal(got, want) {
		t.Errorf("first observer got %v, want %v", got, want)
	}
	if got := second.timestamps(); !slices.Equal(got, want) {
		t.Errorf("second observer got %v, want %v", got, want)
	}
}

// Конкурентные хендлеры не должны менять порядок от приёмника к приёмнику:
// иначе журналы одного аудита нельзя будет сверить построчно.
func TestSubjectKeepsSameOrderForEveryObserver(t *testing.T) {
	const senders, perSender = 8, 100

	first, second := &recorder{}, &recorder{}

	subject := NewSubject(slog.New(slog.DiscardHandler))
	subject.Register(first)
	subject.Register(second)

	var running sync.WaitGroup
	for sender := range int64(senders) {
		running.Go(func() {
			for i := range int64(perSender) {
				subject.Notify(Event{TS: sender*perSender + i})
			}
		})
	}
	running.Wait()

	subject.Close()

	got := first.timestamps()
	if len(got) != senders*perSender {
		t.Fatalf("first observer got %d events, want %d", len(got), senders*perSender)
	}
	if !slices.Equal(got, second.timestamps()) {
		t.Error("observers got the same events in a different order")
	}
}

// Без приёмников аудит выключен: уведомлять некого, но и падать не из-за чего.
func TestSubjectWithoutObservers(t *testing.T) {
	subject := NewSubject(slog.New(slog.DiscardHandler))

	subject.Notify(Event{TS: 1})
	subject.Close()
}

// Сбой одного приёмника не должен мешать другому, а недоставленное событие
// не должно пропасть бесследно.
func TestSubjectFailingObserverDoesNotAffectOthers(t *testing.T) {
	var logs bytes.Buffer

	failing := &recorder{err: errors.New("disk is full")}
	healthy := &recorder{}

	subject := NewSubject(slog.New(slog.NewJSONHandler(&logs, nil)))
	subject.Register(failing)
	subject.Register(healthy)

	subject.Notify(Event{TS: 1, Metrics: []string{"Alloc"}})
	subject.Close()

	if got := healthy.timestamps(); !slices.Equal(got, []int64{1}) {
		t.Errorf("healthy observer got %v, want [1]", got)
	}
	for _, want := range []string{"failed to deliver audit event", "disk is full", "Alloc"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log must contain %q, got:\n%s", want, logs.String())
		}
	}
}

// Медленный приёмник не должен задерживать ни хендлер, ни соседний приёмник.
func TestNotifyDoesNotWaitForSlowObserver(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		slow := &blocked{release: make(chan struct{})}
		healthy := &recorder{}

		subject := NewSubject(slog.New(slog.DiscardHandler))
		subject.Register(slow)
		subject.Register(healthy)

		start := time.Now()

		subject.Notify(Event{TS: 1})
		synctest.Wait()

		if elapsed := time.Since(start); elapsed != 0 {
			t.Errorf("Notify waited for %v", elapsed)
		}
		if got := healthy.timestamps(); !slices.Equal(got, []int64{1}) {
			t.Errorf("healthy observer got %v, want [1]", got)
		}

		close(slow.release)
		subject.Close()
	})
}

// Приёмник не успевает, и очередь заполнена: Notify не ждёт, а теряет событие,
// оставляя о нём запись в логе.
func TestNotifyDropsEventWhenQueueIsFull(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var logs bytes.Buffer

		slow := &blocked{release: make(chan struct{})}

		subject := NewSubject(slog.New(slog.NewJSONHandler(&logs, nil)))
		subject.Register(slow)

		// Первое событие горутина наблюдателя забирает из очереди сразу.
		subject.Notify(Event{TS: 0})
		synctest.Wait()

		for ts := range int64(queueSize) {
			subject.Notify(Event{TS: ts + 1})
		}
		subject.Notify(Event{TS: -1, Metrics: []string{"Dropped"}})

		if got := strings.Count(logs.String(), "audit queue is full"); got != 1 {
			t.Errorf("dropped events logged %d times, want 1:\n%s", got, logs.String())
		}
		if !strings.Contains(logs.String(), "Dropped") {
			t.Errorf("log must contain the dropped event, got:\n%s", logs.String())
		}

		close(slow.release)
		subject.Close()

		got := slow.timestamps()
		if len(got) != queueSize+1 {
			t.Errorf("observer got %d events, want %d", len(got), queueSize+1)
		}
		if slices.Contains(got, -1) {
			t.Error("observer got the event that must have been dropped")
		}
	})
}

// Close дожидается не только текущей доставки, но и всей очереди.
func TestCloseDeliversQueuedEvents(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		slow := &blocked{release: make(chan struct{})}

		subject := NewSubject(slog.New(slog.DiscardHandler))
		subject.Register(slow)

		for ts := range int64(3) {
			subject.Notify(Event{TS: ts})
		}

		time.AfterFunc(time.Second, func() { close(slow.release) })

		subject.Close()

		if got := slow.timestamps(); !slices.Equal(got, []int64{0, 1, 2}) {
			t.Errorf("observer got %v, want [0 1 2]", got)
		}
	})
}

// Зависший приёмник не должен держать остановку сервера, а события, которые
// он так и не принял, должны остаться в логе.
func TestCloseCancelsHungObserver(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var logs bytes.Buffer

		hung := &blocked{release: make(chan struct{})}

		subject := NewSubject(slog.New(slog.NewJSONHandler(&logs, nil)))
		subject.Register(hung)

		for ts := range int64(3) {
			subject.Notify(Event{TS: ts})
		}

		start := time.Now()

		subject.Close()

		if elapsed := time.Since(start); elapsed > closeTimeout {
			t.Errorf("Close took %v, want at most %v", elapsed, closeTimeout)
		}
		if got := strings.Count(logs.String(), "failed to deliver audit event"); got != 3 {
			t.Errorf("lost events logged %d times, want 3:\n%s", got, logs.String())
		}
	})
}

// Один зависший запрос не должен навсегда занимать очередь приёмника.
func TestDeliveryIsLimitedByTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var logs bytes.Buffer

		hung := &blocked{release: make(chan struct{})}

		subject := NewSubject(slog.New(slog.NewJSONHandler(&logs, nil)))
		subject.Register(hung)

		subject.Notify(Event{TS: 1})

		time.Sleep(updateTimeout)
		synctest.Wait()

		if !strings.Contains(logs.String(), context.DeadlineExceeded.Error()) {
			t.Errorf("delivery must have timed out, got log:\n%s", logs.String())
		}

		subject.Close()
	})
}

// Хендлер может пережить таймаут srv.Shutdown и вызвать Notify уже после Close:
// падать из-за него сервер не должен.
func TestNotifyAfterCloseIsIgnored(t *testing.T) {
	observer := &recorder{}

	subject := NewSubject(slog.New(slog.DiscardHandler))
	subject.Register(observer)
	subject.Close()

	subject.Notify(Event{TS: 1})

	if got := observer.timestamps(); len(got) != 0 {
		t.Errorf("observer got %v after Close, want nothing", got)
	}
}
