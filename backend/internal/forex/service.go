package forex

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
)

// Service é a fachada de cotação.
//
// Pipeline de Quote(currency, day):
//
//  1. BRL é identidade (rate = 1) — short-circuit.
//  2. Cache em memória (chave: currency|day).
//  3. Banco — última cotação <= day.
//  4. Provider — busca no BCB; se sucesso, persiste e cacheia.
//  5. Fallback — se o provider devolver ErrRateUnavailable (fim de semana),
//     varre até maxFallbackDays dias para trás.
type Service struct {
	repo     *postgres.ForexRepo
	provider Provider

	maxFallbackDays int

	mu    sync.RWMutex
	cache map[cacheKey]Quote
}

type cacheKey struct {
	currency string
	day      string // YYYY-MM-DD para evitar issues de timezone
}

// NewService monta o serviço com fallback de até 7 dias úteis para trás.
func NewService(repo *postgres.ForexRepo, provider Provider) *Service {
	return &Service{
		repo:            repo,
		provider:        provider,
		maxFallbackDays: 7,
		cache:           make(map[cacheKey]Quote),
	}
}

// Quote devolve a cotação mais aplicável (currency → BRL) na data informada.
// Para BRL, devolve rate=1 sem tocar em I/O.
func (s *Service) Quote(ctx context.Context, currency string, day time.Time) (Quote, error) {
	if currency == "BRL" {
		return Quote{
			Currency:  "BRL",
			RateToBRL: decimal.NewFromInt(1),
			QuotedAt:  day,
			Source:    "identity",
		}, nil
	}

	dayUTC := day.UTC().Truncate(24 * time.Hour)
	key := cacheKey{currency: currency, day: dayUTC.Format("2006-01-02")}

	// Cache.
	s.mu.RLock()
	if q, ok := s.cache[key]; ok {
		s.mu.RUnlock()
		return q, nil
	}
	s.mu.RUnlock()

	// Banco — cotação mais recente <= day.
	if rec, err := s.repo.LatestOnOrBefore(ctx, currency, dayUTC); err == nil {
		q := Quote{
			Currency:  rec.Currency,
			RateToBRL: rec.RateToBRL,
			QuotedAt:  rec.QuotedAt,
			Source:    rec.Source,
		}
		s.cachePut(key, q)
		return q, nil
	} else if !errors.Is(err, postgres.ErrNotFound) {
		return Quote{}, fmt.Errorf("forex repo: %w", err)
	}

	// Provider, com fallback dia-a-dia.
	for offset := 0; offset <= s.maxFallbackDays; offset++ {
		try := dayUTC.AddDate(0, 0, -offset)
		q, err := s.provider.Fetch(ctx, currency, try)
		if errors.Is(err, ErrRateUnavailable) {
			continue
		}
		if err != nil {
			return Quote{}, fmt.Errorf("forex provider: %w", err)
		}

		// Persiste a cotação encontrada e cacheia.
		fr := &postgres.ForexRate{
			Currency:  q.Currency,
			RateToBRL: q.RateToBRL,
			QuotedAt:  q.QuotedAt,
			Source:    q.Source,
		}
		if err := s.repo.Upsert(ctx, fr); err != nil {
			return Quote{}, fmt.Errorf("forex persist: %w", err)
		}
		s.cachePut(key, q)
		return q, nil
	}

	return Quote{}, ErrRateUnavailable
}

func (s *Service) cachePut(k cacheKey, q Quote) {
	s.mu.Lock()
	s.cache[k] = q
	s.mu.Unlock()
}

// Invalidate limpa o cache em memória — útil quando um job de retro-correção
// regrava cotações no banco.
func (s *Service) Invalidate() {
	s.mu.Lock()
	s.cache = make(map[cacheKey]Quote)
	s.mu.Unlock()
}
