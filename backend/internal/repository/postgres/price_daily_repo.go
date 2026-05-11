package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
)

// PriceSummary é o resumo de preço mais recente de uma variante numa condição.
type PriceSummary struct {
	MinBRL      decimal.Decimal `json:"min_brl"`
	AvgBRL      decimal.Decimal `json:"avg_brl"`
	MaxBRL      decimal.Decimal `json:"max_brl"`
	LastUpdated time.Time       `json:"last_updated"`
}

// PriceDailyRepo é a tabela quente que alimenta gráficos de tendência.
// Atualizada por job de agregação que faz UPSERT a partir de price_history.
type PriceDailyRepo struct {
	pool *pgxpool.Pool
}

// NewPriceDailyRepo devolve um repositório pronto para uso.
func NewPriceDailyRepo(pool *pgxpool.Pool) *PriceDailyRepo {
	return &PriceDailyRepo{pool: pool}
}

const upsertPriceDailySQL = `
INSERT INTO price_daily (
    variant_id, condition, source, day,
    sales_count, listings_count,
    sale_min, sale_max, sale_avg, sale_median, sale_p25, sale_p75,
    listing_min, listing_avg,
    last_updated
) VALUES (
    $1, $2, $3, $4,
    $5, $6,
    $7, $8, $9, $10, $11, $12,
    $13, $14,
    NOW()
)
ON CONFLICT (variant_id, condition, source, day) DO UPDATE SET
    sales_count    = EXCLUDED.sales_count,
    listings_count = EXCLUDED.listings_count,
    sale_min       = EXCLUDED.sale_min,
    sale_max       = EXCLUDED.sale_max,
    sale_avg       = EXCLUDED.sale_avg,
    sale_median    = EXCLUDED.sale_median,
    sale_p25       = EXCLUDED.sale_p25,
    sale_p75       = EXCLUDED.sale_p75,
    listing_min    = EXCLUDED.listing_min,
    listing_avg    = EXCLUDED.listing_avg,
    last_updated   = NOW()`

// Upsert grava ou atualiza o ponto agregado do dia. Idempotente — pode ser
// rodado várias vezes para o mesmo (variant, condition, source, day).
func (r *PriceDailyRepo) Upsert(ctx context.Context, p pricing.DailyPoint) error {
	_, err := r.pool.Exec(ctx, upsertPriceDailySQL,
		p.VariantID, string(p.Condition), string(p.Source), p.Day,
		p.SalesCount, p.ListingsCount,
		p.SaleMin, p.SaleMax, p.SaleAvg, p.SaleMedian, p.SaleP25, p.SaleP75,
		p.ListingMin, p.ListingAvg,
	)
	if err != nil {
		return fmt.Errorf("upsert price_daily: %w", err)
	}
	return nil
}

const seriesByVariantSQL = `
SELECT variant_id, condition, source, day,
       sales_count, listings_count,
       sale_min, sale_max, sale_avg, sale_median, sale_p25, sale_p75,
       listing_min, listing_avg, last_updated
FROM price_daily
WHERE variant_id = $1
  AND day >= $2
  AND day <  $3
ORDER BY day ASC`

// SeriesByVariant devolve a série temporal completa (todas as condições e fontes)
// de uma variante numa janela [from, to). Usa o índice (variant_id, day DESC).
//
// Cabe ao caller filtrar/agrupar por condition+source no front, ou chamar a
// versão filtrada SeriesByVariantFiltered.
func (r *PriceDailyRepo) SeriesByVariant(
	ctx context.Context,
	variantID uuid.UUID,
	from, to time.Time,
) ([]pricing.DailyPoint, error) {
	rows, err := r.pool.Query(ctx, seriesByVariantSQL, variantID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query price_daily: %w", err)
	}
	defer rows.Close()

	var out []pricing.DailyPoint
	for rows.Next() {
		var p pricing.DailyPoint
		var cond, source string
		if err := rows.Scan(
			&p.VariantID, &cond, &source, &p.Day,
			&p.SalesCount, &p.ListingsCount,
			&p.SaleMin, &p.SaleMax, &p.SaleAvg, &p.SaleMedian, &p.SaleP25, &p.SaleP75,
			&p.ListingMin, &p.ListingAvg, &p.LastUpdated,
		); err != nil {
			return nil, fmt.Errorf("scan price_daily: %w", err)
		}
		p.Condition = pricing.Condition(cond)
		p.Source = pricing.Source(source)
		out = append(out, p)
	}
	return out, rows.Err()
}

// RebuildDay recalcula price_daily para um dia inteiro a partir de price_history,
// para todas as combinações (variant_id, condition, source) que tiveram
// observação naquele dia. Idempotente.
//
// Implementação: agregação direta no Postgres via SQL — evita trazer milhões
// de linhas para o cliente Go.
const rebuildDaySQL = `
INSERT INTO price_daily (
    variant_id, condition, source, day,
    sales_count, listings_count,
    sale_min, sale_max, sale_avg, sale_median, sale_p25, sale_p75,
    listing_min, listing_avg,
    last_updated
)
SELECT
    variant_id,
    condition,
    source,
    $1::date AS day,
    COUNT(*) FILTER (WHERE kind = 'sale')                                    AS sales_count,
    COUNT(*) FILTER (WHERE kind = 'listing')                                 AS listings_count,
    MIN(price_brl)        FILTER (WHERE kind = 'sale')                       AS sale_min,
    MAX(price_brl)        FILTER (WHERE kind = 'sale')                       AS sale_max,
    AVG(price_brl)        FILTER (WHERE kind = 'sale')                       AS sale_avg,
    PERCENTILE_CONT(0.5)  WITHIN GROUP (ORDER BY price_brl)
        FILTER (WHERE kind = 'sale')                                         AS sale_median,
    PERCENTILE_CONT(0.25) WITHIN GROUP (ORDER BY price_brl)
        FILTER (WHERE kind = 'sale')                                         AS sale_p25,
    PERCENTILE_CONT(0.75) WITHIN GROUP (ORDER BY price_brl)
        FILTER (WHERE kind = 'sale')                                         AS sale_p75,
    MIN(price_brl)        FILTER (WHERE kind = 'listing')                    AS listing_min,
    AVG(price_brl)        FILTER (WHERE kind = 'listing')                    AS listing_avg,
    NOW()
FROM price_history
WHERE observed_at >= $1::date
  AND observed_at <  ($1::date + INTERVAL '1 day')
GROUP BY variant_id, condition, source
ON CONFLICT (variant_id, condition, source, day) DO UPDATE SET
    sales_count    = EXCLUDED.sales_count,
    listings_count = EXCLUDED.listings_count,
    sale_min       = EXCLUDED.sale_min,
    sale_max       = EXCLUDED.sale_max,
    sale_avg       = EXCLUDED.sale_avg,
    sale_median    = EXCLUDED.sale_median,
    sale_p25       = EXCLUDED.sale_p25,
    sale_p75       = EXCLUDED.sale_p75,
    listing_min    = EXCLUDED.listing_min,
    listing_avg    = EXCLUDED.listing_avg,
    last_updated   = NOW()`

// RebuildDay executa a agregação inteira no servidor para o dia indicado.
// Devolve o número de linhas afetadas (inseridas + atualizadas).
func (r *PriceDailyRepo) RebuildDay(ctx context.Context, day time.Time) (int64, error) {
	tag, err := r.pool.Exec(ctx, rebuildDaySQL, day)
	if err != nil {
		return 0, fmt.Errorf("rebuild price_daily for %s: %w", day.Format("2006-01-02"), err)
	}
	return tag.RowsAffected(), nil
}

const getLatestByVariantSQL = `
SELECT sale_min, sale_avg, sale_max, day
FROM price_daily
WHERE variant_id = $1 AND condition = $2::card_condition
ORDER BY day DESC
LIMIT 1`

// GetLatestByVariant retorna o resumo de preço mais recente de uma variante numa
// condição. Retorna nil, nil quando não houver nenhum dado em price_daily para
// essa combinação — o caller deve tratar nil como "sem dados disponíveis".
func (r *PriceDailyRepo) GetLatestByVariant(ctx context.Context, variantID uuid.UUID, condition string) (*PriceSummary, error) {
	var s PriceSummary
	var day time.Time

	err := r.pool.QueryRow(ctx, getLatestByVariantSQL, variantID, condition).Scan(
		&s.MinBRL, &s.AvgBRL, &s.MaxBRL, &day,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest price_daily: %w", err)
	}
	s.LastUpdated = day
	return &s, nil
}
