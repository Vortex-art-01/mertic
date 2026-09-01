package retry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Vortex-art-01/mertic/internal/retry"
)

var (
	errTemporary = errors.New("temporary")
	errPermanent = errors.New("permanent")
)

func temporary(err error) bool { return errors.Is(err, errTemporary) }

// noDelays — три повтора без ожидания: расписание проверяется отдельно.
var noDelays = []time.Duration{0, 0, 0}

// failing возвращает функцию, которая падает первые n раз, и счётчик вызовов.
func failing(n int, err error) (func() error, *int) {
	var calls int

	return func() error {
		calls++

		if calls <= n {
			return err
		}

		return nil
	}, &calls
}

func TestDoWithSucceedsWithoutRetries(t *testing.T) {
	fn, calls := failing(0, errTemporary)

	if err := retry.DoWith(t.Context(), noDelays, temporary, fn); err != nil {
		t.Fatalf("DoWith() error = %v, want nil", err)
	}

	if *calls != 1 {
		t.Errorf("called %d times, want 1", *calls)
	}
}

func TestDoWithRetriesUntilSuccess(t *testing.T) {
	fn, calls := failing(2, errTemporary)

	if err := retry.DoWith(t.Context(), noDelays, temporary, fn); err != nil {
		t.Fatalf("DoWith() error = %v, want nil", err)
	}

	if *calls != 3 {
		t.Errorf("called %d times, want 3", *calls)
	}
}

// Расписание задаёт число повторов: попыток на одну больше, чем пауз.
func TestDoWithGivesUpAfterDelays(t *testing.T) {
	fn, calls := failing(100, errTemporary)

	err := retry.DoWith(t.Context(), noDelays, temporary, fn)
	if !errors.Is(err, errTemporary) {
		t.Fatalf("DoWith() error = %v, want %v", err, errTemporary)
	}

	if want := len(noDelays) + 1; *calls != want {
		t.Errorf("called %d times, want %d", *calls, want)
	}
}

// Неповторяемая ошибка возвращается с первой попытки.
func TestDoWithDoesNotRetryPermanentError(t *testing.T) {
	fn, calls := failing(100, errPermanent)

	err := retry.DoWith(t.Context(), noDelays, temporary, fn)
	if !errors.Is(err, errPermanent) {
		t.Fatalf("DoWith() error = %v, want %v", err, errPermanent)
	}

	if *calls != 1 {
		t.Errorf("called %d times, want 1", *calls)
	}
}

// Паузы выдерживаются, а не пропускаются.
func TestDoWithWaitsBetweenAttempts(t *testing.T) {
	delays := []time.Duration{20 * time.Millisecond, 30 * time.Millisecond}
	fn, _ := failing(100, errTemporary)

	start := time.Now()
	_ = retry.DoWith(t.Context(), delays, temporary, fn)

	if want, got := delays[0]+delays[1], time.Since(start); got < want {
		t.Errorf("took %v, want at least %v", got, want)
	}
}

// Отмена контекста прерывает ожидание: оставшиеся попытки не выполняются,
// а к ошибке попытки добавляется причина отмены.
func TestDoWithStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	fn, calls := failing(100, errTemporary)

	err := retry.DoWith(ctx, []time.Duration{time.Hour}, temporary, fn)
	if !errors.Is(err, errTemporary) {
		t.Errorf("DoWith() error = %v, want it to wrap %v", err, errTemporary)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("DoWith() error = %v, want it to wrap %v", err, context.Canceled)
	}

	if *calls != 1 {
		t.Errorf("called %d times, want 1", *calls)
	}
}

// По умолчанию — три дополнительные попытки с паузами 1s, 3s, 5s.
func TestDefaultDelays(t *testing.T) {
	want := []time.Duration{time.Second, 3 * time.Second, 5 * time.Second}

	got := retry.DefaultDelays()
	if len(got) != len(want) {
		t.Fatalf("DefaultDelays() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("DefaultDelays()[%d] = %v, want %v", i, got[i], want[i])
		}
	}

	// Расписание общее на всю программу: вызывающий получает копию и
	// испортить его не может.
	got[0] = time.Hour
	if retry.DefaultDelays()[0] != want[0] {
		t.Error("DefaultDelays() returns the shared schedule, want a copy")
	}
}

// Do — то же самое с расписанием по умолчанию; повторов здесь не будет,
// иначе тест ждал бы настоящие секунды.
func TestDoUsesDefaultDelays(t *testing.T) {
	fn, calls := failing(0, errTemporary)

	if err := retry.Do(t.Context(), temporary, fn); err != nil {
		t.Fatalf("Do() error = %v, want nil", err)
	}

	if *calls != 1 {
		t.Errorf("called %d times, want 1", *calls)
	}
}
