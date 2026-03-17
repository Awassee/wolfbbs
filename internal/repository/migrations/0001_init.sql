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
    conference TEXT NOT NULL DEFAULT 'General',
    read_acs TEXT NOT NULL DEFAULT '',
    write_acs TEXT NOT NULL DEFAULT '',
    created_by BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE boards ADD COLUMN IF NOT EXISTS conference TEXT NOT NULL DEFAULT 'General';
ALTER TABLE boards ADD COLUMN IF NOT EXISTS read_acs TEXT NOT NULL DEFAULT '';
ALTER TABLE boards ADD COLUMN IF NOT EXISTS write_acs TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS messages (
    id BIGSERIAL PRIMARY KEY,
    board_id BIGINT NOT NULL,
    author_id BIGINT NOT NULL,
    parent_id BIGINT,
    thread_id BIGINT,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE messages ADD COLUMN IF NOT EXISTS parent_id BIGINT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS thread_id BIGINT;
CREATE INDEX IF NOT EXISTS idx_messages_board_thread_created ON messages(board_id, thread_id, created_at, id);

CREATE TABLE IF NOT EXISTS message_pointers (
    user_id BIGINT NOT NULL,
    board_id BIGINT NOT NULL,
    last_read_id BIGINT NOT NULL DEFAULT 0,
    last_read_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, board_id)
);
CREATE INDEX IF NOT EXISTS idx_message_pointers_user_board ON message_pointers(user_id, board_id);

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

CREATE TABLE IF NOT EXISTS admin_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor TEXT NOT NULL,
    target TEXT NOT NULL,
    action TEXT NOT NULL,
    details TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mail_outbound_policies (
    handle TEXT PRIMARY KEY,
    outbound_disabled BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS file_areas (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS file_entries (
    id BIGSERIAL PRIMARY KEY,
    area_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    sha256 TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    uploader_id BIGINT NOT NULL DEFAULT 0,
    uploaded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_file_entries_sha256 ON file_entries(sha256) WHERE sha256 <> '';
CREATE INDEX IF NOT EXISTS idx_file_entries_area_uploaded ON file_entries(area_id, uploaded_at DESC);

CREATE TABLE IF NOT EXISTS file_ratings (
    user_id BIGINT NOT NULL,
    file_id BIGINT NOT NULL,
    rating SMALLINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, file_id)
);

CREATE TABLE IF NOT EXISTS file_filters (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    query TEXT NOT NULL DEFAULT '',
    tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_file_filters_user_name ON file_filters(user_id, name);

CREATE TABLE IF NOT EXISTS download_queue (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    file_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, file_id)
);

CREATE TABLE IF NOT EXISTS download_tickets (
    token TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    file_id BIGINT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    used_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS gateway_settings (
    id SMALLINT PRIMARY KEY,
    smtp_host TEXT NOT NULL DEFAULT '',
    smtp_port INTEGER NOT NULL DEFAULT 587,
    smtp_user TEXT NOT NULL DEFAULT '',
    smtp_pass TEXT NOT NULL DEFAULT '',
    from_domain TEXT NOT NULL DEFAULT '',
    max_recipients INTEGER NOT NULL DEFAULT 3,
    max_message_bytes INTEGER NOT NULL DEFAULT 65536,
    web_timeout_sec INTEGER NOT NULL DEFAULT 10,
    web_max_bytes INTEGER NOT NULL DEFAULT 2097152,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS door_configs (
    door_id TEXT PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    daily_turns INTEGER NOT NULL DEFAULT 40,
    time_bank_max INTEGER NOT NULL DEFAULT 200,
    reset_hour_local INTEGER NOT NULL DEFAULT 0,
    required_role_override TEXT NOT NULL DEFAULT '',
    messages_days INTEGER NOT NULL DEFAULT 30,
    logs_days INTEGER NOT NULL DEFAULT 30,
    max_run_seconds INTEGER NOT NULL DEFAULT 180,
    max_output_rate INTEGER NOT NULL DEFAULT 2048,
    allow_network BOOLEAN NOT NULL DEFAULT FALSE,
    allow_fs_write BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS door_user_state (
    user_id BIGINT NOT NULL,
    door_id TEXT NOT NULL,
    state_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, door_id)
);

CREATE TABLE IF NOT EXISTS door_global_state (
    door_id TEXT PRIMARY KEY,
    state_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS door_event_log (
    id BIGSERIAL PRIMARY KEY,
    door_id TEXT NOT NULL,
    user_id BIGINT NOT NULL,
    event_type TEXT NOT NULL,
    payload_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_door_event_log_door_created ON door_event_log(door_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_door_event_log_user_created ON door_event_log(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS door_turn_bank (
    user_id BIGINT NOT NULL,
    door_id TEXT NOT NULL,
    day_key DATE NOT NULL,
    turns_used INTEGER NOT NULL DEFAULT 0,
    time_bank INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, door_id, day_key)
);
CREATE INDEX IF NOT EXISTS idx_door_turn_bank_lookup ON door_turn_bank(user_id, door_id, day_key DESC);

CREATE TABLE IF NOT EXISTS door_achievements (
    door_id TEXT NOT NULL,
    user_id BIGINT NOT NULL,
    achievement_code TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (door_id, user_id, achievement_code)
);

CREATE TABLE IF NOT EXISTS door_scores (
    id BIGSERIAL PRIMARY KEY,
    door_id TEXT NOT NULL,
    user_id BIGINT NOT NULL,
    score_type TEXT NOT NULL,
    value BIGINT NOT NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_door_scores_door_type_value ON door_scores(door_id, score_type, value DESC, created_at ASC);

CREATE TABLE IF NOT EXISTS door_user_meta (
    user_id BIGINT NOT NULL,
    door_id TEXT NOT NULL,
    favorite BOOLEAN NOT NULL DEFAULT FALSE,
    last_played_at TIMESTAMPTZ,
    play_count INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, door_id)
);
CREATE INDEX IF NOT EXISTS idx_door_user_meta_recent ON door_user_meta(user_id, last_played_at DESC);
