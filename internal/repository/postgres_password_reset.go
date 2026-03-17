package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"wolfbbs/internal/domain"
)

type PostgresPasswordResetRepository struct {
	db *sql.DB
}

func NewPostgresPasswordResetRepository(db *sql.DB) *PostgresPasswordResetRepository {
	return &PostgresPasswordResetRepository{db: db}
}

func (r *PostgresPasswordResetRepository) Create(token *domain.PasswordResetToken) error {
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
INSERT INTO password_reset_tokens(token_hash, handle, created_at, expires_at)
VALUES($1, $2, $3, $4)
ON CONFLICT (token_hash) DO UPDATE
SET handle = EXCLUDED.handle,
    created_at = EXCLUDED.created_at,
    expires_at = EXCLUDED.expires_at,
    consumed_at = NULL`, tokenHash, handle, token.CreatedAt, token.ExpiresAt)
	return err
}

func (r *PostgresPasswordResetRepository) Get(tokenHash string) (*domain.PasswordResetToken, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return nil, ErrNotFound
	}
	var row domain.PasswordResetToken
	var consumed sql.NullTime
	err := r.db.QueryRowContext(context.Background(), `
SELECT token_hash, handle, created_at, expires_at, consumed_at
FROM password_reset_tokens
WHERE token_hash = $1
LIMIT 1`, tokenHash).Scan(&row.TokenHash, &row.Handle, &row.CreatedAt, &row.ExpiresAt, &consumed)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if consumed.Valid {
		row.ConsumedAt = &consumed.Time
	}
	return &row, nil
}

func (r *PostgresPasswordResetRepository) MarkConsumed(tokenHash string, consumedAt time.Time) error {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return ErrNotFound
	}
	if consumedAt.IsZero() {
		consumedAt = time.Now().UTC()
	}
	res, err := r.db.ExecContext(context.Background(), `
UPDATE password_reset_tokens
SET consumed_at = $1
WHERE token_hash = $2`, consumedAt, tokenHash)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresPasswordResetRepository) DeleteExpired(now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	_, err := r.db.ExecContext(context.Background(), `
DELETE FROM password_reset_tokens
WHERE expires_at < $1 OR consumed_at IS NOT NULL`, now)
	return err
}
