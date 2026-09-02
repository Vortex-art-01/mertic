-- +goose Up
CREATE TABLE IF NOT EXISTS gauges
(
    id    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name  text             NOT NULL,
    value double precision NOT NULL
);
ALTER TABLE gauges
    ADD CONSTRAINT gauges_name_key UNIQUE (name);


CREATE TABLE IF NOT EXISTS counters
(
    id    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name  text   NOT NULL,
    delta bigint NOT NULL
);
ALTER TABLE counters
    ADD CONSTRAINT counters_name_key UNIQUE (name);


-- +goose Down
DROP TABLE IF EXISTS counters;

DROP TABLE IF EXISTS gauges;
