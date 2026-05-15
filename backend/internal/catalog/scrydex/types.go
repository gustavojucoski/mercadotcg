// Package scrydex provides a client for the Scrydex catalog API
// (https://api.scrydex.com). It covers Pokémon TCG expansions and cards with
// marketplace variant data (TCGPlayer, Cardmarket).
//
// Authentication: X-Api-Key + X-Team-ID headers (casing is exact — the API
// returns 401 if you deviate).
package scrydex

// Translation holds the English-language equivalents of fields that the
// Scrydex API returns in a non-English language (e.g. Japanese set/card names).
// If a top-level field is absent for a non-EN card, Scrydex already falls back
// to the English value — but explicit EN translations are available here.
type Translation struct {
	Name string `json:"name"`
}

// Translations is the top-level translations object returned by Scrydex.
type Translations struct {
	En Translation `json:"en"`
}

// Expansion represents a single Pokémon TCG set as returned by
// GET /pokemon/v1/expansions.
//
// JSON field names use the exact casing returned by the API (snake_case for
// multi-word fields). Mismatches silently produce zero values, so any change
// here must be validated against a live API response.
type Expansion struct {
	ID           string       `json:"id"`
	Code         string       `json:"code"`          // expansion code, e.g. "SSP"
	Name         string       `json:"name"`
	Language     string       `json:"language"`      // "English", "Japanese", "Korean"
	LanguageCode string       `json:"language_code"` // "EN", "JA", "KO"
	Region       string       `json:"region"`
	ReleaseDate  string       `json:"release_date"`  // e.g. "2024/11/08"
	Total        int          `json:"total"`         // total cards including secret rares
	PrintedTotal int          `json:"printed_total"` // number printed on cards, e.g. 191
	LogoURL      string       `json:"logo"`
	SymbolURL    string       `json:"symbol"`
	IsOnlineOnly bool         `json:"is_online_only"`
	Translations Translations `json:"translations"`
	// Series is the free-form series name from Scrydex. There is no external
	// SeriesID — the importer uses this name as the upsert key for card_series.
	Series string `json:"series"`
}

// Card represents a single card as returned by
// GET /pokemon/v1/expansions/{id}/cards.
type Card struct {
	ID           string        `json:"id"`      // e.g. "sv8-199", "tcgp-A1-35"
	Number       string        `json:"number"`  // e.g. "1"
	Name         string        `json:"name"`
	Rarity       string        `json:"rarity"`
	Artist       string        `json:"artist"`
	Translations Translations  `json:"translations"`
	Images       []CardImage   `json:"images"`
	Variants     []CardVariant `json:"variants"`
}

// LargeImageURL returns the large front image URL, or "" if none.
func (c *Card) LargeImageURL() string {
	for _, img := range c.Images {
		if img.Type == "front" {
			return img.Large
		}
	}
	if len(c.Images) > 0 {
		return c.Images[0].Large
	}
	return ""
}

// SmallImageURL returns the small front image URL, or "" if none.
func (c *Card) SmallImageURL() string {
	for _, img := range c.Images {
		if img.Type == "front" {
			return img.Small
		}
	}
	if len(c.Images) > 0 {
		return c.Images[0].Small
	}
	return ""
}

// CardImage is one entry in the images array returned by the Scrydex API.
// Each card/variant may have multiple images (e.g. front + back).
type CardImage struct {
	Type   string `json:"type"`   // "front", "back"
	Small  string `json:"small"`
	Medium string `json:"medium"`
	Large  string `json:"large"`
}

// CardVariant describes a finish variant of a card (e.g. holofoil, reverse
// holo) and its marketplace product references.
type CardVariant struct {
	Name         string        `json:"name"`         // e.g. "holofoil", "reverseHolofoil"
	Images       []CardImage   `json:"images"`
	Marketplaces []Marketplace `json:"marketplaces"`
}

// Marketplace links a card variant to an external marketplace product.
// Note: marketplace names are lowercase in the API ("tcgplayer", "cardmarket").
type Marketplace struct {
	Name      string `json:"name"`       // "tcgplayer", "cardmarket"
	ProductID string `json:"product_id"` // product ID on that marketplace
}

// listExpansionsResponse is the envelope for GET /pokemon/v1/expansions.
// The API returns a top-level object with a data array and pagination metadata.
type listExpansionsResponse struct {
	Data       []Expansion `json:"data"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`   // API field is snake_case
	TotalCount int         `json:"total_count"` // API field is snake_case
}

// listCardsResponse is the envelope for GET /pokemon/v1/expansions/{id}/cards.
type listCardsResponse struct {
	Data       []Card `json:"data"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`   // API field is snake_case
	TotalCount int    `json:"total_count"` // API field is snake_case
}
