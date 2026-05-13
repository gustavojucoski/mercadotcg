// Package tcgdex provides a client for the TCGDex REST API (https://api.tcgdex.net/v2).
// TCGDex covers the Pokémon TCG (main game + TCG Pocket) with multilingual support.
// No API key is required. Rate limit: ~1 req/s recommended.
package tcgdex

// Serie is a card series/set grouping as returned by the TCGDex API.
type Serie struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CardCount holds the card count breakdown for a set.
type CardCount struct {
	Total    int `json:"total"`    // includes secret rares and promos
	Official int `json:"official"` // printed total (e.g. 198 in SVI)
	Holo     int `json:"holo"`
	Normal   int `json:"normal"`
	Reverse  int `json:"reverse"`
}

// Abbreviation holds the official set abbreviation (e.g. "SVI" for Scarlet & Violet base).
type Abbreviation struct {
	Official string `json:"official"`
}

// SetSummary is a lightweight set entry as returned by GET /sets (list endpoint).
// Does not include the full card list.
type SetSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Logo      string    `json:"logo"`
	Symbol    string    `json:"symbol"`
	CardCount CardCount `json:"cardCount"`
}

// Set is the full set object as returned by GET /sets/{id}.
// Includes the list of cards and all metadata.
type Set struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Serie        Serie        `json:"serie"`
	CardCount    CardCount    `json:"cardCount"`
	ReleaseDate  string       `json:"releaseDate"` // ISO 8601: "2023-03-31" or "2023/03/31"
	Logo         string       `json:"logo"`
	Symbol       string       `json:"symbol"`
	Abbreviation Abbreviation `json:"abbreviation"`
	Cards        []CardRef    `json:"cards"`
}

// CardRef is a minimal card reference embedded in the Set.Cards list.
type CardRef struct {
	ID      string `json:"id"`      // global card ID, e.g. "sv01-1"
	LocalID string `json:"localId"` // position within the set, e.g. "001"
	Name    string `json:"name"`
	Image   string `json:"image"` // base URL without extension
}

// Variants describes which finish variants exist for a card.
// These flags map directly to the variant_finish ENUM in the database.
type Variants struct {
	FirstEdition bool `json:"firstEdition"`
	Holo         bool `json:"holo"`
	Normal       bool `json:"normal"`
	Reverse      bool `json:"reverse"`
	WPromo       bool `json:"wPromo"`
}

// VariantDetailed is an optional richer variant descriptor.
// TCGDex does not consistently return this field; prefer Variants for logic.
type VariantDetailed struct {
	Type      string `json:"type"`
	Size      string `json:"size"`
	VariantID string `json:"variantId"`
}

// Card is the full card object as returned by GET /cards/{id}.
type Card struct {
	ID               string            `json:"id"`
	LocalID          string            `json:"localId"`
	Name             string            `json:"name"`
	Category         string            `json:"category"` // "Pokemon", "Trainer", "Energy"
	Rarity           string            `json:"rarity"`
	Image            string            `json:"image"`
	Variants         Variants          `json:"variants"`
	VariantsDetailed []VariantDetailed `json:"variants_detailed"`
}
