CREATE TABLE IF NOT EXISTS message_thread_locks (
    thread_id BIGINT PRIMARY KEY,
    locked BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS message_reports (
    id BIGSERIAL PRIMARY KEY,
    message_id BIGINT NOT NULL,
    reporter_id BIGINT NOT NULL,
    reason TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    resolved_by TEXT
);

CREATE INDEX IF NOT EXISTS idx_message_reports_status_created ON message_reports(status, created_at DESC, id DESC);
