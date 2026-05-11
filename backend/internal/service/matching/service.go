// internal/service/matching/service.go
//
// Package matching implementa o serviço de resolução de variant_id a partir
// de resultados de scrapers externos.
//
// O algoritmo segue o ADR-020:
//
//	score >= 85 → cria external_card_ref com confidence = score (AutoCreated)
//	             ou devolve AlreadyExists se o ref já existe.
//	60–84       → cria external_card_ref com needs_review = true (AutoCreated).
//	< 60        → salva em match_candidates (quarentena) e retorna Quarantined.
//
// O serviço NÃO grava em price_history — apenas resolve o variant_id e
// gerencia refs/candidatos. A gravação é responsabilidade do pipeline de
// ingestão (cmd/ingest).
package matching

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog/log"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/matching"
	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
	"github.com/gustavojucoski/mercadotcg/backend/internal/repository/postgres"
	"github.com/gustavojucoski/mercadotcg/backend/internal/scraper"
)

// ResolveAction descreve o que o serviço fez com a observação.
type ResolveAction int

const (
	// ActionAlreadyExists indica que já havia um external_card_ref para
	// (source, external_id) — nenhuma escrita foi realizada.
	ActionAlreadyExists ResolveAction = iota

	// ActionAutoCreated indica que um novo external_card_ref foi criado
	// (com ou sem needs_review, dependendo do score).
	ActionAutoCreated

	// ActionQuarantined indica que o score foi insuficiente e a observação
	// foi salva em match_candidates para revisão humana.
	ActionQuarantined
)

// Thresholds de decisão do algoritmo de scoring.
const (
	thresholdAutoConfident = 85 // score >= 85 → cria ref sem needs_review
	thresholdAutoReview    = 60 // 60 <= score < 85 → cria ref com needs_review
	// score < 60 → quarentena
)

// ResolveResult é o retorno público de Resolve.
type ResolveResult struct {
	// VariantID é o UUID da variante resolvida. Zero value quando Action == ActionQuarantined.
	VariantID uuid.UUID

	// Confidence é o score 0–100 calculado pelo algoritmo de scoring.
	Confidence int

	// Action descreve o que o serviço fez.
	Action ResolveAction
}

// Service resolve variant_ids a partir de resultados de scrapers, criando
// ou consultando external_card_refs e, quando necessário, salvando
// match_candidates para revisão humana.
type Service struct {
	pool    *pgxpool.Pool
	refRepo *postgres.ExternalRefRepo
}

// New cria um novo Service. pool é injetado diretamente pois o service
// executa queries de scoring que não pertencem a nenhum repositório existente.
func New(pool *pgxpool.Pool) *Service {
	return &Service{
		pool:    pool,
		refRepo: postgres.NewExternalRefRepo(pool),
	}
}

// Resolve tenta casar um resultado de scraper com uma variante do catálogo.
//
// Fluxo:
//  1. Verifica se já existe external_card_ref para (source, result.ExternalID).
//     Em caso positivo, retorna AlreadyExists com o variant_id existente.
//  2. Executa os 4 passes de scoring (ScoreResult).
//  3. Com os candidatos encontrados, aplica pickVariant para desempate por finish.
//  4. Toma ação conforme o score final (ver thresholds acima).
func (s *Service) Resolve(
	ctx context.Context,
	source pricing.Source,
	result scraper.Result,
	q scraper.Query,
) (ResolveResult, error) {
	// Passo 1: verificar se já existe um ref para este (source, external_id).
	if result.ExternalID != "" {
		ref, err := s.refRepo.GetBySourceID(ctx, source, result.ExternalID)
		if err == nil {
			// Ref já existe — nada a fazer.
			log.Debug().
				Str("source", string(source)).
				Str("external_id", result.ExternalID).
				Stringer("variant_id", ref.VariantID).
				Msg("matching: ref já existe")
			return ResolveResult{
				VariantID:  ref.VariantID,
				Confidence: ref.Confidence,
				Action:     ActionAlreadyExists,
			}, nil
		}
		if !errors.Is(err, postgres.ErrNotFound) {
			return ResolveResult{}, fmt.Errorf("matching.Resolve: consultar ref existente: %w", err)
		}
		// ErrNotFound → segue para scoring.
	}

	// Passo 2: executar os passes de scoring.
	sr, err := ScoreResult(ctx, s.pool, q)
	if err != nil {
		return ResolveResult{}, fmt.Errorf("matching.Resolve: scoring: %w", err)
	}

	log.Debug().
		Str("source", string(source)).
		Str("external_id", result.ExternalID).
		Int("score", sr.score).
		Int("candidates", len(sr.variantIDs)).
		Msg("matching: scoring concluído")

	// Passo 3: se não há candidatos ou score < 60 → quarentena.
	if sr.score < thresholdAutoReview || len(sr.variantIDs) == 0 {
		if err := s.quarantine(ctx, source, result, q, sr.score); err != nil {
			return ResolveResult{}, fmt.Errorf("matching.Resolve: quarentena: %w", err)
		}
		return ResolveResult{
			Confidence: sr.score,
			Action:     ActionQuarantined,
		}, nil
	}

	// Passo 4: desempate de variante.
	variantID, finalScore, err := pickVariant(ctx, s.pool, sr.variantIDs, result.Title, sr.score)
	if err != nil {
		return ResolveResult{}, fmt.Errorf("matching.Resolve: desempate: %w", err)
	}
	if variantID == uuid.Nil {
		// pickVariant não encontrou nada (raro, mas defensivo).
		if err := s.quarantine(ctx, source, result, q, 0); err != nil {
			return ResolveResult{}, fmt.Errorf("matching.Resolve: quarentena (sem variante): %w", err)
		}
		return ResolveResult{Action: ActionQuarantined}, nil
	}

	// Cap do score em 100 (bônus de único candidato pode ultrapassar 95).
	if finalScore > 100 {
		finalScore = 100
	}

	// Passo 5: criar external_card_ref.
	needsReview := finalScore < thresholdAutoConfident
	ref := &matching.ExternalCardRef{
		VariantID:   variantID,
		Source:      source,
		ExternalID:  result.ExternalID,
		ExternalURL: result.URL,
		Language:    result.Language,
		Confidence:  finalScore,
		NeedsReview: needsReview,
		RawTitle:    result.Title,
	}

	if err := s.refRepo.Create(ctx, ref); err != nil {
		if errors.Is(err, postgres.ErrAlreadyExists) {
			// Corrida: outra goroutine criou o ref entre o GetBySourceID e o Create.
			// Busca o existente e retorna AlreadyExists.
			existing, getErr := s.refRepo.GetBySourceID(ctx, source, result.ExternalID)
			if getErr != nil {
				return ResolveResult{}, fmt.Errorf("matching.Resolve: re-buscar ref após race: %w", getErr)
			}
			return ResolveResult{
				VariantID:  existing.VariantID,
				Confidence: existing.Confidence,
				Action:     ActionAlreadyExists,
			}, nil
		}
		return ResolveResult{}, fmt.Errorf("matching.Resolve: criar ref: %w", err)
	}

	log.Info().
		Str("source", string(source)).
		Str("external_id", result.ExternalID).
		Stringer("variant_id", variantID).
		Int("confidence", finalScore).
		Bool("needs_review", needsReview).
		Msg("matching: ref criado")

	return ResolveResult{
		VariantID:  variantID,
		Confidence: finalScore,
		Action:     ActionAutoCreated,
	}, nil
}

// quarantine persiste o resultado em match_candidates para revisão humana.
// Usa upsert (ON CONFLICT) para não duplicar candidatos pendentes.
func (s *Service) quarantine(
	ctx context.Context,
	source pricing.Source,
	result scraper.Result,
	q scraper.Query,
	bestScore int,
) error {
	c := &matching.MatchCandidate{
		Source:     source,
		ExternalID: result.ExternalID,
		RawTitle:   result.Title,
		RawNumber:  q.Number,
		RawSetCode: q.SetCode,
		BestScore:  bestScore,
	}

	log.Debug().
		Str("source", string(source)).
		Str("external_id", result.ExternalID).
		Int("best_score", bestScore).
		Msg("matching: enviando para quarentena")

	if err := s.refRepo.UpsertMatchCandidate(ctx, c); err != nil {
		return fmt.Errorf("quarantine: %w", err)
	}
	return nil
}
