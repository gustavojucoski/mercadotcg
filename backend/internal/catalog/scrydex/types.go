// Package scrydex provides a client for the Scrydex catalog API
// (https://api.scrydex.io). It covers Pokémon TCG expansions and cards with
// marketplace variant data (TCGPlayer, Cardmarket).
//
// Authentication: X-Api-Key + X-Team-ID headers (casing is exact — the API
// returns 401 if you deviate).
package scrydex

// Expansion represents a single Pokémon TCG set as returned by
// GET /pokemon/v1/expansions.
type Expansion struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Language     string `json:"language"`    // "en", "ja", "ko"
	Region       string `json:"region"`      // "international", "japan"
	ReleaseDate  string `json:"releaseDate"` // ISO date string, e.g. "2023-03-31"
	PrintedTotal int    `json:"printedTotal"`
	LogoURL      string `json:"logoUrl"`
	SymbolURL    string `json:"symbolUrl"`
	// Series is the free-form series name from Scrydex. There is no external
	// SeriesID — the importer uses this name as the upsert key for card_series.
	Series string `json:"series"`
}

// Card represents a single card as returned by
// GET /pokemon/v1/expansions/{id}/cards.
type Card struct {
	ID       string      `json:"id"`      // e.g. "sv8-199", "tcgp-A1-35"
	Number   string      `json:"number"`  // e.g. "199"
	Name     string      `json:"name"`
	Images   CardImages  `json:"images"`
	Variants []CardVariant `json:"variants"`
}

// CardImages holds the image URLs at three resolutions.
type CardImages struct {
	Small  string `json:"small"`
	Medium string `json:"medium"`
	Large  string `json:"large"`
}

// CardVariant describes a finish variant of a card (e.g. holofoil, reverse
// holo) and its marketplace product references.
type CardVariant struct {
	Name         string        `json:"name"`         // e.g. "holofoil", "reverseHolofoil"
	Images       CardImages    `json:"images"`
	Marketplaces []Marketplace `json:"marketplaces"`
}

// Marketplace links a card variant to an external marketplace product.
type Marketplace struct {
	Name       string `json:"name"`       // "TCGPlayer", "Cardmarket"
	ExternalID string `json:"externalId"` // product ID on that marketplace
}

// listExpansionsResponse is the envelope for GET /pokemon/v1/expansions.
// The API returns a top-level object with a data array and optional pagination
// metadata; we collect all pages internally.
type listExpansionsResponse struct {
	Data       []Expansion `json:"data"`
	Page       int         `json:"page"`
	PageSize   int         `json:"pageSize"`
	TotalCount int         `json:"totalCount"`
}

// listCardsResponse is the envelope for GET /pokemon/v1/expansions/{id}/cards.
type listCardsResponse struct {
	Data       []Card `json:"data"`
	Page       int    `json:"page"`
	PageSize   int    `json:"pageSize"`
	TotalCount int    `json:"totalCount"`
}
