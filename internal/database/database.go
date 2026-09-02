// Package database открывает пул соединений с PostgreSQL и приводит схему
// к актуальному виду.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pressly/goose/v3"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/Vortex-art-01/mertic/internal/pgerrors"
	"github.com/Vortex-art-01/mertic/internal/retry"
	"github.com/Vortex-art-01/mertic/migrations"
)

const (
	maxOpenConns    = 10
	maxIdleConns    = 5
	connMaxIdleTime = time.Minute
	connectTimeout  = 5 * time.Second
)

func New(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxIdleTime(connMaxIdleTime)

	if err := prepare(ctx, db); err != nil {
		_ = db.Close()

		return nil, err
	}

	return db, nil
}

func prepare(ctx context.Context, db *sql.DB) error {
	if err := connect(ctx, db); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	return migrate(ctx, db)
}

func migrate(ctx context.Context, db *sql.DB) error {
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set migrations dialect: %w", err)
	}

	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}

func connect(ctx context.Context, db *sql.DB) error {
	return retry.Do(ctx, pgerrors.Retriable, func() error {
		// Таймаут отсчитывается заново на каждой попытке.
		connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
		defer cancel()

		return db.PingContext(connectCtx)
	})
}
