export type Condition = 'NM' | 'LP' | 'MP' | 'HP' | 'DMG' | 'GRADED'
export type Source = 'ligapokemon' | 'tcgplayer' | 'cardmarket' | 'ebay'
export type Currency = 'BRL' | 'USD' | 'EUR' | 'JPY'
export type Kind = 'listing' | 'sale' | 'bid'

export interface PriceResult {
  title: string
  url: string
  image_url?: string
  price: string
  currency: Currency
  condition: Condition
  raw_condition?: string
  language?: string
  stock?: number
  kind?: Kind
  external_id?: string
}

export interface SourceResult {
  source: Source
  duration_ms?: number
  error?: string
  results: PriceResult[]
}

export interface CardInfo {
  id: string
  name: string
  number: string
  set_code: string
  set_name: string
}

export interface SearchResult {
  card?: CardInfo
  fetched_at: string
  sources: SourceResult[]
}

export interface PokemonSet {
  id: string
  name: string
  ptcgoCode: string
  releaseDate: string
  series: string
}

// Catalog types

export interface TCGSeries {
  id: string
  name: string
  name_pt: string
  tcg: string
}

export interface TCGSet {
  id: string
  code: string
  name: string
  name_pt: string
  series: string
  series_pt: string
  series_id: string
  release_date: string | null
  total_cards: number
  image_url: string
  tcg: string
}

export interface SetListResponse {
  tcg: string
  total: number
  page: number
  limit: number
  sets: TCGSet[]
}

export interface CardVariantSummary {
  id: string
  finish: string
  label: string
}

export interface CardInSet {
  id: string
  name: string
  name_pt: string
  collector_number: string
  rarity: string
  supertype: string
  image_small_url: string
  variants: CardVariantSummary[]
}

export interface SetCardsResponse {
  set_code: string
  total: number
  page: number
  limit: number
  cards: CardInSet[]
}

export interface PriceSummary {
  min_brl: string
  avg_brl: string
  max_brl: string
  last_updated: string
}

export interface CardVariantDetail {
  id: string
  finish: string
  label: string
  is_promo: boolean
  price_summary: PriceSummary | null
}

export interface CardDetail {
  card: {
    id: string
    name: string
    name_pt: string
    collector_number: string
    rarity: string
    supertype: string
    subtypes: string[]
    types: string[]
    image_small_url: string
    image_large_url: string
  }
  set: TCGSet
  variants: CardVariantDetail[]
}

export interface AutocompleteItem {
  id: string
  name: string
  name_pt: string
  collector_number: string
  set_code: string
  set_name: string
  image_small_url: string
  slug: string
}
