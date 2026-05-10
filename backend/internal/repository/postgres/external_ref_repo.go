package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/matching"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
)

// ExternalRefRepo gerencia o mapeamento (variant_id) ↔ (source, external_id).
//
// Sem este mapeamento, observações vindas dos scrapers não conseguem ser
// vinculadas a uma variante e portanto não viram price_history.
type ExternalRefRepo struct {
	pool *pgxpool.Pool
}

// NewExternalRefRepo devolve um repositório pronto para uso.
func NewExternalRefRepo(pool *pgxpool.Pool) *ExternalRefRepo {
	return &ExternalRefRepo{pool: pool}
}

const insertExternalRefSQL = `
INSERT INTO external_card_refs (
    variant_id, source, external_id, external_url, language, confidence, raw_title
) VALUES (
    $1, $2, $3, NULLIF($4, ''), $5, $6, NULLIF($7, '')
)
RETURNING id, matched_at`

// Create insere um novo match. (source, external_id) é UNIQUE — colisão
// devolve ErrAlreadyExists, sinalizando que já existe um match para essa
// combinação (talvez para outra variante — vale revisar manualmente).
func (r *ExternalRefRepo) Create(ctx context.Context, ref *matching.ExternalCardRef) error {
	err := r.pool.QueryRow(ctx, insertExternalRefSQL,
		ref.VariantID, string(ref.Source), ref.ExternalID, ref.ExternalURL,
		ref.Language, ref.Confidence, ref.RawTitle,
	).Scan(&ref.ID, &ref.MatchedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			return ErrAlreadyExists
		}
		return fmt.Errorf("insert external_card_ref: %w", err)
	}
	return nil
}

const selectBySourceIDSQL = `
SELECT id, variant_id, source, external_id, COALESCE(external_url, ''), language,
       confidence, COALESCE(raw_title, ''), matched_at
FROM external_card_refs
WHERE source = $1 AND external_id = $2`

// GetBySourceID devolve o match para um (source, external_id) específico.
// Usado pelo pipeline de scraping para decidir se persiste a observação.
func (r *ExternalRefRepo) GetBySourceID(
	ctx context.Context,
	source pricing.Source,
	externalID string,
) (matching.ExternalCardRef, error) {
	var ref matching.ExternalCardRef
	var src string
	err := r.pool.QueryRow(ctx, selectBySourceIDSQL, string(source), externalID).Scan(
		&ref.ID, &ref.VariantID, &src, &ref.ExternalID, &ref.ExternalURL, &ref.Language,
		&ref.Confidence, &ref.RawTitle, &ref.MatchedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return matching.ExternalCardRef{}, ErrNotFound
	}
	if err != nil {
		return matching.ExternalCardRef{}, fmt.Errorf("select external_card_ref: %w", err)
	}
	ref.Source = pricing.Source(src)
	return ref, nil
}

const listByVariantSQL = `
SELECT id, variant_id, source, external_id, COALESCE(external_url, ''), language,
       confidence, COALESCE(raw_title, ''), matched_at
FROM external_card_refs
WHERE variant_id = $1
ORDER BY source, language`

// ListByVariant devolve todos os mapeamentos conhecidos para uma variante.
// Útil para enriquecer a view "carta X" com links nos sites externos.
func (r *ExternalRefRepo) ListByVariant(
	ctx context.Context,
	variantID uuid.UUID,
) ([]matching.ExternalCardRef, error) {
	rows, err := r.pool.Query(ctx, listByVariantSQL, variantID)
	if err != nil {
		return nil, fmt.Errorf("list external_card_refs: %w", err)
	}
	defer rows.Close()

	var out []matching.ExternalCardRef
	for rows.Next() {
		var ref matching.ExternalCardRef
		var src string
		if err := rows.Scan(
			&ref.ID, &ref.VariantID, &src, &ref.ExternalID, &ref.ExternalURL, &ref.Language,
			&ref.Confidence, &ref.RawTitle, &ref.MatchedAt,
		); err != nil {
			return nil, fmt.Errorf("scan external_card_ref: %w", err)
		}
		ref.Source = pricing.Source(src)
		out = append(out, ref)
	}
	return out, rows.Err()
}
