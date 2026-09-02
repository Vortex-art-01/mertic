// Package pgerrors классифицирует ошибки PostgreSQL: какие запросы имеет
// смысл повторить, а какие повторять бессмысленно.
package pgerrors

import (
	"errors"
	"net"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// Retriable сообщает, имеет ли смысл повторить запрос, завершившийся
// ошибкой err. Годится как предикат для retry.Do.
func Retriable(err error) bool {
	if err == nil {
		return false
	}

	// Сервер ответил и назвал причину — решает код ошибки.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return retriableCode(pgErr.Code)
	}

	// Ответа не было: соединение не установилось или оборвалось на
	// полпути. До сервера запрос мог и не дойти, так что повторить стоит.
	var connErr *pgconn.ConnectError
	if errors.As(err, &connErr) {
		return true
	}

	var netErr net.Error

	return errors.As(err, &netErr)
}

// retriableCode разбирает код ошибки по классам. Полный список:
// https://www.postgresql.org/docs/current/errcodes-appendix.html
func retriableCode(code string) bool {
	switch {
	// Класс 08 — ошибки соединения: запрос не дошёл или ответ потерялся.
	case pgerrcode.IsConnectionException(code):
		return true

	// Класс 40 — откат транзакции: наткнулись на параллельный запрос,
	// в одиночку та же транзакция пройдёт.
	case pgerrcode.IsTransactionRollback(code):
		return true

	// Класс 53 — ресурсы кончились: место, память, свободные соединения.
	// Всё это может освободиться к следующей попытке.
	case pgerrcode.IsInsufficientResources(code):
		return true
	}

	// Класс 57 берём выборочно: база останавливается или ещё не готова
	// принимать запросы — повторим, а вот отменённый запрос (QueryCanceled)
	// повторять не за чем.
	switch code {
	case pgerrcode.CannotConnectNow, pgerrcode.AdminShutdown, pgerrcode.CrashShutdown:
		return true
	}

	// Остальное — нарушения ограничений (в том числе
	// pgerrcode.UniqueViolation), синтаксис, права: повтор ничего не изменит.
	return false
}
