// Package matching define o tipo de domínio para o mapeamento entre
// nossas variantes (`card_variants`) e os identificadores das fontes
// externas (LigaPokemon, TCGplayer, eBay, Cardmarket).
//
// Este mapeamento é o que permite consolidar séries de preço de várias
// fontes em uma única série temporal por variante.
package matching

import (
	"time"

	"github.com/google/uuid"

	"github.com/gustavojucoski/mercadotcg/backend/internal/domain/pricing"
)

// MatchCandidate é uma linha de match_candidates — quarentena para
// observações cujo score de matching ficou abaixo de 60.
// Um operador pode aceitar, rejeitar ou adiar a resolução via painel admin.
type MatchCandidate struct {
	ID                     uuid.UUID  `json:"id"`
	Source                 pricing.Source `json:"source"`
	ExternalID             string     `json:"external_id"`
	RawTitle               string     `json:"raw_title"`
	RawNumber              string     `json:"raw_number,omitempty"`
	RawSetCode             string     `json:"raw_set_code,omitempty"`
	BestCandidateVariantID *uuid.UUID `json:"best_candidate_variant_id,omitempty"`
	BestScore              int        `json:"best_score"`
	CreatedAt              time.Time  `json:"created_at"`
	ReviewedBy             *uuid.UUID `json:"reviewed_by,omitempty"`
	ReviewedAt             *time.Time `json:"reviewed_at,omitempty"`
	Resolution             string     `json:"resolution,omitempty"` // "accepted", "rejected", "deferred"
}

// ExternalCardRef é uma linha de external_card_refs.
//
// Confidence é um score 0–100 que indica quão certo o matcher está de que
// (variant_id) representa de fato (source, external_id). Matchings manuais
// são gravados com confidence = 100; matchings automáticos por
// nome+número podem ficar abaixo disso e ser revisados.
//
// NeedsReview é setado pelo matching service quando o score ficou entre 60–84:
// a ref existe e pode ser usada, mas um operador deve confirmá-la.
type ExternalCardRef struct {
	ID          uuid.UUID      `json:"id"`
	VariantID   uuid.UUID      `json:"variant_id"`
	Source      pricing.Source `json:"source"`
	ExternalID  string         `json:"external_id"`
	ExternalURL string         `json:"external_url,omitempty"`
	Language    string         `json:"language"`
	Confidence  int            `json:"confidence"`
	NeedsReview bool           `json:"needs_review"`
	RawTitle    string         `json:"raw_title,omitempty"`
	MatchedAt   time.Time      `json:"matched_at"`
}
