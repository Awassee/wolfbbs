package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/lib/pq"
	"wolfbbs/internal/domain"
)

type PostgresUserRepository struct {
	db *sql.DB
}

func NewPostgresUserRepository(db *sql.DB) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

func (r *PostgresUserRepository) Create(user *domain.User) error {
	if user == nil {
		return errors.New("user is required")
	}
	key := strings.TrimSpace(user.Handle)
	if key == "" {
		return errors.New("handle is required")
	}
	user.Handle = key
	var existingID int64
	existsErr := r.db.QueryRowContext(context.Background(), `
SELECT id FROM users WHERE lower(handle) = lower($1) LIMIT 1`, user.Handle).Scan(&existingID)
	if existsErr == nil {
		return errors.New("handle already exists")
	}
	if existsErr != nil && existsErr != sql.ErrNoRows {
		return existsErr
	}
	now := time.Now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = now
	}
	query := `
INSERT INTO users (
  handle,
  password_hash,
  enabled,
  banned,
  force_reset,
  theme,
  time_format_24h,
  ansi_enabled,
  paging_enabled,
  role,
  totp_secret,
  recovery_codes,
  verified,
  last_login_at,
  created_at,
  updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
RETURNING id, created_at, updated_at`

	err := r.db.QueryRowContext(
		context.Background(),
		query,
		user.Handle,
		user.PasswordHash,
		user.Enabled,
		user.Banned,
		user.ForceReset,
		user.Theme,
		user.TimeFormat24h,
		user.ANSIEnabled,
		user.PagingEnabled,
		user.Role,
		nullString(user.TOTPSecret),
		pq.Array(user.RecoveryCodes),
		user.Verified,
		user.LastLoginAt,
		user.CreatedAt,
		user.UpdatedAt,
	).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if pgErr, ok := err.(*pq.Error); ok && pgErr.Code == "23505" {
			return errors.New("handle already exists")
		}
		return err
	}
	return nil
}

func (r *PostgresUserRepository) GetByHandle(handle string) (*domain.User, error) {
	var user domain.User
	var lastLogin sql.NullTime
	var recovery pq.StringArray
	var totpSecret sql.NullString
	err := r.db.QueryRowContext(context.Background(), `
SELECT
  id, handle, password_hash, enabled, banned, force_reset, theme, time_format_24h,
  ansi_enabled, paging_enabled, role, totp_secret, recovery_codes, verified, last_login_at,
  created_at, updated_at
FROM users
WHERE lower(handle) = lower($1)
LIMIT 1
`, strings.TrimSpace(handle)).Scan(
		&user.ID,
		&user.Handle,
		&user.PasswordHash,
		&user.Enabled,
		&user.Banned,
		&user.ForceReset,
		&user.Theme,
		&user.TimeFormat24h,
		&user.ANSIEnabled,
		&user.PagingEnabled,
		&user.Role,
		&totpSecret,
		&recovery,
		&user.Verified,
		&lastLogin,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if totpSecret.Valid {
		user.TOTPSecret = totpSecret.String
	}
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	if recovery != nil {
		user.RecoveryCodes = []string(recovery)
	}
	copy := user
	return &copy, nil
}

func (r *PostgresUserRepository) List() ([]domain.User, error) {
	rows, err := r.db.QueryContext(context.Background(), `
SELECT
  id, handle, password_hash, enabled, banned, force_reset, theme, time_format_24h,
  ansi_enabled, paging_enabled, role, totp_secret, recovery_codes, verified, last_login_at,
  created_at, updated_at
FROM users
ORDER BY handle`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.User, 0)
	for rows.Next() {
		var user domain.User
		var lastLogin sql.NullTime
		var recovery pq.StringArray
		var totpSecret sql.NullString
		if err := rows.Scan(
			&user.ID,
			&user.Handle,
			&user.PasswordHash,
			&user.Enabled,
			&user.Banned,
			&user.ForceReset,
			&user.Theme,
			&user.TimeFormat24h,
			&user.ANSIEnabled,
			&user.PagingEnabled,
			&user.Role,
			&totpSecret,
			&recovery,
			&user.Verified,
			&lastLogin,
			&user.CreatedAt,
			&user.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if totpSecret.Valid {
			user.TOTPSecret = totpSecret.String
		}
		if lastLogin.Valid {
			user.LastLoginAt = &lastLogin.Time
		}
		if recovery != nil {
			user.RecoveryCodes = []string(recovery)
		}
		out = append(out, user)
	}
	return out, rows.Err()
}

func (r *PostgresUserRepository) Update(user *domain.User) error {
	if user == nil {
		return errors.New("user is required")
	}
	user.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(context.Background(), `
UPDATE users
SET
  password_hash = $1,
  enabled = $2,
  banned = $3,
  force_reset = $4,
  theme = $5,
  time_format_24h = $6,
  ansi_enabled = $7,
  paging_enabled = $8,
  role = $9,
  totp_secret = $10,
  recovery_codes = $11,
  verified = $12,
  last_login_at = $13,
  updated_at = $14
WHERE id = $15`, user.PasswordHash, user.Enabled, user.Banned, user.ForceReset, user.Theme, user.TimeFormat24h, user.ANSIEnabled, user.PagingEnabled, user.Role, nullString(user.TOTPSecret), pq.Array(user.RecoveryCodes), user.Verified, user.LastLoginAt, user.UpdatedAt, user.ID)
	return err
}

func nullString(v string) interface{} {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return v
}

type PostgresMessageRepository struct {
	db *sql.DB
}

func NewPostgresMessageRepository(db *sql.DB) *PostgresMessageRepository {
	return &PostgresMessageRepository{db: db}
}

func (r *PostgresMessageRepository) CreateMessage(msg *domain.Message) error {
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
	parentID := sql.NullInt64{}
	if msg.ParentID > 0 {
		var parentBoardID int64
		var parentThreadID sql.NullInt64
		err := r.db.QueryRowContext(context.Background(), `
SELECT board_id, thread_id
FROM messages
WHERE id = $1`, msg.ParentID).Scan(&parentBoardID, &parentThreadID)
		if err == sql.ErrNoRows {
			return errors.New("parent message not found")
		}
		if err != nil {
			return err
		}
		if parentBoardID != msg.BoardID {
			return errors.New("parent message not in board")
		}
		parentID.Int64 = msg.ParentID
		parentID.Valid = true
		if parentThreadID.Valid && parentThreadID.Int64 > 0 {
			msg.ThreadID = parentThreadID.Int64
		} else {
			msg.ThreadID = msg.ParentID
		}
	}
	threadID := sql.NullInt64{}
	if msg.ThreadID > 0 {
		threadID.Int64 = msg.ThreadID
		threadID.Valid = true
	}
	err := r.db.QueryRowContext(context.Background(), `
INSERT INTO messages (board_id, author_id, parent_id, thread_id, subject, body, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, created_at`, msg.BoardID, msg.AuthorID, parentID, threadID, msg.Subject, msg.Body, msg.CreatedAt).Scan(&msg.ID, &msg.CreatedAt)
	if err != nil {
		return err
	}
	if msg.ThreadID <= 0 {
		msg.ThreadID = msg.ID
		_, err = r.db.ExecContext(context.Background(), `UPDATE messages SET thread_id = $1 WHERE id = $2`, msg.ThreadID, msg.ID)
		if err != nil {
			return err
		}
	}
	return err
}

func (r *PostgresMessageRepository) GetMessage(id int64) (*domain.Message, error) {
	var msg domain.Message
	var parentID sql.NullInt64
	var threadID sql.NullInt64
	err := r.db.QueryRowContext(context.Background(), `
SELECT id, board_id, author_id, parent_id, thread_id, subject, body, created_at
FROM messages
WHERE id = $1`, id).Scan(&msg.ID, &msg.BoardID, &msg.AuthorID, &parentID, &threadID, &msg.Subject, &msg.Body, &msg.CreatedAt)
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
	out := msg
	return &out, nil
}

func (r *PostgresMessageRepository) ListByBoard(boardID int64) ([]domain.Message, error) {
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, board_id, author_id, parent_id, thread_id, subject, body, created_at
FROM messages
WHERE board_id = $1
ORDER BY thread_id ASC, created_at ASC, id ASC`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Message
	for rows.Next() {
		var msg domain.Message
		var parentID sql.NullInt64
		var threadID sql.NullInt64
		if err := rows.Scan(&msg.ID, &msg.BoardID, &msg.AuthorID, &parentID, &threadID, &msg.Subject, &msg.Body, &msg.CreatedAt); err != nil {
			return nil, err
		}
		if parentID.Valid {
			msg.ParentID = parentID.Int64
		}
		if threadID.Valid {
			msg.ThreadID = threadID.Int64
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}

type PostgresBoardRepository struct {
	db *sql.DB
}

func NewPostgresBoardRepository(db *sql.DB) *PostgresBoardRepository {
	return &PostgresBoardRepository{db: db}
}

func (r *PostgresBoardRepository) Create(board *domain.Board) error {
	if board == nil {
		return errors.New("board is required")
	}
	board.Name = strings.TrimSpace(board.Name)
	if board.Name == "" {
		return errors.New("board name is required")
	}
	board.Description = strings.TrimSpace(board.Description)
	if board.CreatedAt.IsZero() {
		board.CreatedAt = time.Now().UTC()
	}
	return r.db.QueryRowContext(context.Background(), `
INSERT INTO boards(name, description, created_by, created_at)
VALUES ($1, $2, $3, $4)
RETURNING id, created_at`, board.Name, board.Description, board.CreatedBy, board.CreatedAt).Scan(&board.ID, &board.CreatedAt)
}

func (r *PostgresBoardRepository) Get(id int64) (*domain.Board, error) {
	var board domain.Board
	err := r.db.QueryRowContext(context.Background(), `
SELECT id, name, COALESCE(description, ''), COALESCE(created_by, 0), created_at
FROM boards WHERE id = $1`, id).Scan(&board.ID, &board.Name, &board.Description, &board.CreatedBy, &board.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	out := board
	return &out, nil
}

func (r *PostgresBoardRepository) List() ([]domain.Board, error) {
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, name, COALESCE(description, ''), COALESCE(created_by, 0), created_at
FROM boards ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Board, 0)
	for rows.Next() {
		var board domain.Board
		if err := rows.Scan(&board.ID, &board.Name, &board.Description, &board.CreatedBy, &board.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, board)
	}
	return out, rows.Err()
}

func (r *PostgresBoardRepository) Delete(id int64) error {
	res, err := r.db.ExecContext(context.Background(), `DELETE FROM boards WHERE id = $1`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

type PostgresPrivateMailRepository struct {
	db *sql.DB
}

func NewPostgresPrivateMailRepository(db *sql.DB) *PostgresPrivateMailRepository {
	return &PostgresPrivateMailRepository{db: db}
}

func (r *PostgresPrivateMailRepository) CreateMail(mail *domain.PrivateMail) error {
	if mail == nil {
		return errors.New("mail is required")
	}
	mail.Subject = strings.TrimSpace(mail.Subject)
	if mail.Subject == "" {
		return errors.New("mail subject is required")
	}
	mail.Body = strings.TrimSpace(mail.Body)
	if mail.Body == "" {
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
	var toUserID interface{}
	if mail.ToUserID > 0 {
		toUserID = mail.ToUserID
	}
	return r.db.QueryRowContext(context.Background(), `
INSERT INTO private_mail(from_user_id, to_user_id, external_to, subject, body, created_at, read_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, created_at`, mail.FromUserID, toUserID, nullString(derefString(mail.ExternalTo)), mail.Subject, mail.Body, mail.CreatedAt, mail.ReadAt).
		Scan(&mail.ID, &mail.CreatedAt)
}

func (r *PostgresPrivateMailRepository) GetMail(id int64) (*domain.PrivateMail, error) {
	var row domain.PrivateMail
	var toUserID sql.NullInt64
	var external sql.NullString
	err := r.db.QueryRowContext(context.Background(), `
SELECT id, from_user_id, to_user_id, external_to, subject, body, created_at, read_at
FROM private_mail WHERE id = $1`, id).
		Scan(&row.ID, &row.FromUserID, &toUserID, &external, &row.Subject, &row.Body, &row.CreatedAt, &row.ReadAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if toUserID.Valid {
		row.ToUserID = toUserID.Int64
	}
	if external.Valid {
		value := external.String
		row.ExternalTo = &value
	}
	out := row
	return &out, nil
}

func (r *PostgresPrivateMailRepository) ListInbox(userID int64, limit int) ([]domain.PrivateMail, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, from_user_id, to_user_id, external_to, subject, body, created_at, read_at
FROM private_mail
WHERE to_user_id = $1
ORDER BY created_at DESC
LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPrivateMailRows(rows)
}

func (r *PostgresPrivateMailRepository) ListOutbox(userID int64, limit int) ([]domain.PrivateMail, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, from_user_id, to_user_id, external_to, subject, body, created_at, read_at
FROM private_mail
WHERE from_user_id = $1
ORDER BY created_at DESC
LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPrivateMailRows(rows)
}

func scanPrivateMailRows(rows *sql.Rows) ([]domain.PrivateMail, error) {
	out := make([]domain.PrivateMail, 0)
	for rows.Next() {
		var row domain.PrivateMail
		var toUserID sql.NullInt64
		var external sql.NullString
		if err := rows.Scan(&row.ID, &row.FromUserID, &toUserID, &external, &row.Subject, &row.Body, &row.CreatedAt, &row.ReadAt); err != nil {
			return nil, err
		}
		if toUserID.Valid {
			row.ToUserID = toUserID.Int64
		}
		if external.Valid {
			value := external.String
			row.ExternalTo = &value
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresPrivateMailRepository) MarkRead(id int64, readAt time.Time) error {
	if readAt.IsZero() {
		readAt = time.Now().UTC()
	}
	res, err := r.db.ExecContext(context.Background(), `
UPDATE private_mail SET read_at = $1
WHERE id = $2`, readAt, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

type PostgresAdminRepository struct {
	db *sql.DB
}

func NewPostgresAdminRepository(db *sql.DB) *PostgresAdminRepository {
	return &PostgresAdminRepository{db: db}
}

func (r *PostgresAdminRepository) AddAudit(entry *domain.AdminAudit) error {
	if entry == nil {
		return errors.New("audit entry is required")
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	return r.db.QueryRowContext(context.Background(), `
INSERT INTO admin_audit_logs(actor, target, action, details, created_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, created_at`, strings.TrimSpace(entry.Actor), strings.TrimSpace(entry.Target), strings.TrimSpace(entry.Action), strings.TrimSpace(entry.Details), entry.CreatedAt).
		Scan(&entry.ID, &entry.CreatedAt)
}

func (r *PostgresAdminRepository) ListAudit(limit int) ([]domain.AdminAudit, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, actor, target, action, details, created_at
FROM admin_audit_logs
ORDER BY created_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.AdminAudit, 0)
	for rows.Next() {
		var row domain.AdminAudit
		if err := rows.Scan(&row.ID, &row.Actor, &row.Target, &row.Action, &row.Details, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresAdminRepository) SetMailOutboundPolicy(handle string, disabled bool) error {
	handle = strings.ToLower(strings.TrimSpace(handle))
	if handle == "" {
		return errors.New("handle is required")
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO mail_outbound_policies(handle, outbound_disabled, updated_at)
VALUES ($1, $2, NOW())
ON CONFLICT (handle) DO UPDATE
SET outbound_disabled = EXCLUDED.outbound_disabled,
    updated_at = NOW()`, handle, disabled)
	return err
}

func (r *PostgresAdminRepository) GetMailOutboundPolicy(handle string) (*domain.MailOutboundPolicy, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	var out domain.MailOutboundPolicy
	err := r.db.QueryRowContext(context.Background(), `
SELECT handle, outbound_disabled, updated_at
FROM mail_outbound_policies
WHERE handle = $1`, handle).Scan(&out.Handle, &out.OutboundDisabled, &out.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *PostgresAdminRepository) ListMailOutboundPolicies() ([]domain.MailOutboundPolicy, error) {
	rows, err := r.db.QueryContext(context.Background(), `
SELECT handle, outbound_disabled, updated_at
FROM mail_outbound_policies
ORDER BY handle`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.MailOutboundPolicy, 0)
	for rows.Next() {
		var row domain.MailOutboundPolicy
		if err := rows.Scan(&row.Handle, &row.OutboundDisabled, &row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresAdminRepository) UpsertGatewaySettings(settings *domain.GatewaySettings) error {
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
VALUES (1, $1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
ON CONFLICT (id) DO UPDATE
SET smtp_host = EXCLUDED.smtp_host,
    smtp_port = EXCLUDED.smtp_port,
    smtp_user = EXCLUDED.smtp_user,
    smtp_pass = EXCLUDED.smtp_pass,
    from_domain = EXCLUDED.from_domain,
    max_recipients = EXCLUDED.max_recipients,
    max_message_bytes = EXCLUDED.max_message_bytes,
    web_timeout_sec = EXCLUDED.web_timeout_sec,
    web_max_bytes = EXCLUDED.web_max_bytes,
    updated_at = NOW()`,
		strings.TrimSpace(settings.SMTPHost), settings.SMTPPort,
		strings.TrimSpace(settings.SMTPUser), strings.TrimSpace(settings.SMTPPass),
		strings.TrimSpace(settings.FromDomain), settings.MaxRecipients, settings.MaxMessageBytes,
		settings.WebTimeoutSec, settings.WebMaxBytes,
	)
	return err
}

func (r *PostgresAdminRepository) GetGatewaySettings() (*domain.GatewaySettings, error) {
	var out domain.GatewaySettings
	err := r.db.QueryRowContext(context.Background(), `
SELECT smtp_host, smtp_port, smtp_user, smtp_pass, from_domain, max_recipients, max_message_bytes, web_timeout_sec, web_max_bytes, updated_at
FROM gateway_settings
WHERE id = 1`).
		Scan(&out.SMTPHost, &out.SMTPPort, &out.SMTPUser, &out.SMTPPass, &out.FromDomain, &out.MaxRecipients, &out.MaxMessageBytes, &out.WebTimeoutSec, &out.WebMaxBytes, &out.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *PostgresAdminRepository) ListFileAreas() ([]domain.FileArea, error) {
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, name, path, COALESCE(description, ''), created_at
FROM file_areas
ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.FileArea, 0)
	for rows.Next() {
		var row domain.FileArea
		if err := rows.Scan(&row.ID, &row.Name, &row.Path, &row.Description, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresAdminRepository) CreateFileArea(area *domain.FileArea) error {
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
	return r.db.QueryRowContext(context.Background(), `
INSERT INTO file_areas(name, path, description, created_at)
VALUES ($1, $2, $3, $4)
RETURNING id, created_at`, area.Name, area.Path, strings.TrimSpace(area.Description), area.CreatedAt).
		Scan(&area.ID, &area.CreatedAt)
}

func (r *PostgresAdminRepository) DeleteFileArea(id int64) error {
	res, err := r.db.ExecContext(context.Background(), `DELETE FROM file_areas WHERE id = $1`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func derefString(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}
