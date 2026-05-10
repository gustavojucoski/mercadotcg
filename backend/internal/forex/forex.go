// Package forex normaliza preços em moedas estrangeiras para BRL.
//
// Arquitetura em 3 camadas:
//
//   - Provider — abstração sobre a fonte oficial (ex.: BCB PTAX). Faz I/O HTTP.
//   - Repo     — persistência em forex_rates (postgres).
//   - Service  — fachada com cache em memória, consultada pelo serviço de
//                pricing na hora da ingestão.
//
// O contrato Provider permite trocar a fonte (BCB, OpenExchangeRates, manual)
// sem mexer no Service.
package forex

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
)

// ErrRateUnavailable indica que nenhum provedor conseguiu cotar a moeda no
// dia solicitado (nem no fallback de dias anteriores).
var ErrRateUnavailable = errors.New("forex: cotação indisponível")

// Quote é a unidade que circula entre as camadas. RateToBRL representa
// quantos BRL valem 1 unidade da moeda original.
type Quote struct {
	Currency  string          // ex.: "USD", "JPY", "EUR"
	RateToBRL decimal.Decimal // 1 [Currency] = RateToBRL [BRL]
	QuotedAt  time.Time       // dia da cotação
	Source    string          // ex.: "bcb"
}

// Provider obtém uma cotação para um dia específico. Implementações são
// responsáveis por sua própria política de retry/timeout.
type Provider interface {
	// Name é usado para preencher forex_rates.source.
	Name() string

	// Fetch devolve a cotação do dia. Pode retornar ErrRateUnavailable se a
	// fonte não publica cotação no dia (fim de semana/feriado).
	Fetch(ctx context.Context, currency string, day time.Time) (Quote, error)
}
