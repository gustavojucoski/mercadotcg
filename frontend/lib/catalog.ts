import type {
  AutocompleteItem,
  CardDetail,
  SetCardsResponse,
  SetListResponse,
  TCGSeries,
  TCGSet,
} from './types'

const API = process.env.NEXT_PUBLIC_API_URL ?? 'http://localhost:8080'

export async function fetchSeries(tcg?: string): Promise<TCGSeries[]> {
  const url = tcg
    ? `${API}/api/v1/series?tcg=${encodeURIComponent(tcg)}`
    : `${API}/api/v1/series`
  const res = await fetch(url, { next: { revalidate: 86400 } })
  if (!res.ok) return []
  return res.json()
}

export async function fetchSets(
  tcg: string,
  page = 1,
  limit = 30,
): Promise<SetListResponse> {
  const url = `${API}/api/v1/sets/${encodeURIComponent(tcg)}?page=${page}&limit=${limit}`
  const res = await fetch(url, { next: { revalidate: 86400 } })
  if (!res.ok) throw new Error(`fetchSets: HTTP ${res.status}`)
  return res.json()
}

export async function fetchSet(tcg: string, code: string): Promise<TCGSet> {
  const url = `${API}/api/v1/sets/${encodeURIComponent(tcg)}/${encodeURIComponent(code)}`
  const res = await fetch(url, { next: { revalidate: 3600 } })
  if (!res.ok) throw new Error(`fetchSet: HTTP ${res.status}`)
  return res.json()
}

export async function fetchSetCards(
  tcg: string,
  code: string,
  page = 1,
  limit = 60,
): Promise<SetCardsResponse> {
  const url = `${API}/api/v1/sets/${encodeURIComponent(tcg)}/${encodeURIComponent(code)}/cards?page=${page}&limit=${limit}`
  const res = await fetch(url, { next: { revalidate: 3600 } })
  if (!res.ok) throw new Error(`fetchSetCards: HTTP ${res.status}`)
  return res.json()
}

export async function fetchCard(slug: string): Promise<CardDetail> {
  const url = `${API}/api/v1/cards/${encodeURIComponent(slug)}`
  const res = await fetch(url, { next: { revalidate: 3600 } })
  if (!res.ok) throw new Error(`fetchCard: HTTP ${res.status}`)
  return res.json()
}

export async function autocompleteCards(
  q: string,
  tcg?: string,
): Promise<AutocompleteItem[]> {
  const params = new URLSearchParams({ q, limit: '8' })
  if (tcg) params.set('tcg', tcg)
  const url = `${API}/api/v1/cards/autocomplete?${params.toString()}`
  const res = await fetch(url)
  if (!res.ok) return []
  return res.json()
}
