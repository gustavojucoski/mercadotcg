package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
    variant_id, source, external_id, external_url, language, confidence, needs_review, raw_title
) VALUES (
    $1, $2::price_source, $3, NULLIF($4, ''), $5, $6, $7, NULLIF($8, '')
)
ON CONFLICT (source, external_id) DO NOTHING
RETURNING id, matched_at`

// Create insere um novo match. (source, external_id) é UNIQUE — colisão
// devolve ErrAlreadyExists sem gerar erro no log do Postgres.
func (r *ExternalRefRepo) Create(ctx context.Context, ref *matching.ExternalCardRef) error {
	err := r.pool.QueryRow(ctx, insertExternalRefSQL,
		ref.VariantID, string(ref.Source), ref.ExternalID, ref.ExternalURL,
		ref.Language, ref.Confidence, ref.NeedsReview, ref.RawTitle,
	).Scan(&ref.ID, &ref.MatchedAt)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("insert external_card_ref: %w", err)
	}
	return nil
}

const selectBySourceIDSQL = `
SELECT id, variant_id, source, external_id, COALESCE(external_url, ''), language,
       confidence, needs_review, COALESCE(raw_title, ''), matched_at
FROM external_card_refs
WHERE source = $1::price_source AND external_id = $2`

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
		&ref.Confidence, &ref.NeedsReview, &ref.RawTitle, &ref.MatchedAt,
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
       confidence, needs_review, COALESCE(raw_title, ''), matched_at
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
			&ref.Confidence, &ref.NeedsReview, &ref.RawTitle, &ref.MatchedAt,
		); err != nil {
			return nil, fmt.Errorf("scan external_card_ref: %w", err)
		}
		ref.Source = pricing.Source(src)
		out = append(out, ref)
	}
	return out, rows.Err()
}

const upsertMatchCandidateSQL = `
INSERT INTO match_candidates (
    source, external_id, raw_title, raw_number, raw_set_code,
    best_candidate_variant_id, best_score
) VALUES (
    $1::price_source, $2, $3, NULLIF($4, ''), NULLIF($5, ''),
    $6, $7
)
ON CONFLICT (source, external_id) WHERE reviewed_at IS NULL
DO UPDATE SET
    best_score                = EXCLUDED.best_score,
    best_candidate_variant_id = EXCLUDED.best_candidate_variant_id
RETURNING id, created_at`

// UpsertMatchCandidate insere ou atualiza um candidato de matching em quarentena.
// A constraint parcial (WHERE reviewed_at IS NULL) garante que candidatos já
// revisados não sejam sobrescritos.
func (r *ExternalRefRepo) UpsertMatchCandidate(
	ctx context.Context,
	c *matching.MatchCandidate,
) error {
	var variantID *uuid.UUID
	if c.BestCandidateVariantID != nil && *c.BestCandidateVariantID != uuid.Nil {
		variantID = c.BestCandidateVariantID
	}

	err := r.pool.QueryRow(ctx, upsertMatchCandidateSQL,
		string(c.Source), c.ExternalID, c.RawTitle,
		c.RawNumber, c.RawSetCode,
		variantID, c.BestScore,
	).Scan(&c.ID, &c.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert match_candidate: %w", err)
	}
	return nil
}
