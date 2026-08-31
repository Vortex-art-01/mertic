// Package database открывает пул соединений с PostgreSQL.
package database

import (
	"database/sql"
	"fmt"
	"time"

	// Драйвер pgx регистрируется под именем "pgx" и используется
	// через стандартный database/sql.
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	maxOpenConns    = 10          // всего соединений (in-use + idle)
	maxIdleConns    = 5           // сколько свободных держим открытыми
	connMaxIdleTime = time.Minute // как долго свободное соединение живёт
)

// New готовит пул соединений. Ни разбор DSN, ни подключение здесь не
// происходят — драйвер откладывает их до первого запроса, поэтому неверная
// строка подключения или недоступная база не мешают серверу подняться.
// Их состояние показывает хендлер GET /ping.
func New(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxIdleTime(connMaxIdleTime)

	return db, nil
}
