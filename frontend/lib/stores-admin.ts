import { authedFetch } from './api'

const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

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
  document_type?: 'cpf' | 'cnpj'
  document_number?: string
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

export async function lookupCNPJ(cnpj: string): Promise<{ legal_name: string; situation: string }> {
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
