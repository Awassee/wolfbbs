-- WolfBBS bootstrap schema (Postgres style)
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    handle TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    banned BOOLEAN NOT NULL DEFAULT FALSE,
    force_reset BOOLEAN NOT NULL DEFAULT FALSE,
    role TEXT NOT NULL DEFAULT 'user',
    theme TEXT NOT NULL DEFAULT 'retro-amber',
    time_format_24h BOOLEAN NOT NULL DEFAULT TRUE,
    ansi_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    paging_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    totp_secret TEXT,
    recovery_codes TEXT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_handle_lower ON users (LOWER(handle));

CREATE TABLE IF NOT EXISTS boards (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS messages (
    id BIGSERIAL PRIMARY KEY,
    board_id BIGINT NOT NULL,
    author_id BIGINT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS private_mail (
    id BIGSERIAL PRIMARY KEY,
    from_user_id BIGINT NOT NULL,
    to_user_id BIGINT,
    external_to TEXT,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at TIMESTAMPTZ
);
