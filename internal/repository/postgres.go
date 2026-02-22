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
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	err := r.db.QueryRowContext(context.Background(), `
INSERT INTO messages (board_id, author_id, subject, body, created_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, created_at`, msg.BoardID, msg.AuthorID, msg.Subject, msg.Body, msg.CreatedAt).Scan(&msg.ID, &msg.CreatedAt)
	return err
}

func (r *PostgresMessageRepository) GetMessage(id int64) (*domain.Message, error) {
	var msg domain.Message
	err := r.db.QueryRowContext(context.Background(), `
SELECT id, board_id, author_id, subject, body, created_at
FROM messages
WHERE id = $1`, id).Scan(&msg.ID, &msg.BoardID, &msg.AuthorID, &msg.Subject, &msg.Body, &msg.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, errors.New("message not found")
	}
	if err != nil {
		return nil, err
	}
	out := msg
	return &out, nil
}

func (r *PostgresMessageRepository) ListByBoard(boardID int64) ([]domain.Message, error) {
	rows, err := r.db.QueryContext(context.Background(), `
SELECT id, board_id, author_id, subject, body, created_at
FROM messages
WHERE board_id = $1
ORDER BY created_at ASC`, boardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Message
	for rows.Next() {
		var msg domain.Message
		if err := rows.Scan(&msg.ID, &msg.BoardID, &msg.AuthorID, &msg.Subject, &msg.Body, &msg.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, msg)
	}
	return out, rows.Err()
}
