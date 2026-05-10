// Package pricesignal computa sinais agregados de preço por fonte.
//
// Para uma (variante, condição), devolve o que cada fonte (LigaPokemon,
// TCGplayer, eBay, etc.) viu nos últimos N dias: média ponderada por
// volume, min/max, contagem de vendas e o dia da última venda.
//
// Lê de price_daily (já pré-agregada por dia) — nunca de price_history
// crua, porque varredura em milhões de linhas mata a latência.
package pricesignal

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
)

// PerSourceSignal é o resumo de uma fonte para uma janela de N dias.
type PerSourceSignal struct {
	Source        pricing.Source   `json:"source"`
	SalesCount    int              `json:"sales_count"`
	WeightedAvg   *decimal.Decimal `json:"weighted_avg_brl,omitempty"`
	Min           *decimal.Decimal `json:"min_brl,omitempty"`
	Max           *decimal.Decimal `json:"max_brl,omitempty"`
	LastSaleDay   *time.Time       `json:"last_sale_day,omitempty"`
}

// Signal agrupa todas as fontes para uma (variante, condição).
type Signal struct {
	VariantID uuid.UUID         `json:"variant_id"`
	Condition pricing.Condition `json:"condition"`
	WindowDays int              `json:"window_days"`
	Sources    []PerSourceSignal `json:"sources"`
}

// Service expõe as operações de price signal.
type Service struct {
	pool *pgxpool.Pool

	defaultWindowDays int
}

// NewService monta o Service com janela default de 30 dias.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, defaultWindowDays: 30}
}

const querySignalSQL = `
SELECT
    source,
    SUM(sales_count)                                                   AS total_sales,
    -- Média ponderada por volume: Σ(sale_avg * sales_count) / Σ(sales_count)
    CASE WHEN SUM(sales_count) = 0 THEN NULL
         ELSE SUM(sale_avg * sales_count) / SUM(sales_count)
    END                                                                AS weighted_avg,
    MIN(sale_min)                                                      AS overall_min,
    MAX(sale_max)                                                      AS overall_max,
    MAX(day) FILTER (WHERE sales_count > 0)                            AS last_sale_day
FROM price_daily
WHERE variant_id = $1
  AND condition  = $2
  AND day >= (CURRENT_DATE - ($3::int * INTERVAL '1 day'))
  AND sales_count > 0
GROUP BY source
ORDER BY source`

// For devolve o sinal agregado para a janela default (30 dias).
func (s *Service) For(
	ctx context.Context,
	variantID uuid.UUID,
	condition pricing.Condition,
) (Signal, error) {
	return s.ForWindow(ctx, variantID, condition, s.defaultWindowDays)
}

// ForWindow é como For, mas com janela customizável (em dias).
func (s *Service) ForWindow(
	ctx context.Context,
	variantID uuid.UUID,
	condition pricing.Condition,
	windowDays int,
) (Signal, error) {
	if windowDays <= 0 || windowDays > 365 {
		windowDays = 30
	}

	rows, err := s.pool.Query(ctx, querySignalSQL, variantID, string(condition), windowDays)
	if err != nil {
		return Signal{}, fmt.Errorf("query price signal: %w", err)
	}
	defer rows.Close()

	signal := Signal{
		VariantID:  variantID,
		Condition:  condition,
		WindowDays: windowDays,
		Sources:    []PerSourceSignal{},
	}

	for rows.Next() {
		var ps PerSourceSignal
		var src string
		if err := rows.Scan(
			&src, &ps.SalesCount, &ps.WeightedAvg, &ps.Min, &ps.Max, &ps.LastSaleDay,
		); err != nil {
			return Signal{}, fmt.Errorf("scan price signal: %w", err)
		}
		ps.Source = pricing.Source(src)

		// O CASE WHEN devolve NUMERIC sem casas fixas; truncamos para 2 (BRL).
		if ps.WeightedAvg != nil {
			rounded := ps.WeightedAvg.Round(2)
			ps.WeightedAvg = &rounded
		}
		signal.Sources = append(signal.Sources, ps)
	}
	return signal, rows.Err()
}

// ----------------------------------------------------------------------------
// Matriz condição × fonte
// ----------------------------------------------------------------------------

// ConditionSignal agrupa todos os sinais de fonte para uma condição específica.
type ConditionSignal struct {
	Condition pricing.Condition `json:"condition"`
	Sources   []PerSourceSignal `json:"sources"`
}

// SignalsByCondition é o retorno de ByConditions: lista de condições, cada
// uma com sua lista de fontes. Vazio se a variante não tem dados na janela.
type SignalsByCondition struct {
	VariantID  uuid.UUID         `json:"variant_id"`
	WindowDays int               `json:"window_days"`
	Conditions []ConditionSignal `json:"conditions"`
}

const querySignalsByConditionSQL = `
SELECT
    condition,
    source,
    SUM(sales_count) AS total_sales,
    CASE WHEN SUM(sales_count) = 0 THEN NULL
         ELSE SUM(sale_avg * sales_count) / SUM(sales_count)
    END AS weighted_avg,
    MIN(sale_min) AS overall_min,
    MAX(sale_max) AS overall_max,
    MAX(day) FILTER (WHERE sales_count > 0) AS last_sale_day
FROM price_daily
WHERE variant_id = $1
  AND day >= (CURRENT_DATE - ($2::int * INTERVAL '1 day'))
  AND sales_count > 0
GROUP BY condition, source
ORDER BY condition, source`

// ByConditions devolve a matriz completa (condition × source) para uma
// variante na janela informada. Uma única query agrupando por ambas as
// dimensões — evita N round-trips quando a UI precisa exibir todas as
// condições simultaneamente.
func (s *Service) ByConditions(
	ctx context.Context,
	variantID uuid.UUID,
	windowDays int,
) (SignalsByCondition, error) {
	if windowDays <= 0 || windowDays > 365 {
		windowDays = 30
	}

	rows, err := s.pool.Query(ctx, querySignalsByConditionSQL, variantID, windowDays)
	if err != nil {
		return SignalsByCondition{}, fmt.Errorf("query signals by condition: %w", err)
	}
	defer rows.Close()

	out := SignalsByCondition{
		VariantID:  variantID,
		WindowDays: windowDays,
		Conditions: []ConditionSignal{},
	}

	// As linhas vêm ordenadas por condition; agrupamos no laço.
	var current *ConditionSignal
	for rows.Next() {
		var cond, src string
		var ps PerSourceSignal
		if err := rows.Scan(
			&cond, &src, &ps.SalesCount, &ps.WeightedAvg, &ps.Min, &ps.Max, &ps.LastSaleDay,
		); err != nil {
			return SignalsByCondition{}, fmt.Errorf("scan signals by condition: %w", err)
		}
		ps.Source = pricing.Source(src)
		if ps.WeightedAvg != nil {
			rounded := ps.WeightedAvg.Round(2)
			ps.WeightedAvg = &rounded
		}

		if current == nil || string(current.Condition) != cond {
			out.Conditions = append(out.Conditions, ConditionSignal{
				Condition: pricing.Condition(cond),
				Sources:   []PerSourceSignal{},
			})
			current = &out.Conditions[len(out.Conditions)-1]
		}
		current.Sources = append(current.Sources, ps)
	}
	return out, rows.Err()
}
