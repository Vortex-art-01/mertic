package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Postgres хранит метрики в базе: gauge — в таблице gauges, counter — в
// counters. Схему создают миграции (см. пакет migrations).
type Postgres struct {
	db *sql.DB
}

func NewPostgres(db *sql.DB) *Postgres {
	return &Postgres{db: db}
}

// SaveGauge перезаписывает значение: у gauge важно только последнее.
func (p *Postgres) SaveGauge(ctx context.Context, name string, value float64) error {
	const query = `
		INSERT INTO gauges (name, value) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET value = EXCLUDED.value`

	if _, err := p.db.ExecContext(ctx, query, name, value); err != nil {
		return fmt.Errorf("save gauge %s: %w", name, err)
	}

	return nil
}

// AddCounter накапливает приращение прямо в базе: складывать на стороне
// сервера нельзя — между чтением и записью значение может изменить
// параллельный запрос.
func (p *Postgres) AddCounter(ctx context.Context, name string, value int64) error {
	const query = `
		INSERT INTO counters (name, delta) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET delta = counters.delta + EXCLUDED.delta`

	if _, err := p.db.ExecContext(ctx, query, name, value); err != nil {
		return fmt.Errorf("add counter %s: %w", name, err)
	}

	return nil
}

func (p *Postgres) GetGauge(ctx context.Context, name string) (float64, bool, error) {
	const query = `SELECT value FROM gauges WHERE name = $1`

	var value float64
	switch err := p.db.QueryRowContext(ctx, query, name).Scan(&value); {
	case errors.Is(err, sql.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("get gauge %s: %w", name, err)
	}

	return value, true, nil
}

func (p *Postgres) GetCounter(ctx context.Context, name string) (int64, bool, error) {
	const query = `SELECT delta FROM counters WHERE name = $1`

	var delta int64
	switch err := p.db.QueryRowContext(ctx, query, name).Scan(&delta); {
	case errors.Is(err, sql.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("get counter %s: %w", name, err)
	}

	return delta, true, nil
}

func (p *Postgres) Gauges(ctx context.Context) (map[string]float64, error) {
	const query = `SELECT name, value FROM gauges`

	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list gauges: %w", err)
	}
	defer rows.Close()

	gauges := make(map[string]float64)
	for rows.Next() {
		var (
			name  string
			value float64
		)
		if err := rows.Scan(&name, &value); err != nil {
			return nil, fmt.Errorf("list gauges: %w", err)
		}
		gauges[name] = value
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list gauges: %w", err)
	}

	return gauges, nil
}

func (p *Postgres) Counters(ctx context.Context) (map[string]int64, error) {
	const query = `SELECT name, delta FROM counters`

	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list counters: %w", err)
	}
	defer rows.Close()

	counters := make(map[string]int64)
	for rows.Next() {
		var (
			name  string
			delta int64
		)
		if err := rows.Scan(&name, &delta); err != nil {
			return nil, fmt.Errorf("list counters: %w", err)
		}
		counters[name] = delta
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list counters: %w", err)
	}

	return counters, nil
}
