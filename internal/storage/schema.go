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

CREATE TABLE IF NOT EXISTS cloudtrail_events (
    event_id      TEXT PRIMARY KEY,
    event_time    TEXT NOT NULL,
    event_name    TEXT NOT NULL,
    principal_id  TEXT NOT NULL,
    user_agent    TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id   TEXT NOT NULL,
    region        TEXT NOT NULL,
    deploy_id     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS deploy_events (
    id           TEXT PRIMARY KEY,
    repo         TEXT NOT NULL,
    branch       TEXT NOT NULL,
    commit_sha   TEXT NOT NULL,
    pr_number    INTEGER,
    title        TEXT NOT NULL,
    started_at   TEXT NOT NULL,
    completed_at TEXT NOT NULL,
    status       TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS correlations (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    anomaly_id  INTEGER NOT NULL,
    deploy_id   TEXT NOT NULL,
    confidence  REAL NOT NULL,
    evidence    TEXT NOT NULL,
    UNIQUE(anomaly_id, deploy_id)
);

CREATE TABLE IF NOT EXISTS sync_cursors (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`
