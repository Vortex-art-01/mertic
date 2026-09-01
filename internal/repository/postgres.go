package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Vortex-art-01/mertic/internal/model"
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

// SaveBatch применяет весь пакет за одну транзакцию: в базу попадают либо
// все метрики, либо ни одной.
//
// Повторяющиеся имена схлопываются заранее (см. splitBatch): многострочный
// INSERT ... ON CONFLICT не может изменить одну и ту же строку дважды.
// Приращения counter по-прежнему складывает база, а не сервер, — между
// чтением и записью значение мог бы изменить параллельный запрос.
func (p *Postgres) SaveBatch(ctx context.Context, metrics []model.Metrics) error {
	gauges, counters := splitBatch(metrics)
	if len(gauges) == 0 && len(counters) == 0 {
		return nil
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save batch: %w", err)
	}
	// Откат после успешного Commit — уже не операция: база вернёт
	// sql.ErrTxDone, и ошибка здесь ничего не значит.
	defer func() { _ = tx.Rollback() }()

	if err := saveGauges(ctx, tx, gauges); err != nil {
		return err
	}

	if err := addCounters(ctx, tx, counters); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save batch: %w", err)
	}

	return nil
}

// saveGauges обновляет все gauge одним запросом: имена и значения уезжают
// в базу двумя массивами, а unnest разворачивает их обратно в строки.
//
// Имена отсортированы, чтобы параллельные пакеты брали блокировки строк в
// одном порядке и не вставали в клинч друг с другом.
func saveGauges(ctx context.Context, tx *sql.Tx, gauges map[string]float64) error {
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

	if _, err := tx.ExecContext(ctx, query, names, values); err != nil {
		return fmt.Errorf("save gauges batch: %w", err)
	}

	return nil
}

// addCounters накапливает приращения всех counter одним запросом; про
// порядок имён — см. saveGauges.
func addCounters(ctx context.Context, tx *sql.Tx, counters map[string]int64) error {
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

	if _, err := tx.ExecContext(ctx, query, names, deltas); err != nil {
		return fmt.Errorf("add counters batch: %w", err)
	}

	return nil
}

// splitBatch сводит пакет к двум наборам без повторов: у gauge остаётся
// последнее значение, приращения counter складываются — ровно так же, как
// если бы метрики пришли по одной. Метрики без значения пропускаются:
// хендлер их не пропустит, а восстановление из дампа может.
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
