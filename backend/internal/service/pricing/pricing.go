// Package pricing concentra a lógica de negócio de preços.
//
// A função central é NormalizeBRL: dado um preço em moeda original e a data
// da observação, devolve o preço em BRL e a cotação utilizada. Toda
// observação que entra em price_history passa por aqui — é a única forma de
// preencher (price_brl, fx_rate_used) coerentemente.
package pricing

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/forex"
)

// brlPrecision define o número de casas decimais retidas em BRL.
// Bate com NUMERIC(14,2) das colunas price_brl no banco.
const brlPrecision = 2

// Service agrupa as regras de pricing. Sem estado próprio além das deps.
type Service struct {
	forex *forex.Service
}

// NewService monta o Service com o serviço de câmbio injetado.
func NewService(fx *forex.Service) *Service {
	return &Service{forex: fx}
}

// NormalizationResult é o retorno de NormalizeBRL — agrupa preço convertido
// e a cotação que foi efetivamente usada (para auditoria em price_history).
type NormalizationResult struct {
	PriceBRL   decimal.Decimal
	FxRateUsed decimal.Decimal
	QuotedAt   time.Time
	Source     string
}

// NormalizeBRL converte `price` de `currency` para BRL usando a cotação
// vigente em `observedAt`. Para `currency = BRL`, retorna o próprio valor
// e fxRate=1.
//
// O resultado é arredondado a 2 casas (NUMERIC(14,2)).
func (s *Service) NormalizeBRL(
	ctx context.Context,
	price decimal.Decimal,
	currency pricing.Currency,
	observedAt time.Time,
) (NormalizationResult, error) {
	if price.IsNegative() {
		return NormalizationResult{}, fmt.Errorf("pricing: preço negativo (%s)", price)
	}

	q, err := s.forex.Quote(ctx, string(currency), observedAt)
	if err != nil {
		return NormalizationResult{}, fmt.Errorf("pricing: cotar %s em %s: %w",
			currency, observedAt.Format("2006-01-02"), err)
	}

	priceBRL := price.Mul(q.RateToBRL).Round(brlPrecision)

	return NormalizationResult{
		PriceBRL:   priceBRL,
		FxRateUsed: q.RateToBRL,
		QuotedAt:   q.QuotedAt,
		Source:     q.Source,
	}, nil
}

// FillObservation aplica NormalizeBRL na Observation in-place, deixando-a
// pronta para ser persistida em price_history. Espera-se que VariantID,
// PriceOriginal, Currency e ObservedAt já estejam preenchidos.
func (s *Service) FillObservation(ctx context.Context, o *pricing.Observation) error {
	res, err := s.NormalizeBRL(ctx, o.PriceOriginal, o.Currency, o.ObservedAt)
	if err != nil {
		return err
	}
	o.PriceBRL = res.PriceBRL
	o.FxRateUsed = res.FxRateUsed
	return nil
}
