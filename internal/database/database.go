// Package database открывает пул соединений с PostgreSQL и приводит схему
// к актуальному виду.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/Vortex-art-01/mertic/internal/pgerrors"
	"github.com/Vortex-art-01/mertic/internal/retry"
	"github.com/Vortex-art-01/mertic/migrations"
)

const (
	maxConns        = 10
	minIdleConns    = 5
	maxConnIdleTime = time.Minute
	connectTimeout  = 5 * time.Second
)

func New(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database dsn: %w", err)
	}

	cfg.MaxConns = maxConns
	cfg.MinIdleConns = minIdleConns
	cfg.MaxConnIdleTime = maxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := prepare(ctx, pool); err != nil {
		pool.Close()

		return nil, err
	}

	return pool, nil
}

func prepare(ctx context.Context, pool *pgxpool.Pool) error {
	if err := connect(ctx, pool); err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}

	return migrate(ctx, pool)
}

func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer func() { _ = db.Close() }()

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

func connect(ctx context.Context, pool *pgxpool.Pool) error {
	return retry.Do(ctx, pgerrors.Retriable, func() error {
		connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
		defer cancel()

		return pool.Ping(connectCtx)
	})
}
