import { SearchResult } from './types'
import { getAccessToken, refreshAccessToken } from './auth'
import { API_URL } from './config'

// authedFetch injeta o access token e faz retry automático em 401 via refresh.
export async function authedFetch(url: string, init?: RequestInit): Promise<Response> {
  const token = getAccessToken() ?? await refreshAccessToken()
  const headers = new Headers(init?.headers)
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(url, { ...init, headers })

  if (res.status === 401) {
    const newToken = await refreshAccessToken()
    if (newToken) {
      headers.set('Authorization', `Bearer ${newToken}`)
      return fetch(url, { ...init, headers })
    }
  }
  return res
}

export async function searchCard(number: string, set: string): Promise<SearchResult> {
  const url = `${API_URL}/api/v1/external-search?number=${encodeURIComponent(number)}&set=${encodeURIComponent(set)}`
  const res = await authedFetch(url)
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(text || `Erro HTTP ${res.status}`)
  }
  return res.json()
}
