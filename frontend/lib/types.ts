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
