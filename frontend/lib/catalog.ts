import type {
  AutocompleteItem,
  CardDetail,
  CardInSet,
  SetCardsResponse,
  SetListResponse,
  TCGSeries,
  TCGSet,
} from './types'

import { API_INTERNAL as API } from './config'

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
  const res = await fetch(url, { cache: 'no-store' })
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

export async function fetchAllSetCards(
  tcg: string,
  code: string,
): Promise<CardInSet[]> {
  const first = await fetchSetCards(tcg, code, 1, 200)
  const total = first.total
  if (total <= 200) return first.cards
  const remainingPages = Math.ceil((total - 200) / 200)
  const rest = await Promise.all(
    Array.from({ length: remainingPages }, (_, i) =>
      fetchSetCards(tcg, code, i + 2, 200),
    ),
  )
  return [...first.cards, ...rest.flatMap(r => r.cards)]
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
