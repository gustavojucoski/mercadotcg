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

interface FetchSetsOptions {
  page?: number
  limit?: number
  q?: string
}

export async function fetchSets(
  tcg: string,
  options: FetchSetsOptions = {},
): Promise<SetListResponse> {
  const { page = 1, limit = 30, q = '' } = options
  const params = new URLSearchParams({
    page: String(page),
    limit: String(limit),
  })
  if (q.trim()) params.set('q', q.trim())
  const url = `${API}/api/v1/sets/${encodeURIComponent(tcg)}?${params}`
  const res = await fetch(url, {
    cache: q.trim() ? 'no-store' : undefined,
    next: q.trim() ? undefined : { revalidate: 3600 },
  })
  if (!res.ok) throw new Error(`fetchSets: HTTP ${res.status}`)
  return res.json()
}

export async function fetchSet(tcg: string, code: string, language = 'en'): Promise<TCGSet> {
  const lanParam = language !== 'en' ? `?lan=${encodeURIComponent(language)}` : ''
  const url = `${API}/api/v1/sets/${encodeURIComponent(tcg)}/${encodeURIComponent(code)}${lanParam}`
  const res = await fetch(url, { next: { revalidate: 3600 } })
  if (!res.ok) throw new Error(`fetchSet: HTTP ${res.status}`)
  return res.json()
}

export async function fetchSetCards(
  tcg: string,
  code: string,
  page = 1,
  limit = 60,
  language = 'en',
): Promise<SetCardsResponse> {
  const params = new URLSearchParams({ page: String(page), limit: String(limit) })
  if (language !== 'en') params.set('lan', language)
  const url = `${API}/api/v1/sets/${encodeURIComponent(tcg)}/${encodeURIComponent(code)}/cards?${params}`
  const res = await fetch(url, { next: { revalidate: 3600 } })
  if (!res.ok) throw new Error(`fetchSetCards: HTTP ${res.status}`)
  return res.json()
}

export async function fetchAllSetCards(
  tcg: string,
  code: string,
  language = 'en',
): Promise<CardInSet[]> {
  const first = await fetchSetCards(tcg, code, 1, 200, language)
  const total = first.total
  if (total <= 200) return first.cards
  const remainingPages = Math.ceil((total - 200) / 200)
  const rest = await Promise.all(
    Array.from({ length: remainingPages }, (_, i) =>
      fetchSetCards(tcg, code, i + 2, 200, language),
    ),
  )
  return [...first.cards, ...rest.flatMap(r => r.cards)]
}

export async function fetchCard(code: string, number: string, language = 'en'): Promise<CardDetail> {
  const lanParam = language !== 'en' ? `?lan=${encodeURIComponent(language)}` : ''
  const url = `${API}/api/v1/cards/${encodeURIComponent(code)}/${encodeURIComponent(number)}${lanParam}`
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
