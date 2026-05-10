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
)

// ForexRate é uma cotação diária persistida em forex_rates.
// Mantida neste pacote (e não em domain/) porque é puramente de infraestrutura
// — quem faz lógica de negócio com câmbio é internal/forex.
type ForexRate struct {
	ID        uuid.UUID
	Currency  string
	RateToBRL decimal.Decimal
	QuotedAt  time.Time
	Source    string
	CreatedAt time.Time
}

// ForexRepo persiste e consulta cotações.
type ForexRepo struct {
	pool *pgxpool.Pool
}

// NewForexRepo devolve um repositório pronto para uso.
func NewForexRepo(pool *pgxpool.Pool) *ForexRepo {
	return &ForexRepo{pool: pool}
}

const upsertForexSQL = `
INSERT INTO forex_rates (currency, rate_to_brl, quoted_at, source)
VALUES ($1, $2, $3, $4)
ON CONFLICT (currency, quoted_at, source) DO UPDATE SET
    rate_to_brl = EXCLUDED.rate_to_brl
RETURNING id, created_at`

// Upsert grava ou atualiza a cotação. Idempotente — re-rodar o job do dia
// sobrescreve a cotação se a fonte ajustar publicação.
func (r *ForexRepo) Upsert(ctx context.Context, fr *ForexRate) error {
	err := r.pool.QueryRow(ctx, upsertForexSQL,
		fr.Currency, fr.RateToBRL, fr.QuotedAt, fr.Source,
	).Scan(&fr.ID, &fr.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert forex_rates: %w", err)
	}
	return nil
}

const latestOnOrBeforeSQL = `
SELECT id, currency, rate_to_brl, quoted_at, source, created_at
FROM forex_rates
WHERE currency = $1 AND quoted_at <= $2
ORDER BY quoted_at DESC
LIMIT 1`

// LatestOnOrBefore devolve a cotação mais recente para a moeda em uma data
// igual ou anterior a `day`. Cobre o caso clássico de fim de semana / feriado:
// se a observação é de um sábado, usamos a cotação de sexta.
//
// Retorna ErrNotFound se ainda não existe cotação para essa moeda <= day.
func (r *ForexRepo) LatestOnOrBefore(ctx context.Context, currency string, day time.Time) (ForexRate, error) {
	var fr ForexRate
	err := r.pool.QueryRow(ctx, latestOnOrBeforeSQL, currency, day).Scan(
		&fr.ID, &fr.Currency, &fr.RateToBRL, &fr.QuotedAt, &fr.Source, &fr.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ForexRate{}, ErrNotFound
	}
	if err != nil {
		return ForexRate{}, fmt.Errorf("select latest forex: %w", err)
	}
	return fr, nil
}
