package agent

import (
	"errors"
	"fmt"
	"net/http"
)

// TransportError — запрос не дошёл до сервера или ответ не вернулся:
// соединение не установилось, оборвалось или истёк таймаут. Отдельный тип
// нужен, чтобы отличать такие ошибки от неудачно собранного запроса:
// повторять есть смысл только их.
type TransportError struct {
	URL string
	Err error
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("connect to %s: %v", e.URL, e.Err)
}

// Unwrap открывает исходную ошибку транспорта, чтобы errors.Is и errors.As
// добирались до неё сквозь обёртку.
func (e *TransportError) Unwrap() error {
	return e.Err
}

// StatusError — сервер ответил кодом, отличным от 200.
type StatusError struct {
	Code   int
	Status string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("unexpected status %s", e.Status)
}

// Retriable: 5xx и 429 сервер выдаёт, когда ему сейчас плохо, — тот же
// запрос может пройти позже. Остальные коды повторять бессмысленно:
// от повтора запрос правильнее не станет.
func (e *StatusError) Retriable() bool {
	return e.Code == http.StatusTooManyRequests || e.Code >= http.StatusInternalServerError
}

// retriable сообщает, имеет ли смысл повторить отправку, завершившуюся
// ошибкой err. Годится как предикат для retry.Do.
func retriable(err error) bool {
	var transport *TransportError
	if errors.As(err, &transport) {
		return true
	}

	var status *StatusError
	if errors.As(err, &status) {
		return status.Retriable()
	}

	// Всё прочее — наши собственные промахи: не собрался запрос,
	// не сжалось тело. Повтор их не исправит.
	return false
}
