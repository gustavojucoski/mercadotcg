// auth.ts — utilitários de autenticação no cliente.
// Access token: variável de módulo (perdida no refresh de página, re-obtida via refresh token).
// Refresh token: localStorage (cross-origin com Go backend na porta 8080; segurança real
// é enforced pelo middleware RequirePlatformAdmin no backend).

const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

export interface AuthUser {
  id: string
  email: string
  display_name: string
  avatar_url?: string
  platform_role: 'platform_admin' | 'user'
  is_active: boolean
}

export interface AuthTokens {
  access_token: string
  refresh_token: string
  expires_in: number
  user: AuthUser
}

let _accessToken: string | null = null

export function getAccessToken(): string | null {
  return _accessToken
}

export function setAccessToken(token: string | null): void {
  _accessToken = token
}

export function getRefreshToken(): string | null {
  if (typeof window === 'undefined') return null
  return localStorage.getItem('mtcg_rt')
}

export function setRefreshToken(token: string | null): void {
  if (typeof window === 'undefined') return
  if (token) {
    localStorage.setItem('mtcg_rt', token)
  } else {
    localStorage.removeItem('mtcg_rt')
  }
}

export async function login(email: string, password: string): Promise<AuthTokens> {
  const res = await fetch(`${API_URL}/api/v1/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Erro ${res.status}`)
  }
  const tokens: AuthTokens = await res.json()
  setAccessToken(tokens.access_token)
  setRefreshToken(tokens.refresh_token)
  return tokens
}

export async function register(email: string): Promise<void> {
  const res = await fetch(`${API_URL}/api/v1/auth/register`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `Erro ${res.status}`)
  }
}

export async function verifyEmailWithSetup(
  token: string,
  password: string,
  displayName: string
): Promise<AuthTokens> {
  const res = await fetch(`${API_URL}/api/v1/auth/verify-email`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token, password, display_name: displayName }),
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || 'Erro ao verificar email')
  }
  const tokens: AuthTokens = await res.json()
  setAccessToken(tokens.access_token)
  setRefreshToken(tokens.refresh_token)
  return tokens
}

export async function logout(): Promise<void> {
  const rt = getRefreshToken()
  if (rt) {
    await fetch(`${API_URL}/api/v1/auth/logout`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: rt }),
    }).catch(() => {})
  }
  setAccessToken(null)
  setRefreshToken(null)
}

export async function refreshAccessToken(): Promise<string | null> {
  const rt = getRefreshToken()
  if (!rt) return null
  try {
    const res = await fetch(`${API_URL}/api/v1/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refresh_token: rt }),
    })
    if (!res.ok) {
      setRefreshToken(null)
      return null
    }
    const data = await res.json()
    setAccessToken(data.access_token)
    return data.access_token as string
  } catch {
    return null
  }
}

export async function fetchCurrentUser(): Promise<AuthUser | null> {
  const token = getAccessToken() ?? await refreshAccessToken()
  if (!token) return null
  try {
    const res = await fetch(`${API_URL}/api/v1/auth/me`, {
      headers: { Authorization: `Bearer ${token}` },
    })
    if (!res.ok) return null
    return res.json()
  } catch {
    return null
  }
}
