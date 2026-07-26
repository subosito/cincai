CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY NOT NULL
);

CREATE TABLE IF NOT EXISTS credentials (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    provider TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('oauth', 'api_key')),
    data TEXT NOT NULL,
    identity_key TEXT,
    hosts_override TEXT,
    inject_override TEXT,
    disabled_cause TEXT,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS gateway_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('static', 'issued')),
    hash TEXT NOT NULL,
    expires_at INTEGER,
    scopes TEXT,
    revoked INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    budget_max_tokens INTEGER,
    budget_window_sec INTEGER
);

-- Append-only chips for rolling token budgets (SUM over window).
CREATE TABLE IF NOT EXISTS key_budget_usage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key_id INTEGER NOT NULL,
    principal_id TEXT NOT NULL DEFAULT '',
    tokens INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_key_budget_usage_key_created
    ON key_budget_usage(key_id, created_at);

CREATE TABLE IF NOT EXISTS snapshot_meta (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    generation INTEGER NOT NULL,
    generated_at INTEGER NOT NULL
);