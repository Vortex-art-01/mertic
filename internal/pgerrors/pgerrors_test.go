package pgerrors_test

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Vortex-art-01/mertic/internal/pgerrors"
)

func TestRetriableCodes(t *testing.T) {
	tests := []struct {
		name string
		code string
		want bool
	}{
		{"класс 08: соединение оборвалось", pgerrcode.ConnectionFailure, true},
		{"класс 08: соединение не установлено", pgerrcode.SQLClientUnableToEstablishSQLConnection, true},
		{"класс 40: конфликт транзакций", pgerrcode.SerializationFailure, true},
		{"класс 40: клинч", pgerrcode.DeadlockDetected, true},
		{"класс 53: соединений больше нет", pgerrcode.TooManyConnections, true},
		{"класс 57: база ещё не готова", pgerrcode.CannotConnectNow, true},
		{"класс 57: запрос отменили", pgerrcode.QueryCanceled, false},
		{"класс 23: повтор ключа", pgerrcode.UniqueViolation, false},
		{"класс 23: нарушено NOT NULL", pgerrcode.NotNullViolation, false},
		{"класс 42: ошибка в запросе", pgerrcode.SyntaxError, false},
		{"класс 42: нет такой таблицы", pgerrcode.UndefinedTable, false},
		{"класс 22: не то значение", pgerrcode.DataException, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &pgconn.PgError{Code: tt.code}

			if got := pgerrors.Retriable(err); got != tt.want {
				t.Errorf("Retriable(%s) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

// Ошибка приходит из репозитория обёрнутой — классификатор должен добраться
// до исходной сквозь обёртки.
func TestRetriableUnwrapsError(t *testing.T) {
	pgErr := &pgconn.PgError{Code: pgerrcode.ConnectionException}
	err := fmt.Errorf("save batch: %w", fmt.Errorf("add counters batch: %w", pgErr))

	if !pgerrors.Retriable(err) {
		t.Errorf("Retriable(%v) = false, want true", err)
	}
}

// До базы не достучались: ответа с кодом нет, но повторить стоит — соединение
// может подняться.
func TestRetriableNetworkError(t *testing.T) {
	var err error = &net.OpError{
		Op:  "dial",
		Net: "tcp",
		Err: errors.New("connection refused"),
	}

	if !pgerrors.Retriable(err) {
		t.Errorf("Retriable(%v) = false, want true", err)
	}

	if !pgerrors.Retriable(fmt.Errorf("connect to database: %w", err)) {
		t.Error("Retriable() must see through wrapping, got false")
	}
}

func TestRetriableNonDatabaseErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ошибки нет", nil},
		{"пустой результат", sql.ErrNoRows},
		{"транзакция уже закрыта", sql.ErrTxDone},
		{"ошибка не от базы", errors.New("something went wrong")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if pgerrors.Retriable(tt.err) {
				t.Errorf("Retriable(%v) = true, want false", tt.err)
			}
		})
	}
}
