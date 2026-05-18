import { authedFetch } from './api'
import { API_URL } from './config'

export interface AdminStore {
  id: string
  owner_id: string
  name: string
  slug: string
  description?: string
  logo_url?: string
  is_active: boolean
  document_type?: 'cpf' | 'cnpj'
  document_number?: string
  document_status: 'pending' | 'auto_verified' | 'manually_verified'
  legal_name?: string
  trade_name?: string
  phone?: string
  address_zip?: string
  address_street?: string
  address_number?: string
  address_complement?: string
  address_neighborhood?: string
  address_city?: string
  address_state?: string
  address_country?: string
  document_verified_at?: string
  document_verified_by?: string
  created_at: string
  updated_at: string
}

export interface CreateStoreInput {
  owner_id: string
  name: string
  slug: string
  description?: string
  document_type: 'cpf' | 'cnpj'
  document_number: string
  trade_name?: string
  phone?: string
  address_zip?: string
  address_street?: string
  address_number?: string
  address_complement?: string
  address_neighborhood?: string
  address_city?: string
  address_state?: string
  address_country?: string
}

export interface CNPJLookupResult {
  legal_name: string
  trade_name: string
  situation: string
  phone: string
  address_zip: string
  address_street: string
  address_number: string
  address_complement: string
  address_neighborhood: string
  address_city: string
  address_state: string
}

export interface UserSummary {
  id: string
  email: string
  display_name: string
}

export async function listStores(limit = 50, offset = 0): Promise<AdminStore[]> {
  const res = await authedFetch(
    `${API_URL}/api/v1/admin/stores?limit=${limit}&offset=${offset}`
  )
  if (!res.ok) throw new Error(`Erro ao listar lojas: ${res.status}`)
  return res.json()
}

export async function createStore(input: CreateStoreInput): Promise<AdminStore> {
  const res = await authedFetch(`${API_URL}/api/v1/admin/stores`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Erro ao criar loja: ${res.status}`)
  }
  return res.json()
}

export async function lookupCNPJ(cnpj: string): Promise<CNPJLookupResult> {
  const res = await authedFetch(
    `${API_URL}/api/v1/admin/stores/cnpj-lookup?cnpj=${encodeURIComponent(cnpj)}`
  )
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Erro na consulta: ${res.status}`)
  }
  return res.json()
}

export async function verifyDocument(storeId: string): Promise<AdminStore> {
  const res = await authedFetch(
    `${API_URL}/api/v1/admin/stores/${storeId}/verify-document`,
    { method: 'POST' }
  )
  if (!res.ok) throw new Error(`Erro ao verificar documento: ${res.status}`)
  return res.json()
}

export async function searchUsers(q: string): Promise<UserSummary[]> {
  const res = await authedFetch(
    `${API_URL}/api/v1/admin/users/search?q=${encodeURIComponent(q)}`
  )
  if (!res.ok) throw new Error(`Erro ao buscar usuários: ${res.status}`)
  return res.json()
}

export async function uploadStoreLogo(storeId: string, file: File): Promise<AdminStore> {
  const form = new FormData()
  form.append('logo', file)
  const res = await authedFetch(`${API_URL}/api/v1/admin/stores/${storeId}/logo`, {
    method: 'POST',
    body: form,
    // Não definir Content-Type — o browser precisa setar o boundary do multipart
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Erro ao enviar logo: ${res.status}`)
  }
  return res.json()
}

export async function getStore(id: string): Promise<AdminStore> {
  const res = await authedFetch(`${API_URL}/api/v1/admin/stores/${id}`)
  if (!res.ok) throw new Error(`Erro ao buscar loja: ${res.status}`)
  return res.json()
}

export interface UpdateStoreInput {
  owner_id?: string
  name?: string
  slug?: string
  description?: string
  is_active?: boolean
  legal_name?: string | null
  trade_name?: string
  phone?: string
  address_zip?: string
  address_street?: string
  address_number?: string
  address_complement?: string
  address_neighborhood?: string
  address_city?: string
  address_state?: string
}

export async function updateStore(id: string, data: UpdateStoreInput): Promise<AdminStore> {
  const res = await authedFetch(`${API_URL}/api/v1/admin/stores/${id}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Erro ao atualizar loja: ${res.status}`)
  }
  return res.json()
}

export interface AuditFieldChange {
  old: unknown
  new: unknown
}

export interface AuditEntry {
  id: string
  store_id: string
  changed_by: string
  changed_by_name: string
  changed_by_email: string
  change_type: string
  changes: Record<string, AuditFieldChange>
  created_at: string
}

export async function getAuditLog(id: string, limit = 50, offset = 0): Promise<AuditEntry[]> {
  const res = await authedFetch(
    `${API_URL}/api/v1/admin/stores/${id}/audit-log?limit=${limit}&offset=${offset}`
  )
  if (!res.ok) throw new Error(`Erro ao buscar log: ${res.status}`)
  return res.json()
}

// ---- Store self-service (non-admin) ----------------------------------------

const STORE_API = `${API_URL}/api/v1/stores`

// getStorePublic fetches a store via the public endpoint (no platform_admin required).
export async function getStorePublic(id: string): Promise<AdminStore> {
  const res = await authedFetch(`${STORE_API}/${id}`)
  if (!res.ok) throw new Error(`Loja não encontrada: ${res.status}`)
  return res.json()
}

export async function getMyStores(): Promise<AdminStore[]> {
  const res = await authedFetch(`${STORE_API}/me`)
  if (!res.ok) throw new Error(`Erro ao buscar suas lojas: ${res.status}`)
  return res.json()
}

export interface UpdateProfileInput {
  name?: string
  description?: string
  trade_name?: string
  phone?: string
  address_zip?: string
  address_street?: string
  address_number?: string
  address_complement?: string
  address_neighborhood?: string
  address_city?: string
  address_state?: string
}

export async function updateStoreProfile(id: string, data: UpdateProfileInput): Promise<AdminStore> {
  const res = await authedFetch(`${STORE_API}/${id}/profile`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Erro ao salvar perfil: ${res.status}`)
  }
  return res.json()
}

export async function uploadStoreLogoSelf(storeId: string, file: File): Promise<AdminStore> {
  const form = new FormData()
  form.append('logo', file)
  const res = await authedFetch(`${STORE_API}/${storeId}/logo`, {
    method: 'POST',
    body: form,
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Erro ao enviar logo: ${res.status}`)
  }
  return res.json()
}

export async function getMyRole(storeId: string): Promise<string | null> {
  const res = await authedFetch(`${STORE_API}/${storeId}/my-role`)
  if (!res.ok) return null
  const data = await res.json()
  return data.role ?? null
}

export interface StoreMemberRow {
  id: string
  store_id: string
  user_id: string
  role: 'admin' | 'stock_manager' | 'viewer'
  invited_by?: string
  joined_at: string
  user_email: string
  user_display_name: string
}

export async function listStoreMembers(storeId: string): Promise<StoreMemberRow[]> {
  const res = await authedFetch(`${STORE_API}/${storeId}/members`)
  if (!res.ok) throw new Error(`Erro ao listar membros: ${res.status}`)
  return res.json()
}

export async function addStoreMember(storeId: string, email: string, role: string): Promise<void> {
  const res = await authedFetch(`${STORE_API}/${storeId}/members`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, role }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Erro ao adicionar membro: ${res.status}`)
  }
}

export async function removeStoreMember(storeId: string, userId: string): Promise<void> {
  const res = await authedFetch(`${STORE_API}/${storeId}/members/${userId}`, { method: 'DELETE' })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Erro ao remover membro: ${res.status}`)
  }
}

export async function updateStoreMemberRole(storeId: string, userId: string, role: string): Promise<void> {
  const res = await authedFetch(`${STORE_API}/${storeId}/members/${userId}/role`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ role }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Erro ao atualizar role: ${res.status}`)
  }
}

// ---- Stock ----------------------------------------------------------------

export interface PerSourceSignal {
  source: string
  sales_count: number
  weighted_avg_brl?: string
  min_brl?: string
  max_brl?: string
  last_sale_day?: string
}

export interface StockSignal {
  variant_id: string
  condition: string
  window_days: number
  sources: PerSourceSignal[]
}

export interface StockItem {
  id: string
  store_id: string
  variant_id: string
  condition: string
  language: string
  grade?: string
  quantity: number
  cost_avg_brl?: string
  asking_price_brl?: string
  sku?: string
  notes?: string
  created_at: string
  updated_at: string
  // enrichment fields — added by backend; optional for backwards compat
  card_name?: string
  card_number?: string
  set_name?: string
  set_code?: string
  finish?: string
  variant_label?: string
  image_small_url?: string
}

export interface StockItemWithSignal extends StockItem {
  signal?: StockSignal
}

export interface PurchaseInput {
  variant_id: string
  condition: string
  language: string
  grade?: string
  quantity: number
  unit_cost_brl: string
  notes?: string
}

export async function listStoreStock(
  storeId: string,
  opts: { inStock?: boolean; withSignal?: boolean; limit?: number; offset?: number } = {}
): Promise<StockItemWithSignal[]> {
  const params = new URLSearchParams()
  if (opts.inStock) params.set('in_stock', 'true')
  if (opts.withSignal) params.set('with_signal', 'true')
  if (opts.limit !== undefined) params.set('limit', String(opts.limit))
  if (opts.offset !== undefined) params.set('offset', String(opts.offset))
  const qs = params.toString()
  const res = await authedFetch(`${STORE_API}/${storeId}/stock${qs ? '?' + qs : ''}`)
  if (!res.ok) throw new Error(`Erro ao listar estoque: ${res.status}`)
  return res.json()
}

export async function registerPurchase(storeId: string, input: PurchaseInput): Promise<StockItem> {
  const res = await authedFetch(`${STORE_API}/${storeId}/stock/purchase`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Erro ao registrar compra: ${res.status}`)
  }
  return res.json()
}

export interface SaleInput {
  quantity: number
  unit_price_brl: string
  occurred_at?: string
  notes?: string
}

export async function registerSale(
  storeId: string,
  stockItemId: string,
  input: SaleInput
): Promise<StockItem> {
  const res = await authedFetch(
    `${STORE_API}/${storeId}/stock-items/${stockItemId}/sale`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(input),
    }
  )
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Erro ao registrar venda: ${res.status}`)
  }
  return res.json()
}

// ---- Card lookup (for variant picker) ------------------------------------

export interface CardVariant {
  id: string
  card_id: string
  finish: string
  label?: string
  is_promo: boolean
  notes?: string
}

export interface CardResult {
  id: string
  set_id: string
  number: string
  name: string
  rarity?: string
  image_small_url?: string
}

export interface CardSetResult {
  id: string
  code: string
  name: string
  series?: string
}

export interface CardLookupMatch {
  card: CardResult
  set: CardSetResult
  variants: Array<{ variant: CardVariant }>
}

export interface CardLookupResponse {
  query: { name?: string; number?: string; set?: string }
  matches: CardLookupMatch[]
}

export async function lookupCards(q: string, limit = 10): Promise<CardLookupResponse> {
  const params = new URLSearchParams({ name: q, limit: String(limit) })
  const res = await authedFetch(
    `${API_URL}/api/v1/cards/lookup?${params.toString()}`
  )
  if (!res.ok) throw new Error(`Erro ao buscar cartas: ${res.status}`)
  return res.json()
}
