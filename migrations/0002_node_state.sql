CREATE TABLE IF NOT EXISTS node_sessions (
    session_id TEXT PRIMARY KEY,
    node_id INTEGER NOT NULL,
    username TEXT NOT NULL,
    area TEXT NOT NULL,
    remote_addr TEXT NOT NULL DEFAULT '',
    login_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_activity TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_node_sessions_node ON node_sessions(node_id);
CREATE INDEX IF NOT EXISTS idx_node_sessions_updated ON node_sessions(updated_at DESC);

CREATE TABLE IF NOT EXISTS caller_history (
    id BIGSERIAL PRIMARY KEY,
    session_id TEXT NOT NULL,
    node_id INTEGER NOT NULL,
    username TEXT NOT NULL,
    area TEXT NOT NULL,
    remote_addr TEXT NOT NULL DEFAULT '',
    login_at TIMESTAMPTZ NOT NULL,
    logout_at TIMESTAMPTZ NOT NULL,
    duration_seconds BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_caller_history_created ON caller_history(created_at DESC);
