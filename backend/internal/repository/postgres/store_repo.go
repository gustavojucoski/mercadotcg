package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

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
INSERT INTO stores (
    owner_id, name, slug, description, logo_url, is_active,
    document_type, document_number, document_status, legal_name,
    trade_name, phone,
    address_zip, address_street, address_number, address_complement,
    address_neighborhood, address_city, address_state, address_country
)
VALUES (
    $1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6,
    $7::document_type, $8, COALESCE($9::document_status, 'pending'::document_status), NULLIF($10, ''),
    NULLIF($11, ''), NULLIF($12, ''),
    NULLIF($13, ''), NULLIF($14, ''), NULLIF($15, ''), NULLIF($16, ''),
    NULLIF($17, ''), NULLIF($18, ''), NULLIF($19, ''), COALESCE(NULLIF($20, ''), 'BR')
)
RETURNING id, document_status, created_at, updated_at`

// Create insere uma loja e devolve o ID gerado.
// slug é único globalmente — colisão vira ErrAlreadyExists.
func (r *StoreRepo) Create(ctx context.Context, s *store.Store) error {
	var docStatus string
	var docStatusArg *string
	if s.DocumentStatus != "" {
		v := string(s.DocumentStatus)
		docStatusArg = &v
	}
	err := r.pool.QueryRow(ctx, insertStoreSQL,
		s.OwnerID, s.Name, s.Slug, s.Description, s.LogoURL, s.IsActive,
		(*string)(s.DocumentType), s.DocumentNumber, docStatusArg, s.LegalName,
		s.TradeName, s.Phone,
		s.AddressZip, s.AddressStreet, s.AddressNumber, s.AddressComplement,
		s.AddressNeighborhood, s.AddressCity, s.AddressState, s.AddressCountry,
	).Scan(&s.ID, &docStatus, &s.CreatedAt, &s.UpdatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			if pgErr.ConstraintName == "idx_stores_document" {
				return ErrDocumentAlreadyExists
			}
			return ErrAlreadyExists
		}
		return fmt.Errorf("insert store: %w", err)
	}
	s.DocumentStatus = store.DocumentStatus(docStatus)
	return nil
}

// selectStoreCols is the common column list for store queries.
const selectStoreCols = `
    id, owner_id, name, slug,
    COALESCE(description, ''), COALESCE(logo_url, ''), is_active,
    document_type, document_number, document_status, legal_name,
    document_verified_at, document_verified_by,
    COALESCE(trade_name, ''), COALESCE(phone, ''),
    COALESCE(address_zip, ''), COALESCE(address_street, ''),
    COALESCE(address_number, ''), COALESCE(address_complement, ''),
    COALESCE(address_neighborhood, ''), COALESCE(address_city, ''),
    COALESCE(address_state, ''), COALESCE(address_country, 'BR'),
    created_at, updated_at`

const selectStoreByIDSQL = `SELECT` + selectStoreCols + ` FROM stores WHERE id = $1`

// GetByID busca uma loja pelo UUID.
func (r *StoreRepo) GetByID(ctx context.Context, id uuid.UUID) (store.Store, error) {
	return r.scanOne(ctx, selectStoreByIDSQL, id)
}

const selectStoreBySlugSQL = `SELECT` + selectStoreCols + ` FROM stores WHERE slug = $1`

// GetBySlug busca uma loja pelo slug público.
func (r *StoreRepo) GetBySlug(ctx context.Context, slug string) (store.Store, error) {
	return r.scanOne(ctx, selectStoreBySlugSQL, slug)
}

const listAllStoresSQL = `SELECT` + selectStoreCols + `
FROM stores ORDER BY created_at DESC LIMIT $1 OFFSET $2`

// List devolve todas as lojas paginadas (para o admin).
func (r *StoreRepo) List(ctx context.Context, limit, offset int) ([]store.Store, error) {
	rows, err := r.pool.Query(ctx, listAllStoresSQL, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list stores: %w", err)
	}
	defer rows.Close()

	var out []store.Store
	for rows.Next() {
		s, err := scanStoreRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

const updateStoreSQL = `
UPDATE stores SET
    name = $2,
    slug = $3,
    description = NULLIF($4, ''),
    logo_url = NULLIF($5, ''),
    is_active = $6,
    legal_name = NULLIF($7, ''),
    document_type = $8::document_type,
    document_number = $9,
    document_status = $10::document_status,
    trade_name = NULLIF($11, ''),
    phone = NULLIF($12, ''),
    address_zip = NULLIF($13, ''),
    address_street = NULLIF($14, ''),
    address_number = NULLIF($15, ''),
    address_complement = NULLIF($16, ''),
    address_neighborhood = NULLIF($17, ''),
    address_city = NULLIF($18, ''),
    address_state = NULLIF($19, ''),
    address_country = COALESCE(NULLIF($20, ''), 'BR'),
    updated_at = NOW()
WHERE id = $1
RETURNING updated_at`

// Update persiste alterações básicas de uma loja (sem tocar em verified_at/by).
func (r *StoreRepo) Update(ctx context.Context, s *store.Store) error {
	var legalName *string
	if s.LegalName != nil {
		legalName = s.LegalName
	}
	return r.pool.QueryRow(ctx, updateStoreSQL,
		s.ID, s.Name, s.Slug, s.Description, s.LogoURL, s.IsActive,
		legalName, (*string)(s.DocumentType), s.DocumentNumber,
		string(s.DocumentStatus),
		s.TradeName, s.Phone,
		s.AddressZip, s.AddressStreet, s.AddressNumber, s.AddressComplement,
		s.AddressNeighborhood, s.AddressCity, s.AddressState, s.AddressCountry,
	).Scan(&s.UpdatedAt)
}

const setDocumentVerifiedSQL = `
UPDATE stores SET
    document_status = $2::document_status,
    document_verified_at = NOW(),
    document_verified_by = $3,
    updated_at = NOW()
WHERE id = $1`

// SetDocumentVerified marks a store's document as verified.
func (r *StoreRepo) SetDocumentVerified(ctx context.Context, storeID, verifiedByID uuid.UUID, status store.DocumentStatus) error {
	_, err := r.pool.Exec(ctx, setDocumentVerifiedSQL, storeID, string(status), verifiedByID)
	if err != nil {
		return fmt.Errorf("set document verified: %w", err)
	}
	return nil
}

func (r *StoreRepo) scanOne(ctx context.Context, sql string, arg any) (store.Store, error) {
	row := r.pool.QueryRow(ctx, sql, arg)
	s, err := scanStoreRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Store{}, ErrNotFound
	}
	if err != nil {
		return store.Store{}, fmt.Errorf("select store: %w", err)
	}
	return s, nil
}

// scanStoreRow scans a row into a Store. Works for both QueryRow and Rows.
func scanStoreRow(row interface {
	Scan(...any) error
}) (store.Store, error) {
	var s store.Store
	var docType *string
	var docStatus string
	var docVerifiedAt *time.Time
	var docVerifiedBy *uuid.UUID

	err := row.Scan(
		&s.ID, &s.OwnerID, &s.Name, &s.Slug,
		&s.Description, &s.LogoURL, &s.IsActive,
		&docType, &s.DocumentNumber, &docStatus, &s.LegalName,
		&docVerifiedAt, &docVerifiedBy,
		&s.TradeName, &s.Phone,
		&s.AddressZip, &s.AddressStreet,
		&s.AddressNumber, &s.AddressComplement,
		&s.AddressNeighborhood, &s.AddressCity,
		&s.AddressState, &s.AddressCountry,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return store.Store{}, err
	}
	if docType != nil {
		dt := store.DocumentType(*docType)
		s.DocumentType = &dt
	}
	s.DocumentStatus = store.DocumentStatus(docStatus)
	s.DocumentVerifiedAt = docVerifiedAt
	s.DocumentVerifiedBy = docVerifiedBy
	return s, nil
}

const listStoresByOwnerSQL = `SELECT` + selectStoreCols + `
FROM stores WHERE owner_id = $1 ORDER BY created_at ASC`

// ListByOwner devolve todas as lojas de um dono.
func (r *StoreRepo) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]store.Store, error) {
	rows, err := r.pool.Query(ctx, listStoresByOwnerSQL, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list stores: %w", err)
	}
	defer rows.Close()

	var out []store.Store
	for rows.Next() {
		s, err := scanStoreRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan store: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// listStoresByMemberSQL returns stores where the user is either the owner
// or has an explicit store_members row. UNION deduplicates when both apply.
const listStoresByMemberSQL = `SELECT` + selectStoreCols + `
FROM stores
WHERE owner_id = $1
   OR id IN (SELECT store_id FROM store_members WHERE user_id = $1)
ORDER BY created_at ASC`

// ListByMember returns all stores the user owns or is a member of.
func (r *StoreRepo) ListByMember(ctx context.Context, userID uuid.UUID) ([]store.Store, error) {
	rows, err := r.pool.Query(ctx, listStoresByMemberSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("list stores by member: %w", err)
	}
	defer rows.Close()

	var out []store.Store
	for rows.Next() {
		s, err := scanStoreRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan store: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// statusPtr converts a DocumentStatus to a *string for nullable DB writes.
func statusPtr(s store.DocumentStatus) *string {
	if s == "" {
		return nil
	}
	v := string(s)
	return &v
}
