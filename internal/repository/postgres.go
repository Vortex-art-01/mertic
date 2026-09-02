package repository

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Vortex-art-01/mertic/internal/model"
	"github.com/Vortex-art-01/mertic/internal/pgerrors"
	"github.com/Vortex-art-01/mertic/internal/retry"
)

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

func (p *Postgres) SaveGauge(ctx context.Context, name string, value float64) error {
	const query = `
		INSERT INTO gauges (name, value) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET value = EXCLUDED.value`

	if err := p.exec(ctx, query, name, value); err != nil {
		return fmt.Errorf("save gauge %s: %w", name, err)
	}

	return nil
}

func (p *Postgres) AddCounter(ctx context.Context, name string, value int64) error {
	const query = `
		INSERT INTO counters (name, delta) VALUES ($1, $2)
		ON CONFLICT (name) DO UPDATE SET delta = counters.delta + EXCLUDED.delta`

	if err := p.exec(ctx, query, name, value); err != nil {
		return fmt.Errorf("add counter %s: %w", name, err)
	}

	return nil
}

func (p *Postgres) exec(ctx context.Context, query string, args ...any) error {
	return retry.Do(ctx, pgerrors.Retriable, func() error {
		_, err := p.pool.Exec(ctx, query, args...)
		return err
	})
}

func (p *Postgres) SaveBatch(ctx context.Context, metrics []model.Metrics) error {
	gauges, counters := splitBatch(metrics)
	if len(gauges) == 0 && len(counters) == 0 {
		return nil
	}

	err := retry.Do(ctx, pgerrors.Retriable, func() error {
		return p.saveBatch(ctx, gauges, counters)
	})
	if err != nil {
		return fmt.Errorf("save batch: %w", err)
	}

	return nil
}

func (p *Postgres) saveBatch(ctx context.Context, gauges map[string]float64, counters map[string]int64) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := saveGauges(ctx, tx, gauges); err != nil {
		return err
	}

	if err := addCounters(ctx, tx, counters); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func saveGauges(ctx context.Context, tx pgx.Tx, gauges map[string]float64) error {
	if len(gauges) == 0 {
		return nil
	}

	const query = `
		INSERT INTO gauges (name, value)
		SELECT * FROM unnest($1::text[], $2::double precision[])
		ON CONFLICT (name) DO UPDATE SET value = EXCLUDED.value`

	names := slices.Sorted(maps.Keys(gauges))
	values := make([]float64, len(names))
	for i, name := range names {
		values[i] = gauges[name]
	}

	if _, err := tx.Exec(ctx, query, names, values); err != nil {
		return fmt.Errorf("save gauges batch: %w", err)
	}

	return nil
}

func addCounters(ctx context.Context, tx pgx.Tx, counters map[string]int64) error {
	if len(counters) == 0 {
		return nil
	}

	const query = `
		INSERT INTO counters (name, delta)
		SELECT * FROM unnest($1::text[], $2::bigint[])
		ON CONFLICT (name) DO UPDATE SET delta = counters.delta + EXCLUDED.delta`

	names := slices.Sorted(maps.Keys(counters))
	deltas := make([]int64, len(names))
	for i, name := range names {
		deltas[i] = counters[name]
	}

	if _, err := tx.Exec(ctx, query, names, deltas); err != nil {
		return fmt.Errorf("add counters batch: %w", err)
	}

	return nil
}

func splitBatch(metrics []model.Metrics) (map[string]float64, map[string]int64) {
	gauges := make(map[string]float64)
	counters := make(map[string]int64)

	for _, m := range metrics {
		switch m.MType {
		case model.Gauge:
			if m.Value != nil {
				gauges[m.ID] = *m.Value
			}
		case model.Counter:
			if m.Delta != nil {
				counters[m.ID] += *m.Delta
			}
		}
	}

	return gauges, counters
}

func (p *Postgres) GetGauge(ctx context.Context, name string) (float64, bool, error) {
	const query = `SELECT value FROM gauges WHERE name = $1`

	var value float64

	err := retry.Do(ctx, pgerrors.Retriable, func() error {
		return p.pool.QueryRow(ctx, query, name).Scan(&value)
	})

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("get gauge %s: %w", name, err)
	}

	return value, true, nil
}

func (p *Postgres) GetCounter(ctx context.Context, name string) (int64, bool, error) {
	const query = `SELECT delta FROM counters WHERE name = $1`

	var delta int64

	err := retry.Do(ctx, pgerrors.Retriable, func() error {
		return p.pool.QueryRow(ctx, query, name).Scan(&delta)
	})

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("get counter %s: %w", name, err)
	}

	return delta, true, nil
}

func (p *Postgres) Gauges(ctx context.Context) (map[string]float64, error) {
	const query = `SELECT name, value FROM gauges`

	var gauges map[string]float64

	err := retry.Do(ctx, pgerrors.Retriable, func() error {
		rows, err := p.pool.Query(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		gauges = make(map[string]float64)

		for rows.Next() {
			var (
				name  string
				value float64
			)
			if err := rows.Scan(&name, &value); err != nil {
				return err
			}
			gauges[name] = value
		}

		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("list gauges: %w", err)
	}

	return gauges, nil
}

func (p *Postgres) Counters(ctx context.Context) (map[string]int64, error) {
	const query = `SELECT name, delta FROM counters`

	var counters map[string]int64

	err := retry.Do(ctx, pgerrors.Retriable, func() error {
		rows, err := p.pool.Query(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()

		counters = make(map[string]int64)

		for rows.Next() {
			var (
				name  string
				delta int64
			)
			if err := rows.Scan(&name, &delta); err != nil {
				return err
			}
			counters[name] = delta
		}

		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("list counters: %w", err)
	}

	return counters, nil
}
