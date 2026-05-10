package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/store"
)

// StoreRepo persiste e consulta lojas (multi-tenant).
type StoreRepo struct {
	pool *pgxpool.Pool
}

// NewStoreRepo devolve um repositório pronto para uso.
func NewStoreRepo(pool *pgxpool.Pool) *StoreRepo {
	return &StoreRepo{pool: pool}
}

const insertStoreSQL = `
INSERT INTO stores (owner_id, name, slug, description, logo_url, is_active)
VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6)
RETURNING id, created_at, updated_at`

// Create insere uma loja e devolve o ID gerado.
// slug é único globalmente — colisão vira ErrAlreadyExists.
func (r *StoreRepo) Create(ctx context.Context, s *store.Store) error {
	err := r.pool.QueryRow(ctx, insertStoreSQL,
		s.OwnerID, s.Name, s.Slug, s.Description, s.LogoURL, s.IsActive,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			return ErrAlreadyExists
		}
		return fmt.Errorf("insert store: %w", err)
	}
	return nil
}

const selectStoreByIDSQL = `
SELECT id, owner_id, name, slug, COALESCE(description, ''), COALESCE(logo_url, ''),
       is_active, created_at, updated_at
FROM stores WHERE id = $1`

// GetByID busca uma loja pelo UUID.
func (r *StoreRepo) GetByID(ctx context.Context, id uuid.UUID) (store.Store, error) {
	return r.scanOne(ctx, selectStoreByIDSQL, id)
}

const selectStoreBySlugSQL = `
SELECT id, owner_id, name, slug, COALESCE(description, ''), COALESCE(logo_url, ''),
       is_active, created_at, updated_at
FROM stores WHERE slug = $1`

// GetBySlug busca uma loja pelo slug público.
func (r *StoreRepo) GetBySlug(ctx context.Context, slug string) (store.Store, error) {
	return r.scanOne(ctx, selectStoreBySlugSQL, slug)
}

func (r *StoreRepo) scanOne(ctx context.Context, sql string, arg any) (store.Store, error) {
	var s store.Store
	err := r.pool.QueryRow(ctx, sql, arg).Scan(
		&s.ID, &s.OwnerID, &s.Name, &s.Slug, &s.Description, &s.LogoURL,
		&s.IsActive, &s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Store{}, ErrNotFound
	}
	if err != nil {
		return store.Store{}, fmt.Errorf("select store: %w", err)
	}
	return s, nil
}

const listStoresByOwnerSQL = `
SELECT id, owner_id, name, slug, COALESCE(description, ''), COALESCE(logo_url, ''),
       is_active, created_at, updated_at
FROM stores
WHERE owner_id = $1
ORDER BY created_at ASC`

// ListByOwner devolve todas as lojas de um dono.
func (r *StoreRepo) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]store.Store, error) {
	rows, err := r.pool.Query(ctx, listStoresByOwnerSQL, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list stores: %w", err)
	}
	defer rows.Close()

	var out []store.Store
	for rows.Next() {
		var s store.Store
		if err := rows.Scan(
			&s.ID, &s.OwnerID, &s.Name, &s.Slug, &s.Description, &s.LogoURL,
			&s.IsActive, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan store: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
