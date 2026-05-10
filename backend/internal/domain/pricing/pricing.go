// Package pricing define os tipos de domínio para histórico de preços e
// agregações diárias. Todo valor monetário usa shopspring/decimal — nunca float.
package pricing

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Condition reflete o estado físico/conservação da carta.
type Condition string

const (
	ConditionNearMint         Condition = "NM"
	ConditionLightlyPlayed    Condition = "LP"
	ConditionModeratelyPlayed Condition = "MP"
	ConditionHeavilyPlayed    Condition = "HP"
	ConditionDamaged          Condition = "DMG"
	ConditionGraded           Condition = "GRADED"
)

// ConditionFromTCG converte strings de condição (NM, LP, MP, HP, DMG) para Condition.
func ConditionFromTCG(s string) Condition {
	switch strings.ToUpper(s) {
	case "NM", "NEAR MINT":
		return ConditionNearMint
	case "LP", "LIGHTLY PLAYED":
		return ConditionLightlyPlayed
	case "MP", "MODERATELY PLAYED":
		return ConditionModeratelyPlayed
	case "HP", "HEAVILY PLAYED":
		return ConditionHeavilyPlayed
	case "DMG", "DAMAGED":
		return ConditionDamaged
	}
	return ""
}

// Source identifica de onde veio uma observação de preço.
type Source string

const (
	SourceMercadoTCG       Source = "mercadotcg"
	SourceMercadoLivre     Source = "mercadolivre"
	SourceShopee           Source = "shopee"
	SourceTCGPlayer        Source = "tcgplayer"
	SourceCardmarket       Source = "cardmarket"
	SourceEbay             Source = "ebay"
	SourceYahooAuctionsJP  Source = "yahoo_auctions_jp"
	SourceLigaPokemon      Source = "ligapokemon"
	SourceManual           Source = "manual"
)

// Currency é a moeda original da observação. price_brl em Observation
// guarda sempre o valor convertido pela cotação vigente.
type Currency string

const (
	CurrencyBRL Currency = "BRL"
	CurrencyUSD Currency = "USD"
	CurrencyJPY Currency = "JPY"
	CurrencyEUR Currency = "EUR"
)

// Kind diferencia uma venda real de um anúncio ou lance — peso usado no
// cálculo de "preço justo" pode variar por kind.
type Kind string

const (
	KindSale    Kind = "sale"
	KindListing Kind = "listing"
	KindBid     Kind = "bid"
)

// Observation é uma linha bruta da tabela price_history.
//
// price_original + currency são auditáveis (preservam a fonte). PriceBRL é a
// projeção em moeda local feita com FxRateUsed na hora da ingestão; nunca
// é recalculada a posteriori para preservar consistência histórica.
type Observation struct {
	ID            uuid.UUID       `json:"id"`
	VariantID     uuid.UUID       `json:"variant_id"`
	Condition     Condition       `json:"condition"`
	Grade         string          `json:"grade,omitempty"`
	Source        Source          `json:"source"`
	Kind          Kind            `json:"kind"`
	PriceOriginal decimal.Decimal `json:"price_original"`
	Currency      Currency        `json:"currency"`
	PriceBRL      decimal.Decimal `json:"price_brl"`
	FxRateUsed    decimal.Decimal `json:"fx_rate_used"`
	Quantity      int             `json:"quantity"`
	ExternalURL   string          `json:"external_url,omitempty"`
	ExternalID    string          `json:"external_id,omitempty"`
	SellerCountry string          `json:"seller_country,omitempty"`
	ObservedAt    time.Time       `json:"observed_at"`
	IngestedAt    time.Time       `json:"ingested_at"`
}

// DailyPoint é uma linha agregada de price_daily — o que alimenta gráficos.
// Todos os campos estatísticos podem ser nulos quando não houver vendas
// no dia (por isso *decimal.Decimal).
type DailyPoint struct {
	VariantID     uuid.UUID        `json:"variant_id"`
	Condition     Condition        `json:"condition"`
	Source        Source           `json:"source"`
	Day           time.Time        `json:"day"`
	SalesCount    int              `json:"sales_count"`
	ListingsCount int              `json:"listings_count"`
	SaleMin       *decimal.Decimal `json:"sale_min,omitempty"`
	SaleMax       *decimal.Decimal `json:"sale_max,omitempty"`
	SaleAvg       *decimal.Decimal `json:"sale_avg,omitempty"`
	SaleMedian    *decimal.Decimal `json:"sale_median,omitempty"`
	SaleP25       *decimal.Decimal `json:"sale_p25,omitempty"`
	SaleP75       *decimal.Decimal `json:"sale_p75,omitempty"`
	ListingMin    *decimal.Decimal `json:"listing_min,omitempty"`
	ListingAvg    *decimal.Decimal `json:"listing_avg,omitempty"`
	LastUpdated   time.Time        `json:"last_updated"`
}
