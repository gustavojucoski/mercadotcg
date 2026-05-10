import { SearchResult } from './types'

const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

export async function searchCard(number: string, set: string): Promise<SearchResult> {
  const url = `${API_URL}/api/v1/external-search?number=${encodeURIComponent(number)}&set=${encodeURIComponent(set)}`
  const res = await fetch(url)
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new Error(text || `Erro HTTP ${res.status}`)
  }
  return res.json()
}
