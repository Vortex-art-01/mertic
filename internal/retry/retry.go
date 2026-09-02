// Package retry повторяет операции, завершившиеся временной ошибкой.
package retry

import (
	"context"
	"errors"
	"slices"
	"time"
)

// defaultDelays — паузы перед повторными попытками. Их количество задаёт и
// число самих попыток: три дополнительные сверх первой, всего не больше
// четырёх обращений.
var defaultDelays = []time.Duration{1 * time.Second, 3 * time.Second, 5 * time.Second}

// DefaultDelays возвращает расписание пауз по умолчанию: 1s, 3s, 5s.
// Копия, а не сам срез: расписание общее на всю программу, и менять его
// из-под чужих вызовов нельзя.
func DefaultDelays() []time.Duration {
	return slices.Clone(defaultDelays)
}

// Do выполняет fn с паузами по умолчанию — см. DoWith.
func Do(ctx context.Context, retriable func(error) bool, fn func() error) error {
	return DoWith(ctx, defaultDelays, retriable, fn)
}

func DoWith(ctx context.Context, delays []time.Duration, retriable func(error) bool, fn func() error) error {
	err := fn()

	for _, delay := range delays {
		if err == nil || !retriable(err) {
			return err
		}

		if waitErr := wait(ctx, delay); waitErr != nil {
			return errors.Join(err, waitErr)
		}

		err = fn()
	}

	return err
}

// wait ждёт d или отмены контекста — что наступит раньше.
func wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
