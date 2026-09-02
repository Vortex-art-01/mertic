-- +goose Up
ALTER TABLE gauges ALTER COLUMN name TYPE varchar(255);

ALTER TABLE counters ALTER COLUMN name TYPE varchar(255);

-- +goose Down
ALTER TABLE counters ALTER COLUMN name TYPE text;

ALTER TABLE gauges ALTER COLUMN name TYPE text;
