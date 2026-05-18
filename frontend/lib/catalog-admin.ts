import { authedFetch } from './api'
import { API_URL } from './config'

// ── Types ────────────────────────────────────────────────────────────────────

export interface CatalogSet {
  id: string
  code: string
  name: string
  name_pt: string
  name_en: string
  series: string
  series_pt: string
  series_id?: string
  tcg: string
  language: string
  release_date?: string
  total_cards: number
  printed_total: number
  image_url: string
  symbol_url: string
  import_source: string
  created_at: string
  updated_at: string
}

export interface CatalogCard {
  id: string
  set_id: string
  collector_number: string
  name: string
  name_pt: string
  rarity: string
  supertype: string
  subtypes: string[]
  types: string[]
  hp: number
  illustrator: string
  image_small_url: string
  image_large_url: string
  image_url_pt: string
  import_source: string
  created_at: string
  updated_at: string
}

export interface CatalogVariant {
  id: string
  card_id: string
  finish: string
  label: string
  is_promo: boolean
  notes: string
  created_at: string
}

export interface SetWithSeries extends CatalogSet {
  series_name?: string
  series_name_pt?: string
}

export interface PaginatedSets {
  items: SetWithSeries[]
  total: number
  page: number
  limit: number
}

export interface ConflictError {
  error: string
  blocked_by: Record<string, number>
}

// ── Sets ─────────────────────────────────────────────────────────────────────

export async function fetchAdminSets(params: {
  tcg: string
  series_id?: string
  q?: string
  page?: number
  limit?: number
}): Promise<PaginatedSets> {
  const qs = new URLSearchParams({ tcg: params.tcg })
  if (params.series_id) qs.set('series_id', params.series_id)
  if (params.q) qs.set('q', params.q)
  if (params.page) qs.set('page', String(params.page))
  if (params.limit) qs.set('limit', String(params.limit))

  const res = await authedFetch(`${API_URL}/api/v1/admin/sets?${qs.toString()}`)
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error((body as { error?: string }).error || `Erro ao listar sets: ${res.status}`)
  }
  return res.json()
}

export async function fetchAdminSet(id: string): Promise<CatalogSet> {
  const res = await authedFetch(`${API_URL}/api/v1/admin/sets/${id}`)
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error((body as { error?: string }).error || `Erro ao buscar set: ${res.status}`)
  }
  return res.json()
}

export async function createAdminSet(body: Partial<CatalogSet>): Promise<CatalogSet> {
  const res = await authedFetch(`${API_URL}/api/v1/admin/sets`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const parsed = await res.json().catch(() => ({}))
    throw new Error((parsed as { error?: string }).error || `Erro ao criar set: ${res.status}`)
  }
  return res.json()
}

export async function patchAdminSet(id: string, body: Partial<CatalogSet>): Promise<CatalogSet> {
  const res = await authedFetch(`${API_URL}/api/v1/admin/sets/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const parsed = await res.json().catch(() => ({}))
    throw new Error((parsed as { error?: string }).error || `Erro ao atualizar set: ${res.status}`)
  }
  return res.json()
}

export async function uploadSetImage(
  id: string,
  file: File,
  slot: 'image' | 'symbol',
): Promise<{ image_url?: string; symbol_url?: string }> {
  const form = new FormData()
  form.append('image', file)
  // Backend has separate endpoints: POST /admin/sets/{id}/image and /symbol
  const endpoint = slot === 'image' ? 'image' : 'symbol'
  const res = await authedFetch(`${API_URL}/api/v1/admin/sets/${id}/${endpoint}`, {
    method: 'POST',
    body: form,
  })
  if (!res.ok) {
    const parsed = await res.json().catch(() => ({}))
    throw new Error((parsed as { error?: string }).error || `Erro ao enviar imagem: ${res.status}`)
  }
  return res.json()
}

export async function deleteAdminSet(id: string, confirmCode: string): Promise<void> {
  const res = await authedFetch(`${API_URL}/api/v1/admin/sets/${id}`, {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ confirm_code: confirmCode }),
  })
  if (res.status === 409) {
    const parsed = await res.json().catch(() => ({}))
    const err = new ConflictDeleteError(
      (parsed as ConflictError).error || 'Conflito ao deletar set',
      (parsed as ConflictError).blocked_by || {},
    )
    throw err
  }
  if (!res.ok) {
    const parsed = await res.json().catch(() => ({}))
    throw new Error((parsed as { error?: string }).error || `Erro ao deletar set: ${res.status}`)
  }
}

// ── Cards ────────────────────────────────────────────────────────────────────

export async function fetchAdminCard(id: string): Promise<CatalogCard> {
  const res = await authedFetch(`${API_URL}/api/v1/admin/cards/${id}`)
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error((body as { error?: string }).error || `Erro ao buscar carta: ${res.status}`)
  }
  return res.json()
}

export async function createAdminCard(
  body: Partial<CatalogCard> & { set_id: string },
): Promise<CatalogCard> {
  const res = await authedFetch(`${API_URL}/api/v1/admin/cards`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const parsed = await res.json().catch(() => ({}))
    throw new Error((parsed as { error?: string }).error || `Erro ao criar carta: ${res.status}`)
  }
  return res.json()
}

export async function patchAdminCard(
  id: string,
  body: Partial<CatalogCard>,
): Promise<CatalogCard> {
  const res = await authedFetch(`${API_URL}/api/v1/admin/cards/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const parsed = await res.json().catch(() => ({}))
    throw new Error((parsed as { error?: string }).error || `Erro ao atualizar carta: ${res.status}`)
  }
  return res.json()
}

export async function uploadCardImage(
  id: string,
  file: File,
  slot: 'en' | 'pt',
): Promise<{ image_small_url?: string; image_large_url?: string; image_url_pt?: string }> {
  const form = new FormData()
  form.append('image', file)
  // Backend: POST /admin/cards/{id}/image (EN) and /admin/cards/{id}/image-pt (PT)
  const endpoint = slot === 'pt' ? 'image-pt' : 'image'
  const res = await authedFetch(`${API_URL}/api/v1/admin/cards/${id}/${endpoint}`, {
    method: 'POST',
    body: form,
  })
  if (!res.ok) {
    const parsed = await res.json().catch(() => ({}))
    throw new Error((parsed as { error?: string }).error || `Erro ao enviar imagem: ${res.status}`)
  }
  return res.json()
}

export async function deleteAdminCard(
  id: string,
  confirmCollectorNumber: string,
): Promise<void> {
  const res = await authedFetch(`${API_URL}/api/v1/admin/cards/${id}`, {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ confirm_collector_number: confirmCollectorNumber }),
  })
  if (res.status === 409) {
    const parsed = await res.json().catch(() => ({}))
    throw new ConflictDeleteError(
      (parsed as ConflictError).error || 'Conflito ao deletar carta',
      (parsed as ConflictError).blocked_by || {},
    )
  }
  if (!res.ok) {
    const parsed = await res.json().catch(() => ({}))
    throw new Error((parsed as { error?: string }).error || `Erro ao deletar carta: ${res.status}`)
  }
}

// ── Variants ─────────────────────────────────────────────────────────────────

export async function fetchCardVariants(cardId: string): Promise<CatalogVariant[]> {
  const res = await authedFetch(
    `${API_URL}/api/v1/admin/cards/${cardId}/variants`,
  )
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error((body as { error?: string }).error || `Erro ao buscar variantes: ${res.status}`)
  }
  return res.json()
}

export async function createAdminVariant(
  cardId: string,
  body: { finish: string; label?: string; is_promo: boolean; notes?: string },
): Promise<CatalogVariant> {
  const res = await authedFetch(
    `${API_URL}/api/v1/admin/cards/${cardId}/variants`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
  )
  if (!res.ok) {
    const parsed = await res.json().catch(() => ({}))
    throw new Error(
      (parsed as { error?: string }).error || `Erro ao criar variante: ${res.status}`,
    )
  }
  return res.json()
}

export async function patchAdminVariant(
  id: string,
  body: Partial<CatalogVariant>,
): Promise<CatalogVariant> {
  const res = await authedFetch(`${API_URL}/api/v1/admin/variants/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const parsed = await res.json().catch(() => ({}))
    throw new Error(
      (parsed as { error?: string }).error || `Erro ao atualizar variante: ${res.status}`,
    )
  }
  return res.json()
}

export async function deleteAdminVariant(id: string): Promise<void> {
  const res = await authedFetch(`${API_URL}/api/v1/admin/variants/${id}`, {
    method: 'DELETE',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ confirm: true }),
  })
  if (res.status === 409) {
    const parsed = await res.json().catch(() => ({}))
    throw new ConflictDeleteError(
      (parsed as ConflictError).error || 'Conflito ao deletar variante',
      (parsed as ConflictError).blocked_by || {},
    )
  }
  if (!res.ok) {
    const parsed = await res.json().catch(() => ({}))
    throw new Error(
      (parsed as { error?: string }).error || `Erro ao deletar variante: ${res.status}`,
    )
  }
}

// ── ConflictDeleteError ───────────────────────────────────────────────────────

export class ConflictDeleteError extends Error {
  blockedBy: Record<string, number>
  constructor(message: string, blockedBy: Record<string, number>) {
    super(message)
    this.name = 'ConflictDeleteError'
    this.blockedBy = blockedBy
  }
}
