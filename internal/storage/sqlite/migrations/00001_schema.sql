-- +goose Up
-- The schema knows no vendor and no metric: a new source is new rows.
-- Time is unix seconds; a Number or Bool lands in num, a Text in text.
CREATE TABLE devices (
    id         TEXT PRIMARY KEY,
    source     TEXT NOT NULL,
    name       TEXT NOT NULL,
    room       TEXT,
    kind       TEXT NOT NULL,
    first_seen INTEGER NOT NULL,
    last_seen  INTEGER NOT NULL
);

CREATE TABLE series (
    id     INTEGER PRIMARY KEY,
    device TEXT NOT NULL REFERENCES devices(id),
    metric TEXT NOT NULL,
    unit   TEXT,
    UNIQUE (device, metric)
);

CREATE TABLE readings (
    series INTEGER NOT NULL REFERENCES series(id),
    at     INTEGER NOT NULL,
    num    REAL,
    text   TEXT,
    PRIMARY KEY (series, at)
) WITHOUT ROWID;

CREATE TABLE events (
    id     INTEGER PRIMARY KEY,
    at     INTEGER NOT NULL,
    device TEXT,
    kind   TEXT NOT NULL,
    detail TEXT
);
CREATE INDEX events_at ON events (at);

CREATE TABLE alerts (
    id         INTEGER PRIMARY KEY,
    rule       TEXT NOT NULL,
    device     TEXT,
    severity   TEXT NOT NULL,
    started_at INTEGER NOT NULL,
    ended_at   INTEGER,
    acked_at   INTEGER,
    message    TEXT
);

CREATE TABLE settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);
