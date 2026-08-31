-- +goose Up
CREATE TABLE IF NOT EXISTS gauges (
    name  text NOT NULL PRIMARY KEY,
    value double precision NOT NULL
);

CREATE TABLE IF NOT EXISTS counters (
    name  text NOT NULL PRIMARY KEY,
    delta bigint NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS counters;

DROP TABLE IF EXISTS gauges;
