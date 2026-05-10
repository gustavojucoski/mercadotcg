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

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
)

// PriceHistoryRepo lida com a tabela particionada price_history.
//
// O caminho quente é Insert em lote: scrapers enfileiram observações e
// chamam InsertBatch com algumas centenas/milhares por vez via pgx.CopyFrom,
// que é ~10x mais rápido que INSERT um a um.
type PriceHistoryRepo struct {
	pool *pgxpool.Pool
}

// NewPriceHistoryRepo devolve um repositório pronto para uso.
func NewPriceHistoryRepo(pool *pgxpool.Pool) *PriceHistoryRepo {
	return &PriceHistoryRepo{pool: pool}
}

const insertPriceHistorySQL = `
INSERT INTO price_history (
    variant_id, condition, grade, source, kind,
    price_original, currency, price_brl, fx_rate_used,
    quantity, external_url, external_id, seller_country, observed_at
) VALUES (
    $1, $2, NULLIF($3, ''), $4, $5,
    $6, $7, $8, $9,
    $10, NULLIF($11, ''), NULLIF($12, ''), NULLIF($13, ''), $14
)
RETURNING id, ingested_at`

// Insert grava uma observação avulsa. Para volumes maiores, prefira InsertBatch.
// Colisão na UNIQUE (source, external_id, observed_at) vira ErrAlreadyExists.
func (r *PriceHistoryRepo) Insert(ctx context.Context, o *pricing.Observation) error {
	err := r.pool.QueryRow(ctx, insertPriceHistorySQL,
		o.VariantID, string(o.Condition), o.Grade, string(o.Source), string(o.Kind),
		o.PriceOriginal, string(o.Currency), o.PriceBRL, o.FxRateUsed,
		o.Quantity, o.ExternalURL, o.ExternalID, o.SellerCountry, o.ObservedAt,
	).Scan(&o.ID, &o.IngestedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == PgUniqueViolation {
			return ErrAlreadyExists
		}
		return fmt.Errorf("insert price_history: %w", err)
	}
	return nil
}

// priceHistoryColumns lista as colunas em ordem fixa para CopyFrom.
// Manter idêntica à ordem retornada por toCopyRow.
var priceHistoryColumns = []string{
	"variant_id", "condition", "grade", "source", "kind",
	"price_original", "currency", "price_brl", "fx_rate_used",
	"quantity", "external_url", "external_id", "seller_country", "observed_at",
}

// InsertBatch usa o protocolo COPY (pgx.CopyFrom) para inserir N observações
// em uma única ida ao servidor. Não respeita ON CONFLICT — duplicatas dentro
// do batch ou contra a UNIQUE existente farão a transação falhar.
//
// Devolve o número de linhas inseridas. Os campos ID e IngestedAt das structs
// NÃO são preenchidos por COPY; quem precisa do ID gerado deve usar Insert.
func (r *PriceHistoryRepo) InsertBatch(ctx context.Context, obs []pricing.Observation) (int64, error) {
	if len(obs) == 0 {
		return 0, nil
	}

	rows := make([][]any, len(obs))
	for i, o := range obs {
		rows[i] = toCopyRow(o)
	}

	n, err := r.pool.CopyFrom(
		ctx,
		pgx.Identifier{"price_history"},
		priceHistoryColumns,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return 0, fmt.Errorf("copy price_history: %w", err)
	}
	return n, nil
}

// toCopyRow serializa uma Observation no formato esperado por pgx.CopyFromRows.
// Strings vazias viram nil para honrar as colunas NULLABLE do schema.
func toCopyRow(o pricing.Observation) []any {
	var grade, externalURL, externalID, sellerCountry any
	if o.Grade != "" {
		grade = o.Grade
	}
	if o.ExternalURL != "" {
		externalURL = o.ExternalURL
	}
	if o.ExternalID != "" {
		externalID = o.ExternalID
	}
	if o.SellerCountry != "" {
		sellerCountry = o.SellerCountry
	}

	return []any{
		o.VariantID,
		string(o.Condition),
		grade,
		string(o.Source),
		string(o.Kind),
		o.PriceOriginal,
		string(o.Currency),
		o.PriceBRL,
		o.FxRateUsed,
		o.Quantity,
		externalURL,
		externalID,
		sellerCountry,
		o.ObservedAt,
	}
}

const latestObservationSQL = `
SELECT id, variant_id, condition, COALESCE(grade, ''), source, kind,
       price_original, currency, price_brl, fx_rate_used,
       quantity, COALESCE(external_url, ''), COALESCE(external_id, ''),
       COALESCE(seller_country, ''), observed_at, ingested_at
FROM price_history
WHERE variant_id = $1
ORDER BY observed_at DESC
LIMIT 1`

// LatestForVariant devolve a observação mais recente de uma variante.
// Usa o índice composto (variant_id, observed_at DESC).
func (r *PriceHistoryRepo) LatestForVariant(ctx context.Context, variantID uuid.UUID) (pricing.Observation, error) {
	var o pricing.Observation
	var cond, source, kind, currency string

	err := r.pool.QueryRow(ctx, latestObservationSQL, variantID).Scan(
		&o.ID, &o.VariantID, &cond, &o.Grade, &source, &kind,
		&o.PriceOriginal, &currency, &o.PriceBRL, &o.FxRateUsed,
		&o.Quantity, &o.ExternalURL, &o.ExternalID,
		&o.SellerCountry, &o.ObservedAt, &o.IngestedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return pricing.Observation{}, ErrNotFound
	}
	if err != nil {
		return pricing.Observation{}, fmt.Errorf("select latest observation: %w", err)
	}
	o.Condition = pricing.Condition(cond)
	o.Source = pricing.Source(source)
	o.Kind = pricing.Kind(kind)
	o.Currency = pricing.Currency(currency)
	return o, nil
}

const observationsInRangeSQL = `
SELECT id, variant_id, condition, COALESCE(grade, ''), source, kind,
       price_original, currency, price_brl, fx_rate_used,
       quantity, COALESCE(external_url, ''), COALESCE(external_id, ''),
       COALESCE(seller_country, ''), observed_at, ingested_at
FROM price_history
WHERE variant_id = $1
  AND observed_at >= $2
  AND observed_at <  $3
ORDER BY observed_at DESC`

// ObservationsInRange devolve todas as observações de uma variante numa janela.
// Casa com o índice (variant_id, observed_at DESC) + BRIN(observed_at).
func (r *PriceHistoryRepo) ObservationsInRange(
	ctx context.Context,
	variantID uuid.UUID,
	from, to time.Time,
) ([]pricing.Observation, error) {
	rows, err := r.pool.Query(ctx, observationsInRangeSQL, variantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query observations: %w", err)
	}
	defer rows.Close()

	var out []pricing.Observation
	for rows.Next() {
		var o pricing.Observation
		var cond, source, kind, currency string
		if err := rows.Scan(
			&o.ID, &o.VariantID, &cond, &o.Grade, &source, &kind,
			&o.PriceOriginal, &currency, &o.PriceBRL, &o.FxRateUsed,
			&o.Quantity, &o.ExternalURL, &o.ExternalID,
			&o.SellerCountry, &o.ObservedAt, &o.IngestedAt,
		); err != nil {
			return nil, fmt.Errorf("scan observation: %w", err)
		}
		o.Condition = pricing.Condition(cond)
		o.Source = pricing.Source(source)
		o.Kind = pricing.Kind(kind)
		o.Currency = pricing.Currency(currency)
		out = append(out, o)
	}
	return out, rows.Err()
}

