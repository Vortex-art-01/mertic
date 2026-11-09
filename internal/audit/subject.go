package audit

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	// queueSize — сколько событий может ждать доставки в один приёмник.
	queueSize = 1024

	// updateTimeout ограничивает одну доставку: зависший приёмник не должен
	// держать свою очередь бесконечно.
	updateTimeout = 5 * time.Second

	// closeTimeout — сколько Close ждёт разбора очередей при остановке сервера.
	closeTimeout = 5 * time.Second
)

// Observer — приёмник событий аудита. [Subject] вызывает Update из отдельной
// горутины, по одному событию за раз и в порядке поступления. Контекст
// ограничивает одну доставку пятью секундами, а при остановке отменяется, если
// очередь не успела разобраться (см. [Subject.Close]). Ошибку Update субъект
// пишет в лог; повторов нет.
type Observer interface {
	Update(ctx context.Context, event Event) error
}

// Subject рассылает события наблюдателям. У каждого наблюдателя своя очередь и
// своя горутина: события приходят к нему по порядку, а медленный приёмник не
// задерживает ни соседа, ни хендлер.
type Subject struct {
	log     *slog.Logger
	workers sync.WaitGroup

	// Контекст сервера для доставки не годится: он отменяется в начале
	// остановки, а накопленные события ещё нужно разослать (см. Close).
	ctx    context.Context
	cancel context.CancelFunc

	// mu защищает очереди и closed: Close может прийти одновременно с Register
	// или Notify.
	mu     sync.Mutex
	queues []chan Event
	closed bool
}

// NewSubject возвращает субъект без наблюдателей: пока никто не подписан через
// [Subject.Register], [Subject.Notify] ничего не делает. Ошибки доставки субъект
// пишет в l. Когда события больше не нужны, вызовите [Subject.Close].
func NewSubject(l *slog.Logger) *Subject {
	ctx, cancel := context.WithCancel(context.Background())

	return &Subject{log: l, ctx: ctx, cancel: cancel}
}

// Register подписывает наблюдателя на события. После [Subject.Close] подписаться
// уже нельзя: событий больше не будет, а горутина наблюдателя ждала бы их вечно.
// Такой вызов ничего не делает и оставляет запись в логе.
func (s *Subject) Register(o Observer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		s.log.Error("audit subject is closed, observer ignored",
			slog.String("observer", fmt.Sprintf("%T", o)))
		return
	}

	queue := make(chan Event, queueSize)
	s.queues = append(s.queues, queue)

	// Горутина учитывается в workers под тем же мьютексом, что и очередь. Иначе
	// Close мог бы закрыть очередь и дождаться workers раньше, чем горутина в них
	// попадёт, и вернуть управление, пока она ещё работает.
	s.workers.Go(func() {
		for event := range queue {
			s.deliver(o, event)
		}
	})
}

// Enabled сообщает, есть ли кому рассылать события. Если нет, собирать событие незачем.
func (s *Subject) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return len(s.queues) > 0
}

// Notify ставит событие в очереди наблюдателей и сразу возвращает управление:
// метрика уже принята, и ответ клиенту не должен зависеть от приёмников.
// Поэтому же при переполненной очереди событие не ждёт места, а теряется,
// оставляя запись в логе.
func (s *Subject) Notify(event Event) {
	// Мьютекс держится всю рассылку, чтобы конкурентные хендлеры не перемешали
	// порядок событий от приёмника к приёмнику. Это дёшево: отправка не ждёт.
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, queue := range s.queues {
		select {
		case queue <- event:
		default:
			s.log.Error("audit queue is full, event dropped", slog.Any("event", event))
		}
	}
}

// Close дожидается доставки накопленных событий, но не дольше closeTimeout:
// после него незавершённые доставки отменяются, а события остаются в логе.
// После Close субъект выключен: [Subject.Enabled] возвращает false, события из
// [Subject.Notify] никому не достаются, а [Subject.Register] никого не подписывает.
func (s *Subject) Close() {
	// Хендлер, переживший таймаут srv.Shutdown, может вызвать Notify и после
	// Close. Без очередей его событие просто никому не достанется, а запись
	// в закрытый канал уронила бы сервер.
	s.mu.Lock()
	s.closed = true
	for _, queue := range s.queues {
		close(queue)
	}
	s.queues = nil
	s.mu.Unlock()

	defer s.cancel()

	timer := time.AfterFunc(closeTimeout, s.cancel)
	defer timer.Stop()

	s.workers.Wait()
}

// deliver не возвращает ошибку: метрика уже принята, и отвечать за сбой
// приёмника некому. Само событие уходит в лог, чтобы не пропасть бесследно.
func (s *Subject) deliver(o Observer, event Event) {
	ctx, cancel := context.WithTimeout(s.ctx, updateTimeout)
	defer cancel()

	if err := o.Update(ctx, event); err != nil {
		s.log.Error("failed to deliver audit event",
			slog.Any("event", event), slog.Any("error", err))
	}
}
