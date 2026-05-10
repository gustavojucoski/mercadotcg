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

// ExternalCardRef é uma linha de external_card_refs.
//
// Confidence é um score 0–100 que indica quão certo o matcher está de que
// (variant_id) representa de fato (source, external_id). Matchings manuais
// são gravados com confidence = 100; matchings automáticos por
// nome+número podem ficar abaixo disso e ser revisados.
type ExternalCardRef struct {
	ID          uuid.UUID       `json:"id"`
	VariantID   uuid.UUID       `json:"variant_id"`
	Source      pricing.Source  `json:"source"`
	ExternalID  string          `json:"external_id"`
	ExternalURL string          `json:"external_url,omitempty"`
	Language    string          `json:"language"`
	Confidence  int             `json:"confidence"`
	RawTitle    string          `json:"raw_title,omitempty"`
	MatchedAt   time.Time       `json:"matched_at"`
}
