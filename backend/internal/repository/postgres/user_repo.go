package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/user"
)

// UserRepo implementa acesso a dados para a tabela users e user_oauth_providers.
type UserRepo struct {
	pool *pgxpool.Pool
}

func NewUserRepo(pool *pgxpool.Pool) *UserRepo {
	return &UserRepo{pool: pool}
}

func (r *UserRepo) Create(ctx context.Context, u *user.User) error {
	const q = `
		INSERT INTO users (id, email, display_name, avatar_url, password_hash,
		                   platform_role, email_verified_at, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`
	err := r.pool.QueryRow(ctx, q,
		u.ID, u.Email, u.DisplayName, nullStr(u.AvatarURL),
		nullStr(u.PasswordHash), string(u.PlatformRole),
		u.EmailVerifiedAt, u.IsActive,
	).Scan(&u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			return ErrAlreadyExists
		}
		return fmt.Errorf("user create: %w", err)
	}
	return nil
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (user.User, error) {
	const q = `
		SELECT id, email, display_name, COALESCE(avatar_url,''), COALESCE(password_hash,''),
		       platform_role, email_verified_at, is_active, created_at, updated_at
		FROM users WHERE id = $1`
	return scanUser(r.pool.QueryRow(ctx, q, id))
}

func (r *UserRepo) GetByEmail(ctx context.Context, email string) (user.User, error) {
	const q = `
		SELECT id, email, display_name, COALESCE(avatar_url,''), COALESCE(password_hash,''),
		       platform_role, email_verified_at, is_active, created_at, updated_at
		FROM users WHERE email = $1`
	return scanUser(r.pool.QueryRow(ctx, q, email))
}

func (r *UserRepo) UpdatePasswordHash(ctx context.Context, id uuid.UUID, hash string) error {
	const q = `UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`
	tag, err := r.pool.Exec(ctx, q, hash, id)
	if err != nil {
		return fmt.Errorf("user update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *UserRepo) MarkEmailVerified(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE users SET email_verified_at = NOW(), updated_at = NOW() WHERE id = $1`
	tag, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("user mark verified: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *UserRepo) CompleteRegistration(ctx context.Context, id uuid.UUID, displayName, passwordHash string) error {
	const q = `UPDATE users SET password_hash = $1, display_name = $2, email_verified_at = NOW(), updated_at = NOW() WHERE id = $3`
	tag, err := r.pool.Exec(ctx, q, passwordHash, displayName, id)
	if err != nil {
		return fmt.Errorf("user complete registration: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *UserRepo) UpdatePlatformRole(ctx context.Context, id uuid.UUID, role user.PlatformRole) error {
	const q = `UPDATE users SET platform_role = $1, updated_at = NOW() WHERE id = $2`
	tag, err := r.pool.Exec(ctx, q, string(role), id)
	if err != nil {
		return fmt.Errorf("user update role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *UserRepo) SetActive(ctx context.Context, id uuid.UUID, active bool) error {
	const q = `UPDATE users SET is_active = $1, updated_at = NOW() WHERE id = $2`
	tag, err := r.pool.Exec(ctx, q, active, id)
	if err != nil {
		return fmt.Errorf("user set active: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *UserRepo) GetByOAuthProvider(ctx context.Context, provider, providerUID string) (user.User, error) {
	const q = `
		SELECT u.id, u.email, u.display_name, COALESCE(u.avatar_url,''),
		       COALESCE(u.password_hash,''), u.platform_role, u.email_verified_at,
		       u.is_active, u.created_at, u.updated_at
		FROM users u
		JOIN user_oauth_providers p ON p.user_id = u.id
		WHERE p.provider = $1 AND p.provider_uid = $2`
	return scanUser(r.pool.QueryRow(ctx, q, provider, providerUID))
}

func (r *UserRepo) LinkOAuthProvider(ctx context.Context, userID uuid.UUID, provider, providerUID string) error {
	const q = `
		INSERT INTO user_oauth_providers (user_id, provider, provider_uid)
		VALUES ($1, $2, $3)
		ON CONFLICT (provider, provider_uid) DO NOTHING`
	_, err := r.pool.Exec(ctx, q, userID, provider, providerUID)
	if err != nil {
		return fmt.Errorf("link oauth provider: %w", err)
	}
	return nil
}

func (r *UserRepo) List(ctx context.Context, limit, offset int) ([]user.User, error) {
	const q = `
		SELECT id, email, display_name, COALESCE(avatar_url,''), COALESCE(password_hash,''),
		       platform_role, email_verified_at, is_active, created_at, updated_at
		FROM users
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`
	rows, err := r.pool.Query(ctx, q, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("user list: %w", err)
	}
	defer rows.Close()
	var users []user.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *UserRepo) SearchByEmail(ctx context.Context, q string, limit int) ([]user.User, error) {
	const sql = `
		SELECT id, email, display_name, COALESCE(avatar_url,''), COALESCE(password_hash,''),
		       platform_role, email_verified_at, is_active, created_at, updated_at
		FROM users
		WHERE email ILIKE $1
		ORDER BY email
		LIMIT $2`
	rows, err := r.pool.Query(ctx, sql, "%"+q+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("user search: %w", err)
	}
	defer rows.Close()
	var users []user.User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// scanUser funciona tanto com pgx.Row quanto com pgx.Rows.
func scanUser(row pgx.Row) (user.User, error) {
	var u user.User
	var roleStr string
	err := row.Scan(
		&u.ID, &u.Email, &u.DisplayName, &u.AvatarURL, &u.PasswordHash,
		&roleStr, &u.EmailVerifiedAt, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return user.User{}, ErrNotFound
	}
	if err != nil {
		return user.User{}, fmt.Errorf("scan user: %w", err)
	}
	u.PlatformRole = user.PlatformRole(roleStr)
	return u, nil
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
