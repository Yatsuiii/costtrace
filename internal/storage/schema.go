package storage

const schema = `
CREATE TABLE IF NOT EXISTS cost_data (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    date        TEXT NOT NULL,
    service     TEXT NOT NULL,
    usage_type  TEXT NOT NULL,
    amount      REAL NOT NULL,
    currency    TEXT NOT NULL DEFAULT 'USD',
    UNIQUE(date, service, usage_type)
);

CREATE TABLE IF NOT EXISTS anomalies (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    detected_at     TEXT NOT NULL,
    service         TEXT NOT NULL,
    date            TEXT NOT NULL,
    baseline_amount REAL NOT NULL,
    actual_amount   REAL NOT NULL,
    delta           REAL NOT NULL,
    sigma           REAL NOT NULL
);

CREATE TABLE IF NOT EXISTS sync_cursors (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`
