// Package card define os tipos de domínio para cartas, sets e variantes.
// Toda formação de preço acontece a nível de Variant — Card é apenas a
// identidade impressa, sem distinção de acabamento.
package card

import (
	"time"

	"github.com/google/uuid"
)

// Set representa uma coleção/expansão de Pokémon TCG.
type Set struct {
	ID          uuid.UUID  `json:"id"`
	Code        string     `json:"code"`
	Name        string     `json:"name"`
	Series      string     `json:"series,omitempty"`
	Language    Language   `json:"language"`
	ReleaseDate *time.Time `json:"release_date,omitempty"`
	TotalCards  int        `json:"total_cards,omitempty"`
	ImageURL    string     `json:"image_url,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Language enumera os idiomas suportados pela base de cartas.
type Language string

const (
	LanguagePortuguese Language = "pt"
	LanguageEnglish    Language = "en"
	LanguageJapanese   Language = "jp"
)

// Card representa uma carta de TCG, identificada por (set, número).
// As variações de acabamento (holo, master ball, etc.) ficam em Variant.
type Card struct {
	ID            uuid.UUID         `json:"id"`
	SetID         uuid.UUID         `json:"set_id"`
	Number        string            `json:"number"`
	Name          string            `json:"name"`
	Rarity        string            `json:"rarity,omitempty"`
	Supertype     string            `json:"supertype,omitempty"`
	Subtypes      []string          `json:"subtypes,omitempty"`
	Types         []string          `json:"types,omitempty"`
	HP            int               `json:"hp,omitempty"`
	Illustrator   string            `json:"illustrator,omitempty"`
	ImageSmallURL string            `json:"image_small_url,omitempty"`
	ImageLargeURL string            `json:"image_large_url,omitempty"`
	ExternalIDs   map[string]string `json:"external_ids,omitempty"`
	CreatedAt     time.Time         `json:"created_at"`
	UpdatedAt     time.Time         `json:"updated_at"`
}

// Finish identifica o acabamento físico de uma variante. Espelha o ENUM
// variant_finish do banco — qualquer mudança aqui exige migration nova.
type Finish string

const (
	FinishNormal            Finish = "normal"
	FinishHolo              Finish = "holo"
	FinishReverseHolo       Finish = "reverse_holo"
	FinishMasterBallMirror  Finish = "master_ball_mirror"
	FinishPokeBallMirror    Finish = "poke_ball_mirror"
	FinishCosmosHolo        Finish = "cosmos_holo"
	FinishGalaxyHolo        Finish = "galaxy_holo"
	FinishTextured          Finish = "textured"
	FinishGoldEtched        Finish = "gold_etched"
	FinishFirstEdition      Finish = "first_edition"
	FinishShadowless        Finish = "shadowless"
	FinishUnlimited         Finish = "unlimited"
)

// Variant é a unidade real de precificação. A combinação (CardID, Finish, Label)
// é única por construção (constraint UNIQUE no banco).
type Variant struct {
	ID        uuid.UUID `json:"id"`
	CardID    uuid.UUID `json:"card_id"`
	Finish    Finish    `json:"finish"`
	Label     string    `json:"label,omitempty"`
	IsPromo   bool      `json:"is_promo"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// CardWithSet é uma view de leitura que junta uma carta ao seu set.
// Útil para endpoints de busca onde a UI precisa exibir "Charizard ex (sv8 — Surging Sparks)".
type CardWithSet struct {
	Card Card `json:"card"`
	Set  Set  `json:"set"`
}
