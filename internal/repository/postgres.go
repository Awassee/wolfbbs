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
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
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
		locked, err := r.IsThreadLocked(msg.ThreadID)
		if err != nil {
			return err
		}
		if locked {
			return errors.New("thread is locked")
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

func (r *PostgresMessageRepository) UpdateMessage(msg *domain.Message) error {
	if msg == nil || msg.ID <= 0 {
		return errors.New("message is required")
	}
	res, err := r.db.ExecContext(context.Background(), `
UPDATE messages
SET subject = $1, body = $2
WHERE id = $3`, strings.TrimSpace(msg.Subject), msg.Body, msg.ID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresMessageRepository) DeleteMessage(id int64) error {
	if id <= 0 {
		return errors.New("message id is required")
	}
	res, err := r.db.ExecContext(context.Background(), `DELETE FROM messages WHERE id = $1`, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresMessageRepository) MoveThread(threadID, toBoardID int64) error {
	if threadID <= 0 || toBoardID <= 0 {
		return errors.New("thread id and destination board id are required")
	}
	res, err := r.db.ExecContext(context.Background(), `UPDATE messages SET board_id = $1 WHERE thread_id = $2`, toBoardID, threadID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresMessageRepository) SetThreadLocked(threadID int64, locked bool) error {
	if threadID <= 0 {
		return errors.New("thread id is required")
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO message_thread_locks(thread_id, locked, updated_at)
VALUES ($1, $2, $3)
ON CONFLICT(thread_id) DO UPDATE SET
  locked = EXCLUDED.locked,
  updated_at = EXCLUDED.updated_at`,
		threadID,
		locked,
		time.Now().UTC(),
	)
	return err
}

func (r *PostgresMessageRepository) IsThreadLocked(threadID int64) (bool, error) {
	if threadID <= 0 {
		return false, errors.New("thread id is required")
	}
	var locked bool
	err := r.db.QueryRowContext(context.Background(), `
SELECT locked
FROM message_thread_locks
WHERE thread_id = $1
LIMIT 1`, threadID).Scan(&locked)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return locked, nil
}

func (r *PostgresMessageRepository) CreateReport(report *domain.MessageReport) error {
	if report == nil {
		return errors.New("report is required")
	}
	if report.MessageID <= 0 || report.ReporterID <= 0 {
		return errors.New("message id and reporter id are required")
	}
	report.Reason = strings.TrimSpace(report.Reason)
	if report.Reason == "" {
		return errors.New("report reason is required")
	}
	report.Status = strings.TrimSpace(report.Status)
	if report.Status == "" {
		report.Status = "open"
	}
	if report.CreatedAt.IsZero() {
		report.CreatedAt = time.Now().UTC()
	}
	return r.db.QueryRowContext(context.Background(), `
INSERT INTO message_reports(message_id, reporter_id, reason, status, created_at, resolved_at, resolved_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id`,
		report.MessageID,
		report.ReporterID,
		report.Reason,
		report.Status,
		report.CreatedAt,
		report.ResolvedAt,
		nullString(report.ResolvedBy),
	).Scan(&report.ID)
}

func (r *PostgresMessageRepository) ListReports(limit int, status string) ([]domain.MessageReport, error) {
	if limit <= 0 {
		limit = 100
	}
	status = strings.TrimSpace(status)
	baseQuery := `
SELECT id, message_id, reporter_id, reason, status, created_at, resolved_at, COALESCE(resolved_by, '')
FROM message_reports`
	var (
		rows *sql.Rows
		err  error
	)
	if status == "" {
		rows, err = r.db.QueryContext(context.Background(), baseQuery+`
ORDER BY created_at DESC, id DESC
LIMIT $1`, limit)
	} else {
		rows, err = r.db.QueryContext(context.Background(), baseQuery+`
WHERE status = $1
ORDER BY created_at DESC, id DESC
LIMIT $2`, status, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.MessageReport, 0, limit)
	for rows.Next() {
		var (
			row        domain.MessageReport
			resolvedAt sql.NullTime
		)
		if err := rows.Scan(&row.ID, &row.MessageID, &row.ReporterID, &row.Reason, &row.Status, &row.CreatedAt, &resolvedAt, &row.ResolvedBy); err != nil {
			return nil, err
		}
		if resolvedAt.Valid {
			ts := resolvedAt.Time
			row.ResolvedAt = &ts
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresMessageRepository) ResolveReport(id int64, resolvedBy string, resolvedAt time.Time) error {
	if id <= 0 {
		return errors.New("report id is required")
	}
	resolvedBy = strings.TrimSpace(resolvedBy)
	if resolvedBy == "" {
		return errors.New("resolved by is required")
	}
	if resolvedAt.IsZero() {
		resolvedAt = time.Now().UTC()
	}
	res, err := r.db.ExecContext(context.Background(), `
UPDATE message_reports
SET status = 'resolved', resolved_by = $1, resolved_at = $2
WHERE id = $3`, resolvedBy, resolvedAt.UTC(), id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresMessageRepository) GetPointer(userID, boardID int64) (*domain.MessagePointer, error) {
	if userID <= 0 || boardID <= 0 {
		return nil, ErrNotFound
	}
	var row domain.MessagePointer
	err := r.db.QueryRowContext(context.Background(), `
SELECT user_id, board_id, last_read_id, last_read_at, updated_at
FROM message_pointers
WHERE user_id = $1 AND board_id = $2
LIMIT 1`, userID, boardID).Scan(&row.UserID, &row.BoardID, &row.LastReadID, &row.LastReadAt, &row.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	copy := row
	return &copy, nil
}

func (r *PostgresMessageRepository) SetPointer(userID, boardID, lastReadID int64, lastReadAt time.Time) error {
	if userID <= 0 || boardID <= 0 || lastReadID <= 0 {
		return errors.New("user id, board id, and last read id are required")
	}
	now := time.Now().UTC()
	if lastReadAt.IsZero() {
		lastReadAt = now
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO message_pointers(user_id, board_id, last_read_id, last_read_at, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT(user_id, board_id) DO UPDATE SET
  last_read_id = GREATEST(message_pointers.last_read_id, EXCLUDED.last_read_id),
  last_read_at = CASE
    WHEN EXCLUDED.last_read_id > message_pointers.last_read_id THEN EXCLUDED.last_read_at
    ELSE message_pointers.last_read_at
  END,
  updated_at = EXCLUDED.updated_at`,
		userID, boardID, lastReadID, lastReadAt.UTC(), now)
	return err
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
	board.Conference = defaultConference(board.Conference)
	board.ReadACS = strings.TrimSpace(board.ReadACS)
	board.WriteACS = strings.TrimSpace(board.WriteACS)
	if board.CreatedAt.IsZero() {
		board.CreatedAt = time.Now().UTC()
	}
	return r.db.QueryRowContext(context.Background(), `
INSERT INTO boards(name, description, conference, read_acs, write_acs, created_by, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, created_at`,
		board.Name,
		board.Description,
		board.Conference,
		board.ReadACS,
		board.WriteACS,
		board.CreatedBy,
		board.CreatedAt).Scan(&board.ID, &board.CreatedAt)
}

func (r *PostgresBoardRepository) Get(id int64) (*domain.Board, error) {
	var board domain.Board
	err := r.db.QueryRowContext(context.Background(), `
SELECT id, name, COALESCE(description, ''), COALESCE(conference, 'General'), COALESCE(read_acs, ''), COALESCE(write_acs, ''), COALESCE(created_by, 0), created_at
FROM boards WHERE id = $1`, id).Scan(&board.ID, &board.Name, &board.Description, &board.Conference, &board.ReadACS, &board.WriteACS, &board.CreatedBy, &board.CreatedAt)
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
SELECT id, name, COALESCE(description, ''), COALESCE(conference, 'General'), COALESCE(read_acs, ''), COALESCE(write_acs, ''), COALESCE(created_by, 0), created_at
FROM boards ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]domain.Board, 0)
	for rows.Next() {
		var board domain.Board
		if err := rows.Scan(&board.ID, &board.Name, &board.Description, &board.Conference, &board.ReadACS, &board.WriteACS, &board.CreatedBy, &board.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, board)
	}
	return out, rows.Err()
}

func (r *PostgresBoardRepository) Update(board *domain.Board) error {
	if board == nil {
		return errors.New("board is required")
	}
	board.Name = strings.TrimSpace(board.Name)
	if board.Name == "" {
		return errors.New("board name is required")
	}
	board.Conference = defaultConference(board.Conference)
	board.ReadACS = strings.TrimSpace(board.ReadACS)
	board.WriteACS = strings.TrimSpace(board.WriteACS)
	res, err := r.db.ExecContext(context.Background(), `
UPDATE boards
SET name = $1, description = $2, conference = $3, read_acs = $4, write_acs = $5
WHERE id = $6`, board.Name, strings.TrimSpace(board.Description), board.Conference, board.ReadACS, board.WriteACS, board.ID)
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

func (r *PostgresPrivateMailRepository) DeleteMail(id int64) error {
	res, err := r.db.ExecContext(context.Background(), `DELETE FROM private_mail WHERE id = $1`, id)
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

func (r *PostgresAdminRepository) UpsertSystemSetting(key, value string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return errors.New("setting key is required")
	}
	_, err := r.db.ExecContext(context.Background(), `
INSERT INTO system_settings(key, value, updated_at)
VALUES ($1, $2, NOW())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = NOW()`, key, value)
	return err
}

func (r *PostgresAdminRepository) GetSystemSetting(key string) (string, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return "", ErrNotFound
	}
	var value string
	err := r.db.QueryRowContext(context.Background(), `
SELECT value
FROM system_settings
WHERE key = $1`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

func (r *PostgresAdminRepository) ListSystemSettings() (map[string]string, error) {
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

func (r *PostgresAdminRepository) UpsertNodeSession(session *domain.NodeSession) error {
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
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (session_id) DO UPDATE
SET node_id = EXCLUDED.node_id,
    username = EXCLUDED.username,
    area = EXCLUDED.area,
    remote_addr = EXCLUDED.remote_addr,
    login_at = EXCLUDED.login_at,
    last_activity = EXCLUDED.last_activity,
    updated_at = EXCLUDED.updated_at`,
		sessionID,
		session.NodeID,
		strings.TrimSpace(session.Username),
		strings.TrimSpace(session.Area),
		strings.TrimSpace(session.RemoteAddr),
		session.LoginAt,
		session.LastActivity,
		session.UpdatedAt,
	)
	return err
}

func (r *PostgresAdminRepository) DeleteNodeSession(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ErrNotFound
	}
	res, err := r.db.ExecContext(context.Background(), `DELETE FROM node_sessions WHERE session_id = $1`, sessionID)
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

func (r *PostgresAdminRepository) ListNodeSessions(limit int) ([]domain.NodeSession, error) {
	if limit <= 0 {
		limit = 256
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT session_id, node_id, username, area, remote_addr, login_at, last_activity, updated_at
FROM node_sessions
ORDER BY node_id, session_id
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.NodeSession, 0)
	for rows.Next() {
		var row domain.NodeSession
		if err := rows.Scan(&row.SessionID, &row.NodeID, &row.Username, &row.Area, &row.RemoteAddr, &row.LoginAt, &row.LastActivity, &row.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresAdminRepository) AddCallerHistory(entry *domain.CallerHistory) error {
	if entry == nil {
		return errors.New("caller history entry is required")
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	return r.db.QueryRowContext(context.Background(), `
INSERT INTO caller_history(session_id, node_id, username, area, remote_addr, login_at, logout_at, duration_seconds, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING id, created_at`,
		strings.TrimSpace(entry.SessionID),
		entry.NodeID,
		strings.TrimSpace(entry.Username),
		strings.TrimSpace(entry.Area),
		strings.TrimSpace(entry.RemoteAddr),
		entry.LoginAt,
		entry.LogoutAt,
		entry.DurationSeconds,
		entry.CreatedAt,
	).Scan(&entry.ID, &entry.CreatedAt)
}

func (r *PostgresAdminRepository) ListCallerHistory(limit int) ([]domain.CallerHistory, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, session_id, node_id, username, area, remote_addr, login_at, logout_at, duration_seconds, created_at
FROM caller_history
ORDER BY created_at DESC
LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.CallerHistory, 0)
	for rows.Next() {
		var row domain.CallerHistory
		if err := rows.Scan(&row.ID, &row.SessionID, &row.NodeID, &row.Username, &row.Area, &row.RemoteAddr, &row.LoginAt, &row.LogoutAt, &row.DurationSeconds, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
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

func (r *PostgresAdminRepository) GetFileEntry(id int64) (*domain.FileEntry, error) {
	var row domain.FileEntry
	var tagsJSON string
	var ratingCount int64
	err := r.db.QueryRowContext(context.Background(), `
SELECT f.id,
       f.area_id,
       f.name,
       f.path,
       f.description,
       f.tags_json::text,
       f.sha256,
       f.size_bytes,
       f.uploader_id,
       f.uploaded_at,
       f.created_at,
       f.updated_at,
       COALESCE(rs.avg_rating, 0),
       COALESCE(rs.rating_count, 0)
FROM file_entries f
LEFT JOIN (
  SELECT file_id, AVG(rating)::float8 AS avg_rating, COUNT(*)::int8 AS rating_count
  FROM file_ratings
  GROUP BY file_id
) rs ON rs.file_id = f.id
WHERE f.id = $1
LIMIT 1`, id).Scan(
		&row.ID,
		&row.AreaID,
		&row.Name,
		&row.Path,
		&row.Description,
		&tagsJSON,
		&row.SHA256,
		&row.SizeBytes,
		&row.UploaderID,
		&row.UploadedAt,
		&row.CreatedAt,
		&row.UpdatedAt,
		&row.RatingAvg,
		&ratingCount,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	row.RatingCount = int(ratingCount)
	row.Tags = decodeStringSlice(tagsJSON)
	out := row
	return &out, nil
}

func (r *PostgresAdminRepository) UpsertFileEntry(entry *domain.FileEntry) error {
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
		err := r.db.QueryRowContext(context.Background(), `
SELECT id
FROM file_entries
WHERE sha256 = $1
LIMIT 1`, entry.SHA256).Scan(&existingID)
		if err == nil && existingID > 0 {
			entry.ID = existingID
		}
		if err != nil && err != sql.ErrNoRows {
			return err
		}
	}
	if entry.ID > 0 {
		var createdAt time.Time
		err := r.db.QueryRowContext(context.Background(), `
SELECT created_at
FROM file_entries
WHERE id = $1`, entry.ID).Scan(&createdAt)
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		entry.CreatedAt = createdAt
		entry.UpdatedAt = now
		_, err = r.db.ExecContext(context.Background(), `
UPDATE file_entries
SET area_id = $1,
    name = $2,
    path = $3,
    description = $4,
    tags_json = $5::jsonb,
    sha256 = $6,
    size_bytes = $7,
    uploader_id = $8,
    uploaded_at = $9,
    updated_at = $10
WHERE id = $11`,
			entry.AreaID,
			entry.Name,
			entry.Path,
			entry.Description,
			encodeStringSlice(entry.Tags),
			entry.SHA256,
			entry.SizeBytes,
			entry.UploaderID,
			entry.UploadedAt.UTC(),
			entry.UpdatedAt.UTC(),
			entry.ID,
		)
		return err
	}

	entry.CreatedAt = now
	entry.UpdatedAt = now
	err := r.db.QueryRowContext(context.Background(), `
INSERT INTO file_entries(area_id, name, path, description, tags_json, sha256, size_bytes, uploader_id, uploaded_at, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9, $10, $11)
RETURNING id, created_at, updated_at`,
		entry.AreaID,
		entry.Name,
		entry.Path,
		entry.Description,
		encodeStringSlice(entry.Tags),
		entry.SHA256,
		entry.SizeBytes,
		entry.UploaderID,
		entry.UploadedAt.UTC(),
		entry.CreatedAt.UTC(),
		entry.UpdatedAt.UTC(),
	).Scan(&entry.ID, &entry.CreatedAt, &entry.UpdatedAt)
	if err != nil {
		if pgErr, ok := err.(*pq.Error); ok && pgErr.Code == "23505" && entry.SHA256 != "" {
			var existingID int64
			qErr := r.db.QueryRowContext(context.Background(), `
SELECT id
FROM file_entries
WHERE sha256 = $1
LIMIT 1`, entry.SHA256).Scan(&existingID)
			if qErr == nil && existingID > 0 {
				entry.ID = existingID
				return r.UpsertFileEntry(entry)
			}
		}
		return err
	}
	return nil
}

func (r *PostgresAdminRepository) DeleteFileEntry(id int64) error {
	tx, err := r.db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	res, err := tx.ExecContext(context.Background(), `
DELETE FROM file_entries
WHERE id = $1`, id)
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
	if _, err := tx.ExecContext(context.Background(), `DELETE FROM file_ratings WHERE file_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(context.Background(), `DELETE FROM download_queue WHERE file_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(context.Background(), `DELETE FROM download_tickets WHERE file_id = $1`, id); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func (r *PostgresAdminRepository) ListFileEntries(areaID int64, query string, tags []string, limit int) ([]domain.FileEntry, error) {
	if limit <= 0 {
		limit = 200
	}
	query = strings.ToLower(strings.TrimSpace(query))
	rows, err := r.db.QueryContext(context.Background(), `
SELECT f.id,
       f.area_id,
       f.name,
       f.path,
       f.description,
       f.tags_json::text,
       f.sha256,
       f.size_bytes,
       f.uploader_id,
       f.uploaded_at,
       f.created_at,
       f.updated_at,
       COALESCE(rs.avg_rating, 0),
       COALESCE(rs.rating_count, 0)
FROM file_entries f
LEFT JOIN (
  SELECT file_id, AVG(rating)::float8 AS avg_rating, COUNT(*)::int8 AS rating_count
  FROM file_ratings
  GROUP BY file_id
) rs ON rs.file_id = f.id
WHERE ($1 <= 0 OR f.area_id = $1)
  AND ($2 = '' OR lower(f.name || ' ' || f.description || ' ' || f.path) LIKE '%' || $2 || '%')
ORDER BY f.uploaded_at DESC, f.id DESC
LIMIT $3`, areaID, query, limit)
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
		var ratingCount int64
		if err := rows.Scan(
			&row.ID,
			&row.AreaID,
			&row.Name,
			&row.Path,
			&row.Description,
			&tagsJSON,
			&row.SHA256,
			&row.SizeBytes,
			&row.UploaderID,
			&row.UploadedAt,
			&row.CreatedAt,
			&row.UpdatedAt,
			&row.RatingAvg,
			&ratingCount,
		); err != nil {
			return nil, err
		}
		row.RatingCount = int(ratingCount)
		row.Tags = decodeStringSlice(tagsJSON)
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

func (r *PostgresAdminRepository) SetFileRating(userID, fileID int64, rating int) error {
	if userID <= 0 || fileID <= 0 {
		return errors.New("user id and file id are required")
	}
	if rating < 1 || rating > 5 {
		return errors.New("rating must be between 1 and 5")
	}
	var exists int64
	err := r.db.QueryRowContext(context.Background(), `
SELECT id
FROM file_entries
WHERE id = $1`, fileID).Scan(&exists)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(context.Background(), `
INSERT INTO file_ratings(user_id, file_id, rating, updated_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT(user_id, file_id) DO UPDATE SET
  rating = EXCLUDED.rating,
  updated_at = EXCLUDED.updated_at`,
		userID, fileID, rating)
	return err
}

func (r *PostgresAdminRepository) SaveFileFilter(filter *domain.FileFilter) error {
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
SET name = $1,
    query = $2,
    tags_json = $3::jsonb,
    updated_at = $4
WHERE id = $5 AND user_id = $6`,
			filter.Name,
			strings.TrimSpace(filter.Query),
			encodeStringSlice(filter.Tags),
			now,
			filter.ID,
			filter.UserID,
		)
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
		filter.UpdatedAt = now
		return nil
	}
	filter.CreatedAt = now
	filter.UpdatedAt = now
	return r.db.QueryRowContext(context.Background(), `
INSERT INTO file_filters(user_id, name, query, tags_json, created_at, updated_at)
VALUES ($1, $2, $3, $4::jsonb, $5, $6)
RETURNING id, created_at, updated_at`,
		filter.UserID,
		filter.Name,
		strings.TrimSpace(filter.Query),
		encodeStringSlice(filter.Tags),
		filter.CreatedAt,
		filter.UpdatedAt,
	).Scan(&filter.ID, &filter.CreatedAt, &filter.UpdatedAt)
}

func (r *PostgresAdminRepository) ListFileFilters(userID int64) ([]domain.FileFilter, error) {
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, user_id, name, query, tags_json::text, created_at, updated_at
FROM file_filters
WHERE ($1 <= 0 OR user_id = $1)
ORDER BY name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.FileFilter, 0)
	for rows.Next() {
		var row domain.FileFilter
		var tagsJSON string
		if err := rows.Scan(&row.ID, &row.UserID, &row.Name, &row.Query, &tagsJSON, &row.CreatedAt, &row.UpdatedAt); err != nil {
			return nil, err
		}
		row.Tags = decodeStringSlice(tagsJSON)
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresAdminRepository) EnqueueDownload(userID, fileID int64) error {
	if userID <= 0 || fileID <= 0 {
		return errors.New("user id and file id are required")
	}
	var exists int64
	err := r.db.QueryRowContext(context.Background(), `
SELECT id
FROM file_entries
WHERE id = $1`, fileID).Scan(&exists)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(context.Background(), `
INSERT INTO download_queue(user_id, file_id, created_at)
VALUES ($1, $2, NOW())
ON CONFLICT(user_id, file_id) DO NOTHING`, userID, fileID)
	return err
}

func (r *PostgresAdminRepository) ListDownloadQueue(userID int64, limit int) ([]domain.DownloadQueueItem, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, user_id, file_id, created_at
FROM download_queue
WHERE ($1 <= 0 OR user_id = $1)
ORDER BY created_at ASC, id ASC
LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.DownloadQueueItem, 0)
	for rows.Next() {
		var row domain.DownloadQueueItem
		if err := rows.Scan(&row.ID, &row.UserID, &row.FileID, &row.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (r *PostgresAdminRepository) DequeueDownload(userID, fileID int64) error {
	res, err := r.db.ExecContext(context.Background(), `
DELETE FROM download_queue
WHERE user_id = $1 AND file_id = $2`, userID, fileID)
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

func (r *PostgresAdminRepository) CreateDownloadTicket(ticket *domain.DownloadTicket) error {
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
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT(token) DO UPDATE SET
  user_id = EXCLUDED.user_id,
  file_id = EXCLUDED.file_id,
  expires_at = EXCLUDED.expires_at,
  created_at = EXCLUDED.created_at,
  used_at = EXCLUDED.used_at`,
		ticket.Token,
		ticket.UserID,
		ticket.FileID,
		ticket.ExpiresAt.UTC(),
		ticket.CreatedAt.UTC(),
		ticket.UsedAt,
	)
	return err
}

func (r *PostgresAdminRepository) GetDownloadTicket(token string, now time.Time) (*domain.DownloadTicket, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrNotFound
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var row domain.DownloadTicket
	var usedAt sql.NullTime
	err := r.db.QueryRowContext(context.Background(), `
SELECT token, user_id, file_id, expires_at, created_at, used_at
FROM download_tickets
WHERE token = $1`, token).Scan(&row.Token, &row.UserID, &row.FileID, &row.ExpiresAt, &row.CreatedAt, &usedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if usedAt.Valid {
		row.UsedAt = &usedAt.Time
	}
	if row.ExpiresAt.Before(now.UTC()) || row.UsedAt != nil {
		return nil, ErrNotFound
	}
	out := row
	return &out, nil
}

func (r *PostgresAdminRepository) MarkDownloadTicketUsed(token string, usedAt time.Time) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrNotFound
	}
	if usedAt.IsZero() {
		usedAt = time.Now().UTC()
	}
	res, err := r.db.ExecContext(context.Background(), `
UPDATE download_tickets
SET used_at = $1
WHERE token = $2`, usedAt.UTC(), token)
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
