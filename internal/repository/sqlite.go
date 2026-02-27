package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"wolfbbs/internal/domain"
)

func ensureSQLiteSchema(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  handle TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  banned INTEGER NOT NULL DEFAULT 0,
  force_reset INTEGER NOT NULL DEFAULT 0,
  role TEXT NOT NULL DEFAULT 'user',
  theme TEXT NOT NULL DEFAULT 'retro-amber',
  time_format_24h INTEGER NOT NULL DEFAULT 1,
  ansi_enabled INTEGER NOT NULL DEFAULT 1,
  paging_enabled INTEGER NOT NULL DEFAULT 1,
  verified INTEGER NOT NULL DEFAULT 0,
  totp_secret TEXT NOT NULL DEFAULT '',
  recovery_codes TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  last_login_at TEXT
)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_users_handle_lower ON users (lower(handle))`,
		`CREATE TABLE IF NOT EXISTS boards (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  conference TEXT NOT NULL DEFAULT 'General',
  read_acs TEXT NOT NULL DEFAULT '',
  write_acs TEXT NOT NULL DEFAULT '',
  created_by INTEGER,
  created_at TEXT NOT NULL
)`,
		`ALTER TABLE boards ADD COLUMN conference TEXT NOT NULL DEFAULT 'General'`,
		`ALTER TABLE boards ADD COLUMN read_acs TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE boards ADD COLUMN write_acs TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  board_id INTEGER NOT NULL,
  author_id INTEGER NOT NULL,
  parent_id INTEGER,
  thread_id INTEGER,
  subject TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at TEXT NOT NULL
)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_board_thread_created ON messages(board_id, thread_id, created_at, id)`,
		`CREATE TABLE IF NOT EXISTS message_pointers (
  user_id INTEGER NOT NULL,
  board_id INTEGER NOT NULL,
  last_read_id INTEGER NOT NULL DEFAULT 0,
  last_read_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (user_id, board_id)
)`,
		`CREATE INDEX IF NOT EXISTS idx_message_pointers_user_board ON message_pointers(user_id, board_id)`,
		`CREATE TABLE IF NOT EXISTS private_mail (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  from_user_id INTEGER NOT NULL,
  to_user_id INTEGER,
  external_to TEXT,
  subject TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at TEXT NOT NULL,
  read_at TEXT
)`,
		`CREATE TABLE IF NOT EXISTS admin_audit_logs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  actor TEXT NOT NULL,
  target TEXT NOT NULL,
  action TEXT NOT NULL,
  details TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS mail_outbound_policies (
  handle TEXT PRIMARY KEY,
  outbound_disabled INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS file_areas (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  path TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS file_entries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  area_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  path TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  tags_json TEXT NOT NULL DEFAULT '[]',
  sha256 TEXT NOT NULL DEFAULT '',
  size_bytes INTEGER NOT NULL DEFAULT 0,
  uploader_id INTEGER NOT NULL DEFAULT 0,
  uploaded_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_file_entries_sha256 ON file_entries(sha256) WHERE sha256 <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_file_entries_area_uploaded ON file_entries(area_id, uploaded_at DESC)`,
		`CREATE TABLE IF NOT EXISTS file_ratings (
  user_id INTEGER NOT NULL,
  file_id INTEGER NOT NULL,
  rating INTEGER NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (user_id, file_id)
)`,
		`CREATE TABLE IF NOT EXISTS file_filters (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  query TEXT NOT NULL DEFAULT '',
  tags_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
)`,
		`CREATE INDEX IF NOT EXISTS idx_file_filters_user_name ON file_filters(user_id, name)`,
		`CREATE TABLE IF NOT EXISTS download_queue (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  file_id INTEGER NOT NULL,
  created_at TEXT NOT NULL
)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_download_queue_user_file ON download_queue(user_id, file_id)`,
		`CREATE TABLE IF NOT EXISTS download_tickets (
  token TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL,
  file_id INTEGER NOT NULL,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  used_at TEXT
)`,
		`CREATE TABLE IF NOT EXISTS gateway_settings (
  id INTEGER PRIMARY KEY,
  smtp_host TEXT NOT NULL DEFAULT '',
  smtp_port INTEGER NOT NULL DEFAULT 587,
  smtp_user TEXT NOT NULL DEFAULT '',
  smtp_pass TEXT NOT NULL DEFAULT '',
  from_domain TEXT NOT NULL DEFAULT '',
  max_recipients INTEGER NOT NULL DEFAULT 3,
  max_message_bytes INTEGER NOT NULL DEFAULT 65536,
  web_timeout_sec INTEGER NOT NULL DEFAULT 10,
  web_max_bytes INTEGER NOT NULL DEFAULT 2097152,
  updated_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS system_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS node_sessions (
  session_id TEXT PRIMARY KEY,
  node_id INTEGER NOT NULL,
  username TEXT NOT NULL,
  area TEXT NOT NULL,
  remote_addr TEXT NOT NULL DEFAULT '',
  login_at TEXT NOT NULL,
  last_activity TEXT NOT NULL,
  updated_at TEXT NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS caller_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  node_id INTEGER NOT NULL,
  username TEXT NOT NULL,
  area TEXT NOT NULL,
  remote_addr TEXT NOT NULL DEFAULT '',
  login_at TEXT NOT NULL,
  logout_at TEXT NOT NULL,
  duration_seconds INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
)`,
		`CREATE INDEX IF NOT EXISTS idx_caller_history_created ON caller_history(created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS password_reset_tokens (
  token_hash TEXT PRIMARY KEY,
  handle TEXT NOT NULL,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  consumed_at TEXT
)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS message_search USING fts5(subject, body, content='messages', content_rowid='id')`,
		`CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
  INSERT INTO message_search(rowid, subject, body) VALUES (new.id, new.subject, new.body);
END`,
		`CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
  INSERT INTO message_search(message_search, rowid, subject, body) VALUES ('delete', old.id, old.subject, old.body);
END`,
		`CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
  INSERT INTO message_search(message_search, rowid, subject, body) VALUES ('delete', old.id, old.subject, old.body);
  INSERT INTO message_search(rowid, subject, body) VALUES (new.id, new.subject, new.body);
END`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS file_area_search USING fts5(name, path, description, content='file_areas', content_rowid='id')`,
		`CREATE TRIGGER IF NOT EXISTS file_areas_ai AFTER INSERT ON file_areas BEGIN
  INSERT INTO file_area_search(rowid, name, path, description) VALUES (new.id, new.name, new.path, new.description);
END`,
		`CREATE TRIGGER IF NOT EXISTS file_areas_ad AFTER DELETE ON file_areas BEGIN
  INSERT INTO file_area_search(file_area_search, rowid, name, path, description) VALUES ('delete', old.id, old.name, old.path, old.description);
END`,
		`CREATE TRIGGER IF NOT EXISTS file_areas_au AFTER UPDATE ON file_areas BEGIN
  INSERT INTO file_area_search(file_area_search, rowid, name, path, description) VALUES ('delete', old.id, old.name, old.path, old.description);
  INSERT INTO file_area_search(rowid, name, path, description) VALUES (new.id, new.name, new.path, new.description);
END`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate column name") {
				continue
			}
			return err
		}
	}
	return nil
}

type SQLiteUserRepository struct {
	db *sql.DB
}

func NewSQLiteUserRepository(db *sql.DB) *SQLiteUserRepository {
	return &SQLiteUserRepository{db: db}
}

func (r *SQLiteUserRepository) Create(user *domain.User) error {
	if user == nil {
		return errors.New("user is required")
	}
	handle := strings.TrimSpace(user.Handle)
	if handle == "" {
		return errors.New("handle is required")
	}
	user.Handle = handle
	if user.Theme == "" {
		user.Theme = "retro-amber"
	}
	now := time.Now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = now
	}
	res, err := r.db.ExecContext(context.Background(), `
INSERT INTO users (
  handle, password_hash, enabled, banned, force_reset, role, theme,
  time_format_24h, ansi_enabled, paging_enabled, verified, totp_secret, recovery_codes, created_at, updated_at, last_login_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.Handle, user.PasswordHash, boolToInt(user.Enabled), boolToInt(user.Banned), boolToInt(user.ForceReset), strings.TrimSpace(user.Role), strings.TrimSpace(user.Theme),
		boolToInt(user.TimeFormat24h), boolToInt(user.ANSIEnabled), boolToInt(user.PagingEnabled), boolToInt(user.Verified),
		strings.TrimSpace(user.TOTPSecret), encodeStringSlice(user.RecoveryCodes), formatSQLiteTime(user.CreatedAt), formatSQLiteTime(user.UpdatedAt), nullableSQLiteTime(user.LastLoginAt))
	if err != nil {
		if sqliteConstraintErr(err) {
			return errors.New("handle already exists")
		}
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	user.ID = id
	return nil
}

func (r *SQLiteUserRepository) GetByHandle(handle string) (*domain.User, error) {
	var user domain.User
	var enabled, banned, forceReset, tf24, ansi, paging, verified int
	var recovery string
	var createdAt, updatedAt string
	var lastLogin sql.NullString
	err := r.db.QueryRowContext(context.Background(), `
SELECT id, handle, password_hash, enabled, banned, force_reset, role, theme, time_format_24h, ansi_enabled, paging_enabled,
       verified, totp_secret, recovery_codes, created_at, updated_at, last_login_at
FROM users
WHERE lower(handle) = lower(?)
LIMIT 1`, strings.TrimSpace(handle)).
		Scan(&user.ID, &user.Handle, &user.PasswordHash, &enabled, &banned, &forceReset, &user.Role, &user.Theme, &tf24, &ansi, &paging, &verified, &user.TOTPSecret, &recovery, &createdAt, &updatedAt, &lastLogin)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	user.Enabled = intToBool(enabled)
	user.Banned = intToBool(banned)
	user.ForceReset = intToBool(forceReset)
	user.TimeFormat24h = intToBool(tf24)
	user.ANSIEnabled = intToBool(ansi)
	user.PagingEnabled = intToBool(paging)
	user.Verified = intToBool(verified)
	user.RecoveryCodes = decodeStringSlice(recovery)
	user.CreatedAt = parseSQLiteTime(createdAt)
	user.UpdatedAt = parseSQLiteTime(updatedAt)
	if lastLogin.Valid && strings.TrimSpace(lastLogin.String) != "" {
		ts := parseSQLiteTime(lastLogin.String)
		user.LastLoginAt = &ts
	}
	copy := user
	return &copy, nil
}

func (r *SQLiteUserRepository) Update(user *domain.User) error {
	if user == nil {
		return errors.New("user is required")
	}
	user.UpdatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(context.Background(), `
UPDATE users
SET password_hash = ?, enabled = ?, banned = ?, force_reset = ?, role = ?, theme = ?,
    time_format_24h = ?, ansi_enabled = ?, paging_enabled = ?, verified = ?, totp_secret = ?,
    recovery_codes = ?, updated_at = ?, last_login_at = ?
WHERE id = ?`,
		user.PasswordHash, boolToInt(user.Enabled), boolToInt(user.Banned), boolToInt(user.ForceReset), strings.TrimSpace(user.Role), strings.TrimSpace(user.Theme),
		boolToInt(user.TimeFormat24h), boolToInt(user.ANSIEnabled), boolToInt(user.PagingEnabled), boolToInt(user.Verified), strings.TrimSpace(user.TOTPSecret),
		encodeStringSlice(user.RecoveryCodes), formatSQLiteTime(user.UpdatedAt), nullableSQLiteTime(user.LastLoginAt), user.ID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *SQLiteUserRepository) List() ([]domain.User, error) {
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, handle, password_hash, enabled, banned, force_reset, role, theme, time_format_24h, ansi_enabled, paging_enabled,
       verified, totp_secret, recovery_codes, created_at, updated_at, last_login_at
FROM users
ORDER BY handle COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.User, 0)
	for rows.Next() {
		var user domain.User
		var enabled, banned, forceReset, tf24, ansi, paging, verified int
		var recovery string
		var createdAt, updatedAt string
		var lastLogin sql.NullString
		if err := rows.Scan(&user.ID, &user.Handle, &user.PasswordHash, &enabled, &banned, &forceReset, &user.Role, &user.Theme, &tf24, &ansi, &paging, &verified, &user.TOTPSecret, &recovery, &createdAt, &updatedAt, &lastLogin); err != nil {
			return nil, err
		}
		user.Enabled = intToBool(enabled)
		user.Banned = intToBool(banned)
		user.ForceReset = intToBool(forceReset)
		user.TimeFormat24h = intToBool(tf24)
		user.ANSIEnabled = intToBool(ansi)
		user.PagingEnabled = intToBool(paging)
		user.Verified = intToBool(verified)
		user.RecoveryCodes = decodeStringSlice(recovery)
		user.CreatedAt = parseSQLiteTime(createdAt)
		user.UpdatedAt = parseSQLiteTime(updatedAt)
		if lastLogin.Valid && strings.TrimSpace(lastLogin.String) != "" {
			ts := parseSQLiteTime(lastLogin.String)
			user.LastLoginAt = &ts
		}
		out = append(out, user)
	}
	return out, rows.Err()
}

type SQLiteBoardRepository struct {
	db *sql.DB
}

func NewSQLiteBoardRepository(db *sql.DB) *SQLiteBoardRepository {
	return &SQLiteBoardRepository{db: db}
}

func (r *SQLiteBoardRepository) Create(board *domain.Board) error {
	if board == nil {
		return errors.New("board is required")
	}
	board.Name = strings.TrimSpace(board.Name)
	if board.Name == "" {
		return errors.New("board name is required")
	}
	if board.CreatedAt.IsZero() {
		board.CreatedAt = time.Now().UTC()
	}
	res, err := r.db.ExecContext(context.Background(), `
INSERT INTO boards(name, description, conference, read_acs, write_acs, created_by, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		board.Name,
		strings.TrimSpace(board.Description),
		defaultConference(board.Conference),
		strings.TrimSpace(board.ReadACS),
		strings.TrimSpace(board.WriteACS),
		board.CreatedBy,
		formatSQLiteTime(board.CreatedAt))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	board.ID = id
	return nil
}

func (r *SQLiteBoardRepository) Get(id int64) (*domain.Board, error) {
	var board domain.Board
	var createdAt string
	err := r.db.QueryRowContext(context.Background(), `
SELECT id, name, description, COALESCE(conference, 'General'), COALESCE(read_acs, ''), COALESCE(write_acs, ''), COALESCE(created_by, 0), created_at
FROM boards WHERE id = ?`, id).Scan(&board.ID, &board.Name, &board.Description, &board.Conference, &board.ReadACS, &board.WriteACS, &board.CreatedBy, &createdAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	board.CreatedAt = parseSQLiteTime(createdAt)
	copy := board
	return &copy, nil
}

func (r *SQLiteBoardRepository) List() ([]domain.Board, error) {
	rows, err := r.db.QueryContext(context.Background(), `SELECT id, name, description, COALESCE(conference, 'General'), COALESCE(read_acs, ''), COALESCE(write_acs, ''), COALESCE(created_by, 0), created_at FROM boards ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Board, 0)
	for rows.Next() {
		var board domain.Board
		var createdAt string
		if err := rows.Scan(&board.ID, &board.Name, &board.Description, &board.Conference, &board.ReadACS, &board.WriteACS, &board.CreatedBy, &createdAt); err != nil {
			return nil, err
		}
		board.CreatedAt = parseSQLiteTime(createdAt)
		out = append(out, board)
	}
	return out, rows.Err()
}

func (r *SQLiteBoardRepository) Update(board *domain.Board) error {
	if board == nil {
		return errors.New("board is required")
	}
	board.Name = strings.TrimSpace(board.Name)
	if board.Name == "" {
		return errors.New("board name is required")
	}
	res, err := r.db.ExecContext(context.Background(), `
UPDATE boards
SET name = ?, description = ?, conference = ?, read_acs = ?, write_acs = ?
WHERE id = ?`,
		board.Name,
		strings.TrimSpace(board.Description),
		defaultConference(board.Conference),
		strings.TrimSpace(board.ReadACS),
		strings.TrimSpace(board.WriteACS),
		board.ID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteBoardRepository) Delete(id int64) error {
	res, err := r.db.ExecContext(context.Background(), `DELETE FROM boards WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

type SQLiteMessageRepository struct {
	db *sql.DB
}

func NewSQLiteMessageRepository(db *sql.DB) *SQLiteMessageRepository {
	return &SQLiteMessageRepository{db: db}
}

func (r *SQLiteMessageRepository) CreateMessage(msg *domain.Message) error {
	if msg == nil {
		return errors.New("message is required")
	}
	if msg.BoardID <= 0 {
		return errors.New("board id is required")
	}
	if msg.AuthorID <= 0 {
		return errors.New("author id is required")
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	var parentID interface{}
	var threadID interface{}
	if msg.ParentID > 0 {
		var parentBoardID int64
		var parentThread sql.NullInt64
		err := r.db.QueryRowContext(context.Background(), `SELECT board_id, thread_id FROM messages WHERE id = ?`, msg.ParentID).Scan(&parentBoardID, &parentThread)
		if err == sql.ErrNoRows {
			return errors.New("parent message not found")
		}
		if err != nil {
			return err
		}
		if parentBoardID != msg.BoardID {
			return errors.New("parent message not in board")
		}
		parentID = msg.ParentID
		if parentThread.Valid && parentThread.Int64 > 0 {
			msg.ThreadID = parentThread.Int64
		} else {
			msg.ThreadID = msg.ParentID
		}
	}
	if msg.ThreadID > 0 {
		threadID = msg.ThreadID
	}
	res, err := r.db.ExecContext(context.Background(), `
INSERT INTO messages(board_id, author_id, parent_id, thread_id, subject, body, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		msg.BoardID, msg.AuthorID, parentID, threadID, strings.TrimSpace(msg.Subject), msg.Body, formatSQLiteTime(msg.CreatedAt))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	msg.ID = id
	if msg.ThreadID <= 0 {
		msg.ThreadID = msg.ID
		if _, err := r.db.ExecContext(context.Background(), `UPDATE messages SET thread_id = ? WHERE id = ?`, msg.ThreadID, msg.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *SQLiteMessageRepository) GetMessage(id int64) (*domain.Message, error) {
	var msg domain.Message
	var parentID sql.NullInt64
	var threadID sql.NullInt64
	var createdAt string
	err := r.db.QueryRowContext(context.Background(), `
SELECT id, board_id, author_id, parent_id, thread_id, subject, body, created_at
FROM messages
WHERE id = ?`, id).Scan(&msg.ID, &msg.BoardID, &msg.AuthorID, &parentID, &threadID, &msg.Subject, &msg.Body, &createdAt)
	if err == sql.ErrNoRows {
		return nil, errors.New("message not found")
	}
	if err != nil {
		return nil, err
	}
	if parentID.Valid {
		msg.ParentID = parentID.Int64
	}
	if threadID.Valid {
		msg.ThreadID = threadID.Int64
	}
	msg.CreatedAt = parseSQLiteTime(createdAt)
	out := msg
	return &out, nil
}

func (r *SQLiteMessageRepository) ListByBoard(boardID int64) ([]domain.Message, error) {
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, board_id, author_id, parent_id, thread_id, subject, body, created_at
FROM messages
WHERE board_id = ?
ORDER BY thread_id ASC, created_at ASC, id ASC`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.Message, 0)
	for rows.Next() {
		var msg domain.Message
		var parentID sql.NullInt64
		var threadID sql.NullInt64
		var createdAt string
		if err := rows.Scan(&msg.ID, &msg.BoardID, &msg.AuthorID, &parentID, &threadID, &msg.Subject, &msg.Body, &createdAt); err != nil {
			return nil, err
		}
		if parentID.Valid {
			msg.ParentID = parentID.Int64
		}
		if threadID.Valid {
			msg.ThreadID = threadID.Int64
		}
		msg.CreatedAt = parseSQLiteTime(createdAt)
		out = append(out, msg)
	}
	return out, rows.Err()
}

func (r *SQLiteMessageRepository) GetPointer(userID, boardID int64) (*domain.MessagePointer, error) {
	if userID <= 0 || boardID <= 0 {
		return nil, ErrNotFound
	}
	var row domain.MessagePointer
	var lastReadAt string
	var updatedAt string
	err := r.db.QueryRowContext(context.Background(), `
SELECT user_id, board_id, last_read_id, last_read_at, updated_at
FROM message_pointers
WHERE user_id = ? AND board_id = ?
LIMIT 1`, userID, boardID).Scan(&row.UserID, &row.BoardID, &row.LastReadID, &lastReadAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	row.LastReadAt = parseSQLiteTime(lastReadAt)
	row.UpdatedAt = parseSQLiteTime(updatedAt)
	copy := row
	return &copy, nil
}

func (r *SQLiteMessageRepository) SetPointer(userID, boardID, lastReadID int64, lastReadAt time.Time) error {
	if userID <= 0 || boardID <= 0 || lastReadID <= 0 {
		return errors.New("user id, board id, and last read id are required")
	}
	now := time.Now().UTC()
	if lastReadAt.IsZero() {
		lastReadAt = now
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO message_pointers(user_id, board_id, last_read_id, last_read_at, updated_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(user_id, board_id) DO UPDATE SET
  last_read_id = CASE
    WHEN excluded.last_read_id > message_pointers.last_read_id THEN excluded.last_read_id
    ELSE message_pointers.last_read_id
  END,
  last_read_at = CASE
    WHEN excluded.last_read_id > message_pointers.last_read_id THEN excluded.last_read_at
    ELSE message_pointers.last_read_at
  END,
  updated_at = excluded.updated_at`,
		userID,
		boardID,
		lastReadID,
		formatSQLiteTime(lastReadAt),
		formatSQLiteTime(now),
	)
	return err
}

type SQLitePrivateMailRepository struct {
	db *sql.DB
}

func NewSQLitePrivateMailRepository(db *sql.DB) *SQLitePrivateMailRepository {
	return &SQLitePrivateMailRepository{db: db}
}

func (r *SQLitePrivateMailRepository) CreateMail(mail *domain.PrivateMail) error {
	if mail == nil {
		return errors.New("mail is required")
	}
	if strings.TrimSpace(mail.Subject) == "" {
		return errors.New("mail subject is required")
	}
	if strings.TrimSpace(mail.Body) == "" {
		return errors.New("mail body is required")
	}
	if mail.FromUserID <= 0 {
		return errors.New("from user id is required")
	}
	if mail.ToUserID <= 0 && mail.ExternalTo == nil {
		return errors.New("to user id or external recipient is required")
	}
	if mail.CreatedAt.IsZero() {
		mail.CreatedAt = time.Now().UTC()
	}
	var toUser interface{}
	if mail.ToUserID > 0 {
		toUser = mail.ToUserID
	}
	var external interface{}
	if mail.ExternalTo != nil {
		trimmed := strings.TrimSpace(*mail.ExternalTo)
		if trimmed != "" {
			external = trimmed
		}
	}
	res, err := r.db.ExecContext(context.Background(), `
INSERT INTO private_mail(from_user_id, to_user_id, external_to, subject, body, created_at, read_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		mail.FromUserID, toUser, external, strings.TrimSpace(mail.Subject), mail.Body, formatSQLiteTime(mail.CreatedAt), nullableSQLiteTime(mail.ReadAt))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	mail.ID = id
	return nil
}

func (r *SQLitePrivateMailRepository) GetMail(id int64) (*domain.PrivateMail, error) {
	var row domain.PrivateMail
	var toUser sql.NullInt64
	var external sql.NullString
	var createdAt string
	var readAt sql.NullString
	err := r.db.QueryRowContext(context.Background(), `
SELECT id, from_user_id, to_user_id, external_to, subject, body, created_at, read_at
FROM private_mail
WHERE id = ?`, id).Scan(&row.ID, &row.FromUserID, &toUser, &external, &row.Subject, &row.Body, &createdAt, &readAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if toUser.Valid {
		row.ToUserID = toUser.Int64
	}
	if external.Valid {
		value := external.String
		row.ExternalTo = &value
	}
	row.CreatedAt = parseSQLiteTime(createdAt)
	if readAt.Valid && strings.TrimSpace(readAt.String) != "" {
		ts := parseSQLiteTime(readAt.String)
		row.ReadAt = &ts
	}
	copy := row
	return &copy, nil
}

func (r *SQLitePrivateMailRepository) ListInbox(userID int64, limit int) ([]domain.PrivateMail, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, from_user_id, to_user_id, external_to, subject, body, created_at, read_at
FROM private_mail
WHERE to_user_id = ?
ORDER BY created_at DESC, id DESC
LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteMailRows(rows)
}

func (r *SQLitePrivateMailRepository) ListOutbox(userID int64, limit int) ([]domain.PrivateMail, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, from_user_id, to_user_id, external_to, subject, body, created_at, read_at
FROM private_mail
WHERE from_user_id = ?
ORDER BY created_at DESC, id DESC
LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSQLiteMailRows(rows)
}

func (r *SQLitePrivateMailRepository) MarkRead(id int64, readAt time.Time) error {
	if readAt.IsZero() {
		readAt = time.Now().UTC()
	}
	res, err := r.db.ExecContext(context.Background(), `UPDATE private_mail SET read_at = ? WHERE id = ?`, formatSQLiteTime(readAt), id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

type SQLiteAdminRepository struct {
	db *sql.DB
}

func NewSQLiteAdminRepository(db *sql.DB) *SQLiteAdminRepository {
	return &SQLiteAdminRepository{db: db}
}

func (r *SQLiteAdminRepository) AddAudit(entry *domain.AdminAudit) error {
	if entry == nil {
		return errors.New("audit entry is required")
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	res, err := r.db.ExecContext(context.Background(), `
INSERT INTO admin_audit_logs(actor, target, action, details, created_at)
VALUES (?, ?, ?, ?, ?)`,
		strings.TrimSpace(entry.Actor), strings.TrimSpace(entry.Target), strings.TrimSpace(entry.Action), strings.TrimSpace(entry.Details), formatSQLiteTime(entry.CreatedAt))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	entry.ID = id
	return nil
}

func (r *SQLiteAdminRepository) ListAudit(limit int) ([]domain.AdminAudit, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, actor, target, action, details, created_at
FROM admin_audit_logs
ORDER BY created_at DESC, id DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.AdminAudit, 0)
	for rows.Next() {
		var row domain.AdminAudit
		var createdAt string
		if err := rows.Scan(&row.ID, &row.Actor, &row.Target, &row.Action, &row.Details, &createdAt); err != nil {
			return nil, err
		}
		row.CreatedAt = parseSQLiteTime(createdAt)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *SQLiteAdminRepository) SetMailOutboundPolicy(handle string, disabled bool) error {
	handle = strings.ToLower(strings.TrimSpace(handle))
	if handle == "" {
		return errors.New("handle is required")
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO mail_outbound_policies(handle, outbound_disabled, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(handle) DO UPDATE SET outbound_disabled = excluded.outbound_disabled, updated_at = excluded.updated_at`,
		handle, boolToInt(disabled), formatSQLiteTime(time.Now().UTC()))
	return err
}

func (r *SQLiteAdminRepository) GetMailOutboundPolicy(handle string) (*domain.MailOutboundPolicy, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	var out domain.MailOutboundPolicy
	var disabled int
	var updatedAt string
	err := r.db.QueryRowContext(context.Background(), `
SELECT handle, outbound_disabled, updated_at
FROM mail_outbound_policies
WHERE handle = ?`, handle).Scan(&out.Handle, &disabled, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	out.OutboundDisabled = intToBool(disabled)
	out.UpdatedAt = parseSQLiteTime(updatedAt)
	return &out, nil
}

func (r *SQLiteAdminRepository) ListMailOutboundPolicies() ([]domain.MailOutboundPolicy, error) {
	rows, err := r.db.QueryContext(context.Background(), `SELECT handle, outbound_disabled, updated_at FROM mail_outbound_policies ORDER BY handle`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.MailOutboundPolicy, 0)
	for rows.Next() {
		var row domain.MailOutboundPolicy
		var disabled int
		var updatedAt string
		if err := rows.Scan(&row.Handle, &disabled, &updatedAt); err != nil {
			return nil, err
		}
		row.OutboundDisabled = intToBool(disabled)
		row.UpdatedAt = parseSQLiteTime(updatedAt)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *SQLiteAdminRepository) UpsertGatewaySettings(settings *domain.GatewaySettings) error {
	if settings == nil {
		return errors.New("settings are required")
	}
	if settings.MaxRecipients <= 0 {
		settings.MaxRecipients = 3
	}
	if settings.MaxMessageBytes <= 0 {
		settings.MaxMessageBytes = 65536
	}
	if settings.WebTimeoutSec <= 0 {
		settings.WebTimeoutSec = 10
	}
	if settings.WebMaxBytes <= 0 {
		settings.WebMaxBytes = 2 * 1024 * 1024
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO gateway_settings(id, smtp_host, smtp_port, smtp_user, smtp_pass, from_domain, max_recipients, max_message_bytes, web_timeout_sec, web_max_bytes, updated_at)
VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  smtp_host = excluded.smtp_host,
  smtp_port = excluded.smtp_port,
  smtp_user = excluded.smtp_user,
  smtp_pass = excluded.smtp_pass,
  from_domain = excluded.from_domain,
  max_recipients = excluded.max_recipients,
  max_message_bytes = excluded.max_message_bytes,
  web_timeout_sec = excluded.web_timeout_sec,
  web_max_bytes = excluded.web_max_bytes,
  updated_at = excluded.updated_at`,
		strings.TrimSpace(settings.SMTPHost), settings.SMTPPort, strings.TrimSpace(settings.SMTPUser), strings.TrimSpace(settings.SMTPPass),
		strings.TrimSpace(settings.FromDomain), settings.MaxRecipients, settings.MaxMessageBytes, settings.WebTimeoutSec, settings.WebMaxBytes, formatSQLiteTime(time.Now().UTC()))
	return err
}

func (r *SQLiteAdminRepository) GetGatewaySettings() (*domain.GatewaySettings, error) {
	var out domain.GatewaySettings
	var updatedAt string
	err := r.db.QueryRowContext(context.Background(), `
SELECT smtp_host, smtp_port, smtp_user, smtp_pass, from_domain, max_recipients, max_message_bytes, web_timeout_sec, web_max_bytes, updated_at
FROM gateway_settings WHERE id = 1`).
		Scan(&out.SMTPHost, &out.SMTPPort, &out.SMTPUser, &out.SMTPPass, &out.FromDomain, &out.MaxRecipients, &out.MaxMessageBytes, &out.WebTimeoutSec, &out.WebMaxBytes, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	out.UpdatedAt = parseSQLiteTime(updatedAt)
	return &out, nil
}

func (r *SQLiteAdminRepository) UpsertSystemSetting(key, value string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return errors.New("setting key is required")
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO system_settings(key, value, updated_at)
VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
  value = excluded.value,
  updated_at = excluded.updated_at`,
		key, value, formatSQLiteTime(time.Now().UTC()))
	return err
}

func (r *SQLiteAdminRepository) GetSystemSetting(key string) (string, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return "", ErrNotFound
	}
	var value string
	err := r.db.QueryRowContext(context.Background(), `
SELECT value
FROM system_settings
WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

func (r *SQLiteAdminRepository) ListSystemSettings() (map[string]string, error) {
	rows, err := r.db.QueryContext(context.Background(), `
SELECT key, value
FROM system_settings
ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key string
		var value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, rows.Err()
}

func (r *SQLiteAdminRepository) UpsertNodeSession(session *domain.NodeSession) error {
	if session == nil {
		return errors.New("node session is required")
	}
	sessionID := strings.TrimSpace(session.SessionID)
	if sessionID == "" {
		return errors.New("session id is required")
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO node_sessions(session_id, node_id, username, area, remote_addr, login_at, last_activity, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(session_id) DO UPDATE SET
  node_id = excluded.node_id,
  username = excluded.username,
  area = excluded.area,
  remote_addr = excluded.remote_addr,
  login_at = excluded.login_at,
  last_activity = excluded.last_activity,
  updated_at = excluded.updated_at`,
		sessionID,
		session.NodeID,
		strings.TrimSpace(session.Username),
		strings.TrimSpace(session.Area),
		strings.TrimSpace(session.RemoteAddr),
		formatSQLiteTime(session.LoginAt),
		formatSQLiteTime(session.LastActivity),
		formatSQLiteTime(session.UpdatedAt),
	)
	return err
}

func (r *SQLiteAdminRepository) DeleteNodeSession(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ErrNotFound
	}
	res, err := r.db.ExecContext(context.Background(), `DELETE FROM node_sessions WHERE session_id = ?`, sessionID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteAdminRepository) ListNodeSessions(limit int) ([]domain.NodeSession, error) {
	if limit <= 0 {
		limit = 256
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT session_id, node_id, username, area, remote_addr, login_at, last_activity, updated_at
FROM node_sessions
ORDER BY node_id, session_id
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.NodeSession, 0)
	for rows.Next() {
		var row domain.NodeSession
		var loginAt, lastActivity, updatedAt string
		if err := rows.Scan(&row.SessionID, &row.NodeID, &row.Username, &row.Area, &row.RemoteAddr, &loginAt, &lastActivity, &updatedAt); err != nil {
			return nil, err
		}
		row.LoginAt = parseSQLiteTime(loginAt)
		row.LastActivity = parseSQLiteTime(lastActivity)
		row.UpdatedAt = parseSQLiteTime(updatedAt)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *SQLiteAdminRepository) AddCallerHistory(entry *domain.CallerHistory) error {
	if entry == nil {
		return errors.New("caller history entry is required")
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	res, err := r.db.ExecContext(context.Background(), `
INSERT INTO caller_history(session_id, node_id, username, area, remote_addr, login_at, logout_at, duration_seconds, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(entry.SessionID),
		entry.NodeID,
		strings.TrimSpace(entry.Username),
		strings.TrimSpace(entry.Area),
		strings.TrimSpace(entry.RemoteAddr),
		formatSQLiteTime(entry.LoginAt),
		formatSQLiteTime(entry.LogoutAt),
		entry.DurationSeconds,
		formatSQLiteTime(entry.CreatedAt),
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	entry.ID = id
	return nil
}

func (r *SQLiteAdminRepository) ListCallerHistory(limit int) ([]domain.CallerHistory, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, session_id, node_id, username, area, remote_addr, login_at, logout_at, duration_seconds, created_at
FROM caller_history
ORDER BY created_at DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.CallerHistory, 0)
	for rows.Next() {
		var row domain.CallerHistory
		var loginAt, logoutAt, createdAt string
		if err := rows.Scan(&row.ID, &row.SessionID, &row.NodeID, &row.Username, &row.Area, &row.RemoteAddr, &loginAt, &logoutAt, &row.DurationSeconds, &createdAt); err != nil {
			return nil, err
		}
		row.LoginAt = parseSQLiteTime(loginAt)
		row.LogoutAt = parseSQLiteTime(logoutAt)
		row.CreatedAt = parseSQLiteTime(createdAt)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *SQLiteAdminRepository) ListFileAreas() ([]domain.FileArea, error) {
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, name, path, description, created_at
FROM file_areas
ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.FileArea, 0)
	for rows.Next() {
		var row domain.FileArea
		var createdAt string
		if err := rows.Scan(&row.ID, &row.Name, &row.Path, &row.Description, &createdAt); err != nil {
			return nil, err
		}
		row.CreatedAt = parseSQLiteTime(createdAt)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *SQLiteAdminRepository) CreateFileArea(area *domain.FileArea) error {
	if area == nil {
		return errors.New("file area is required")
	}
	area.Name = strings.TrimSpace(area.Name)
	if area.Name == "" {
		return errors.New("file area name is required")
	}
	area.Path = strings.TrimSpace(area.Path)
	if area.Path == "" {
		return errors.New("file area path is required")
	}
	if area.CreatedAt.IsZero() {
		area.CreatedAt = time.Now().UTC()
	}
	res, err := r.db.ExecContext(context.Background(), `
INSERT INTO file_areas(name, path, description, created_at)
VALUES (?, ?, ?, ?)`,
		area.Name, area.Path, strings.TrimSpace(area.Description), formatSQLiteTime(area.CreatedAt))
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	area.ID = id
	return nil
}

func (r *SQLiteAdminRepository) DeleteFileArea(id int64) error {
	res, err := r.db.ExecContext(context.Background(), `DELETE FROM file_areas WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteAdminRepository) GetFileEntry(id int64) (*domain.FileEntry, error) {
	var row domain.FileEntry
	var tagsJSON string
	var uploadedAt, createdAt, updatedAt string
	err := r.db.QueryRowContext(context.Background(), `
SELECT f.id, f.area_id, f.name, f.path, f.description, f.tags_json, f.sha256, f.size_bytes, f.uploader_id, f.uploaded_at, f.created_at, f.updated_at,
       COALESCE(rs.avg_rating, 0), COALESCE(rs.rating_count, 0)
FROM file_entries f
LEFT JOIN (
  SELECT file_id, AVG(rating) AS avg_rating, COUNT(*) AS rating_count
  FROM file_ratings
  GROUP BY file_id
) rs ON rs.file_id = f.id
WHERE f.id = ?`,
		id,
	).Scan(
		&row.ID, &row.AreaID, &row.Name, &row.Path, &row.Description, &tagsJSON, &row.SHA256, &row.SizeBytes, &row.UploaderID,
		&uploadedAt, &createdAt, &updatedAt, &row.RatingAvg, &row.RatingCount,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	row.Tags = decodeStringSlice(tagsJSON)
	row.UploadedAt = parseSQLiteTime(uploadedAt)
	row.CreatedAt = parseSQLiteTime(createdAt)
	row.UpdatedAt = parseSQLiteTime(updatedAt)
	copy := row
	return &copy, nil
}

func (r *SQLiteAdminRepository) UpsertFileEntry(entry *domain.FileEntry) error {
	if entry == nil {
		return errors.New("file entry is required")
	}
	entry.Name = strings.TrimSpace(entry.Name)
	entry.Path = strings.TrimSpace(entry.Path)
	entry.Description = strings.TrimSpace(entry.Description)
	entry.SHA256 = strings.ToLower(strings.TrimSpace(entry.SHA256))
	if entry.AreaID <= 0 || entry.Name == "" || entry.Path == "" {
		return errors.New("area id, file name, and path are required")
	}
	now := time.Now().UTC()
	if entry.UploadedAt.IsZero() {
		entry.UploadedAt = now
	}
	if entry.ID <= 0 && entry.SHA256 != "" {
		var existingID int64
		err := r.db.QueryRowContext(context.Background(), `SELECT id FROM file_entries WHERE sha256 = ? LIMIT 1`, entry.SHA256).Scan(&existingID)
		if err == nil && existingID > 0 {
			entry.ID = existingID
		}
	}
	if entry.ID > 0 {
		var createdAt string
		err := r.db.QueryRowContext(context.Background(), `SELECT created_at FROM file_entries WHERE id = ?`, entry.ID).Scan(&createdAt)
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		entry.CreatedAt = parseSQLiteTime(createdAt)
		entry.UpdatedAt = now
		_, err = r.db.ExecContext(context.Background(), `
UPDATE file_entries
SET area_id = ?, name = ?, path = ?, description = ?, tags_json = ?, sha256 = ?, size_bytes = ?, uploader_id = ?, uploaded_at = ?, updated_at = ?
WHERE id = ?`,
			entry.AreaID,
			entry.Name,
			entry.Path,
			entry.Description,
			encodeStringSlice(entry.Tags),
			entry.SHA256,
			entry.SizeBytes,
			entry.UploaderID,
			formatSQLiteTime(entry.UploadedAt),
			formatSQLiteTime(entry.UpdatedAt),
			entry.ID,
		)
		return err
	}
	entry.CreatedAt = now
	entry.UpdatedAt = now
	res, err := r.db.ExecContext(context.Background(), `
INSERT INTO file_entries(area_id, name, path, description, tags_json, sha256, size_bytes, uploader_id, uploaded_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.AreaID,
		entry.Name,
		entry.Path,
		entry.Description,
		encodeStringSlice(entry.Tags),
		entry.SHA256,
		entry.SizeBytes,
		entry.UploaderID,
		formatSQLiteTime(entry.UploadedAt),
		formatSQLiteTime(entry.CreatedAt),
		formatSQLiteTime(entry.UpdatedAt),
	)
	if err != nil {
		if sqliteConstraintErr(err) && entry.SHA256 != "" {
			var existingID int64
			if qErr := r.db.QueryRowContext(context.Background(), `SELECT id FROM file_entries WHERE sha256 = ? LIMIT 1`, entry.SHA256).Scan(&existingID); qErr == nil && existingID > 0 {
				entry.ID = existingID
				return r.UpsertFileEntry(entry)
			}
		}
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	entry.ID = id
	return nil
}

func (r *SQLiteAdminRepository) ListFileEntries(areaID int64, query string, tags []string, limit int) ([]domain.FileEntry, error) {
	if limit <= 0 {
		limit = 200
	}
	query = strings.ToLower(strings.TrimSpace(query))
	rows, err := r.db.QueryContext(context.Background(), `
SELECT f.id, f.area_id, f.name, f.path, f.description, f.tags_json, f.sha256, f.size_bytes, f.uploader_id, f.uploaded_at, f.created_at, f.updated_at,
       COALESCE(rs.avg_rating, 0), COALESCE(rs.rating_count, 0)
FROM file_entries f
LEFT JOIN (
  SELECT file_id, AVG(rating) AS avg_rating, COUNT(*) AS rating_count
  FROM file_ratings
  GROUP BY file_id
) rs ON rs.file_id = f.id
WHERE (? <= 0 OR f.area_id = ?)
  AND (? = '' OR lower(f.name || ' ' || f.description || ' ' || f.path) LIKE '%' || ? || '%')
ORDER BY f.uploaded_at DESC
LIMIT ?`, areaID, areaID, query, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tagSet := map[string]struct{}{}
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if tag != "" {
			tagSet[tag] = struct{}{}
		}
	}
	out := make([]domain.FileEntry, 0)
	for rows.Next() {
		var row domain.FileEntry
		var tagsJSON string
		var uploadedAt, createdAt, updatedAt string
		if err := rows.Scan(
			&row.ID, &row.AreaID, &row.Name, &row.Path, &row.Description, &tagsJSON, &row.SHA256, &row.SizeBytes, &row.UploaderID,
			&uploadedAt, &createdAt, &updatedAt, &row.RatingAvg, &row.RatingCount,
		); err != nil {
			return nil, err
		}
		row.Tags = decodeStringSlice(tagsJSON)
		row.UploadedAt = parseSQLiteTime(uploadedAt)
		row.CreatedAt = parseSQLiteTime(createdAt)
		row.UpdatedAt = parseSQLiteTime(updatedAt)
		if len(tagSet) > 0 {
			owned := map[string]struct{}{}
			for _, tag := range row.Tags {
				owned[strings.ToLower(strings.TrimSpace(tag))] = struct{}{}
			}
			miss := false
			for tag := range tagSet {
				if _, ok := owned[tag]; !ok {
					miss = true
					break
				}
			}
			if miss {
				continue
			}
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *SQLiteAdminRepository) SetFileRating(userID, fileID int64, rating int) error {
	if userID <= 0 || fileID <= 0 {
		return errors.New("user id and file id are required")
	}
	if rating < 1 || rating > 5 {
		return errors.New("rating must be between 1 and 5")
	}
	var exists int64
	err := r.db.QueryRowContext(context.Background(), `SELECT id FROM file_entries WHERE id = ?`, fileID).Scan(&exists)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(context.Background(), `
INSERT INTO file_ratings(user_id, file_id, rating, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(user_id, file_id) DO UPDATE SET
  rating = excluded.rating,
  updated_at = excluded.updated_at`,
		userID, fileID, rating, formatSQLiteTime(time.Now().UTC()))
	return err
}

func (r *SQLiteAdminRepository) SaveFileFilter(filter *domain.FileFilter) error {
	if filter == nil {
		return errors.New("file filter is required")
	}
	if filter.UserID <= 0 {
		return errors.New("user id is required")
	}
	filter.Name = strings.TrimSpace(filter.Name)
	if filter.Name == "" {
		return errors.New("filter name is required")
	}
	now := time.Now().UTC()
	if filter.ID > 0 {
		res, err := r.db.ExecContext(context.Background(), `
UPDATE file_filters
SET name = ?, query = ?, tags_json = ?, updated_at = ?
WHERE id = ? AND user_id = ?`,
			filter.Name,
			strings.TrimSpace(filter.Query),
			encodeStringSlice(filter.Tags),
			formatSQLiteTime(now),
			filter.ID,
			filter.UserID,
		)
		if err != nil {
			return err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return ErrNotFound
		}
		filter.UpdatedAt = now
		return nil
	}
	filter.CreatedAt = now
	filter.UpdatedAt = now
	res, err := r.db.ExecContext(context.Background(), `
INSERT INTO file_filters(user_id, name, query, tags_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)`,
		filter.UserID,
		filter.Name,
		strings.TrimSpace(filter.Query),
		encodeStringSlice(filter.Tags),
		formatSQLiteTime(filter.CreatedAt),
		formatSQLiteTime(filter.UpdatedAt),
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	filter.ID = id
	return nil
}

func (r *SQLiteAdminRepository) ListFileFilters(userID int64) ([]domain.FileFilter, error) {
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, user_id, name, query, tags_json, created_at, updated_at
FROM file_filters
WHERE (? <= 0 OR user_id = ?)
ORDER BY name ASC`, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.FileFilter, 0)
	for rows.Next() {
		var row domain.FileFilter
		var tagsJSON string
		var createdAt, updatedAt string
		if err := rows.Scan(&row.ID, &row.UserID, &row.Name, &row.Query, &tagsJSON, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		row.Tags = decodeStringSlice(tagsJSON)
		row.CreatedAt = parseSQLiteTime(createdAt)
		row.UpdatedAt = parseSQLiteTime(updatedAt)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *SQLiteAdminRepository) EnqueueDownload(userID, fileID int64) error {
	if userID <= 0 || fileID <= 0 {
		return errors.New("user id and file id are required")
	}
	var exists int64
	err := r.db.QueryRowContext(context.Background(), `SELECT id FROM file_entries WHERE id = ?`, fileID).Scan(&exists)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(context.Background(), `
INSERT INTO download_queue(user_id, file_id, created_at)
VALUES (?, ?, ?)
ON CONFLICT(user_id, file_id) DO NOTHING`,
		userID, fileID, formatSQLiteTime(time.Now().UTC()))
	return err
}

func (r *SQLiteAdminRepository) ListDownloadQueue(userID int64, limit int) ([]domain.DownloadQueueItem, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, user_id, file_id, created_at
FROM download_queue
WHERE (? <= 0 OR user_id = ?)
ORDER BY created_at ASC
LIMIT ?`, userID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.DownloadQueueItem, 0)
	for rows.Next() {
		var row domain.DownloadQueueItem
		var createdAt string
		if err := rows.Scan(&row.ID, &row.UserID, &row.FileID, &createdAt); err != nil {
			return nil, err
		}
		row.CreatedAt = parseSQLiteTime(createdAt)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *SQLiteAdminRepository) DequeueDownload(userID, fileID int64) error {
	res, err := r.db.ExecContext(context.Background(), `
DELETE FROM download_queue
WHERE user_id = ? AND file_id = ?`, userID, fileID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLiteAdminRepository) CreateDownloadTicket(ticket *domain.DownloadTicket) error {
	if ticket == nil {
		return errors.New("download ticket is required")
	}
	ticket.Token = strings.TrimSpace(ticket.Token)
	if ticket.Token == "" || ticket.UserID <= 0 || ticket.FileID <= 0 || ticket.ExpiresAt.IsZero() {
		return errors.New("token, user, file, and expiry are required")
	}
	if ticket.CreatedAt.IsZero() {
		ticket.CreatedAt = time.Now().UTC()
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO download_tickets(token, user_id, file_id, expires_at, created_at, used_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(token) DO UPDATE SET
  user_id = excluded.user_id,
  file_id = excluded.file_id,
  expires_at = excluded.expires_at,
  created_at = excluded.created_at,
  used_at = excluded.used_at`,
		ticket.Token,
		ticket.UserID,
		ticket.FileID,
		formatSQLiteTime(ticket.ExpiresAt),
		formatSQLiteTime(ticket.CreatedAt),
		nullableSQLiteTime(ticket.UsedAt),
	)
	return err
}

func (r *SQLiteAdminRepository) GetDownloadTicket(token string, now time.Time) (*domain.DownloadTicket, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrNotFound
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var row domain.DownloadTicket
	var expiresAt, createdAt string
	var usedAt sql.NullString
	err := r.db.QueryRowContext(context.Background(), `
SELECT token, user_id, file_id, expires_at, created_at, used_at
FROM download_tickets
WHERE token = ?`, token).Scan(&row.Token, &row.UserID, &row.FileID, &expiresAt, &createdAt, &usedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	row.ExpiresAt = parseSQLiteTime(expiresAt)
	row.CreatedAt = parseSQLiteTime(createdAt)
	if usedAt.Valid && strings.TrimSpace(usedAt.String) != "" {
		ts := parseSQLiteTime(usedAt.String)
		row.UsedAt = &ts
	}
	if row.ExpiresAt.Before(now.UTC()) || row.UsedAt != nil {
		return nil, ErrNotFound
	}
	copy := row
	return &copy, nil
}

func (r *SQLiteAdminRepository) MarkDownloadTicketUsed(token string, usedAt time.Time) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrNotFound
	}
	if usedAt.IsZero() {
		usedAt = time.Now().UTC()
	}
	res, err := r.db.ExecContext(context.Background(), `
UPDATE download_tickets
SET used_at = ?
WHERE token = ?`, formatSQLiteTime(usedAt), token)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

type SQLitePasswordResetRepository struct {
	db *sql.DB
}

func NewSQLitePasswordResetRepository(db *sql.DB) *SQLitePasswordResetRepository {
	return &SQLitePasswordResetRepository{db: db}
}

func (r *SQLitePasswordResetRepository) Create(token *domain.PasswordResetToken) error {
	if token == nil {
		return errors.New("token is required")
	}
	tokenHash := strings.TrimSpace(token.TokenHash)
	handle := strings.TrimSpace(token.Handle)
	if tokenHash == "" || handle == "" {
		return errors.New("token hash and handle are required")
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now().UTC()
	}
	if token.ExpiresAt.IsZero() {
		return errors.New("expiry is required")
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO password_reset_tokens(token_hash, handle, created_at, expires_at, consumed_at)
VALUES (?, ?, ?, ?, NULL)
ON CONFLICT(token_hash) DO UPDATE SET
  handle = excluded.handle,
  created_at = excluded.created_at,
  expires_at = excluded.expires_at,
  consumed_at = NULL`,
		tokenHash, handle, formatSQLiteTime(token.CreatedAt), formatSQLiteTime(token.ExpiresAt))
	return err
}

func (r *SQLitePasswordResetRepository) Get(tokenHash string) (*domain.PasswordResetToken, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return nil, ErrNotFound
	}
	var row domain.PasswordResetToken
	var createdAt, expiresAt string
	var consumed sql.NullString
	err := r.db.QueryRowContext(context.Background(), `
SELECT token_hash, handle, created_at, expires_at, consumed_at
FROM password_reset_tokens
WHERE token_hash = ?
LIMIT 1`, tokenHash).Scan(&row.TokenHash, &row.Handle, &createdAt, &expiresAt, &consumed)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	row.CreatedAt = parseSQLiteTime(createdAt)
	row.ExpiresAt = parseSQLiteTime(expiresAt)
	if consumed.Valid && strings.TrimSpace(consumed.String) != "" {
		ts := parseSQLiteTime(consumed.String)
		row.ConsumedAt = &ts
	}
	return &row, nil
}

func (r *SQLitePasswordResetRepository) MarkConsumed(tokenHash string, consumedAt time.Time) error {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return ErrNotFound
	}
	if consumedAt.IsZero() {
		consumedAt = time.Now().UTC()
	}
	res, err := r.db.ExecContext(context.Background(), `
UPDATE password_reset_tokens
SET consumed_at = ?
WHERE token_hash = ?`, formatSQLiteTime(consumedAt), tokenHash)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SQLitePasswordResetRepository) DeleteExpired(now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	_, err := r.db.ExecContext(context.Background(), `
DELETE FROM password_reset_tokens
WHERE expires_at < ? OR consumed_at IS NOT NULL`, formatSQLiteTime(now))
	return err
}

func scanSQLiteMailRows(rows *sql.Rows) ([]domain.PrivateMail, error) {
	out := make([]domain.PrivateMail, 0)
	for rows.Next() {
		var row domain.PrivateMail
		var toUser sql.NullInt64
		var external sql.NullString
		var createdAt string
		var readAt sql.NullString
		if err := rows.Scan(&row.ID, &row.FromUserID, &toUser, &external, &row.Subject, &row.Body, &createdAt, &readAt); err != nil {
			return nil, err
		}
		if toUser.Valid {
			row.ToUserID = toUser.Int64
		}
		if external.Valid && strings.TrimSpace(external.String) != "" {
			value := strings.TrimSpace(external.String)
			row.ExternalTo = &value
		}
		row.CreatedAt = parseSQLiteTime(createdAt)
		if readAt.Valid && strings.TrimSpace(readAt.String) != "" {
			ts := parseSQLiteTime(readAt.String)
			row.ReadAt = &ts
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func sqliteConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "constraint failed") || strings.Contains(lower, "unique")
}

func formatSQLiteTime(value time.Time) string {
	if value.IsZero() {
		value = time.Now().UTC()
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseSQLiteTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999999999-07:00",
	}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, raw); err == nil {
			return ts.UTC()
		}
	}
	return time.Time{}
}

func nullableSQLiteTime(value *time.Time) interface{} {
	if value == nil {
		return nil
	}
	return formatSQLiteTime(*value)
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func intToBool(v int) bool {
	return v != 0
}

func encodeStringSlice(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	clean := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		clean = append(clean, value)
	}
	body, err := json.Marshal(clean)
	if err != nil {
		return "[]"
	}
	return string(body)
}

func decodeStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	clean := make([]string, 0, len(out))
	for _, value := range out {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		clean = append(clean, value)
	}
	return clean
}

func defaultConference(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "General"
	}
	return value
}
