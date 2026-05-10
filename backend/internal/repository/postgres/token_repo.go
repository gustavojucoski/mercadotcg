package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavojucoski/mercadotcg/backend/internal/auth"
)

// TokenRepo gerencia email_verification_tokens, password_reset_tokens e refresh_tokens.
type TokenRepo struct {
	pool *pgxpool.Pool
}

func NewTokenRepo(pool *pgxpool.Pool) *TokenRepo {
	return &TokenRepo{pool: pool}
}

// ---- Email verification -----------------------------------------------------

func (r *TokenRepo) CreateVerificationToken(ctx context.Context, userID uuid.UUID, hash string, expiresAt time.Time) error {
	const q = `
		INSERT INTO email_verification_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`
	_, err := r.pool.Exec(ctx, q, userID, hash, expiresAt)
	if err != nil {
		return fmt.Errorf("create verification token: %w", err)
	}
	return nil
}

// UseVerificationToken marca o token como usado (atômico) e retorna o user_id.
// Retorna ErrNotFound se o token não existir, estiver expirado ou já usado.
func (r *TokenRepo) UseVerificationToken(ctx context.Context, hash string) (uuid.UUID, error) {
	const q = `
		UPDATE email_verification_tokens
		SET used_at = NOW()
		WHERE token_hash = $1
		  AND used_at IS NULL
		  AND expires_at > NOW()
		RETURNING user_id`
	var userID uuid.UUID
	err := r.pool.QueryRow(ctx, q, hash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("use verification token: %w", err)
	}
	return userID, nil
}

// ---- Password reset ---------------------------------------------------------

func (r *TokenRepo) CreatePasswordResetToken(ctx context.Context, userID uuid.UUID, hash string, expiresAt time.Time) error {
	const q = `
		INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`
	_, err := r.pool.Exec(ctx, q, userID, hash, expiresAt)
	if err != nil {
		return fmt.Errorf("create password reset token: %w", err)
	}
	return nil
}

// UsePasswordResetToken marca o token como usado e retorna o user_id.
func (r *TokenRepo) UsePasswordResetToken(ctx context.Context, hash string) (uuid.UUID, error) {
	const q = `
		UPDATE password_reset_tokens
		SET used_at = NOW()
		WHERE token_hash = $1
		  AND used_at IS NULL
		  AND expires_at > NOW()
		RETURNING user_id`
	var userID uuid.UUID
	err := r.pool.QueryRow(ctx, q, hash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("use password reset token: %w", err)
	}
	return userID, nil
}

// ---- Refresh tokens ---------------------------------------------------------

func (r *TokenRepo) CreateRefreshToken(ctx context.Context, userID uuid.UUID, hash string, expiresAt time.Time) error {
	const q = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`
	_, err := r.pool.Exec(ctx, q, userID, hash, expiresAt)
	if err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

func (r *TokenRepo) GetRefreshToken(ctx context.Context, hash string) (auth.RefreshTokenRecord, error) {
	const q = `SELECT user_id, expires_at, revoked_at FROM refresh_tokens WHERE token_hash = $1`
	var rec auth.RefreshTokenRecord
	err := r.pool.QueryRow(ctx, q, hash).Scan(&rec.UserID, &rec.ExpiresAt, &rec.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.RefreshTokenRecord{}, ErrNotFound
	}
	if err != nil {
		return auth.RefreshTokenRecord{}, fmt.Errorf("get refresh token: %w", err)
	}
	return rec, nil
}

func (r *TokenRepo) RevokeRefreshToken(ctx context.Context, hash string) error {
	const q = `UPDATE refresh_tokens SET revoked_at = NOW() WHERE token_hash = $1 AND revoked_at IS NULL`
	_, err := r.pool.Exec(ctx, q, hash)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}
