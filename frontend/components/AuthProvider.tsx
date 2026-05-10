'use client'

import { createContext, useContext, useEffect, useState, ReactNode } from 'react'
import { AuthUser, fetchCurrentUser, getRefreshToken, setAccessToken, setRefreshToken } from '@/lib/auth'

interface AuthContextValue {
  user: AuthUser | null
  loading: boolean
  refresh: () => Promise<void>
  clearAuth: () => void
}

const AuthContext = createContext<AuthContextValue>({
  user: null,
  loading: true,
  refresh: async () => {},
  clearAuth: () => {},
})

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null)
  const [loading, setLoading] = useState(true)

  const loadUser = async () => {
    if (getRefreshToken()) {
      const u = await fetchCurrentUser()
      setUser(u)
    }
    setLoading(false)
  }

  const clearAuth = () => {
    setAccessToken(null)
    setRefreshToken(null)
    setUser(null)
  }

  useEffect(() => {
    loadUser()
  }, [])

  return (
    <AuthContext.Provider value={{ user, loading, refresh: loadUser, clearAuth }}>
      {children}
    </AuthContext.Provider>
  )
}

export const useAuth = () => useContext(AuthContext)
