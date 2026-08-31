// Package database открывает пул соединений с PostgreSQL и приводит схему
// к актуальному виду.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pressly/goose/v3"

	// Драйвер pgx регистрируется под именем "pgx" и используется
	// через стандартный database/sql.
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Vortex-art-01/mertic/migrations"
)

const (
	maxOpenConns    = 10          // всего соединений (in-use + idle)
	maxIdleConns    = 5           // сколько свободных держим открытыми
	connMaxIdleTime = time.Minute // как долго свободное соединение живёт

	// connectTimeout ограничивает первое обращение к базе: недоступный
	// сервер не должен держать запуск до истечения таймаутов драйвера.
	connectTimeout = 5 * time.Second
)

// New готовит пул соединений. Ни разбор DSN, ни подключение здесь не
// происходят — драйвер откладывает их до первого запроса, поэтому неверная
// строка подключения или недоступная база выяснятся только в Migrate или
// в хендлере GET /ping.
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

// Migrate накатывает недостающие миграции. Свою служебную таблицу с версией
// схемы goose создаёт сам, так что пустой базы достаточно.
func Migrate(ctx context.Context, db *sql.DB) error {
	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	if err := db.PingContext(connectCtx); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	goose.SetBaseFS(migrations.FS)
	// Лог goose уходит в никуда: о результате сервер пишет сам, а os.Stdout
	// занят структурированным логом.
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set migrations dialect: %w", err)
	}

	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
