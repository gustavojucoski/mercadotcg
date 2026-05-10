'use client'

import { Suspense, useEffect } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { setAccessToken, setRefreshToken } from '@/lib/auth'
import { useAuth } from '@/components/AuthProvider'

function CallbackInner() {
  const searchParams = useSearchParams()
  const router = useRouter()
  const { refresh } = useAuth()

  useEffect(() => {
    const accessToken = searchParams.get('access_token')
    const refreshToken = searchParams.get('refresh_token')

    if (accessToken && refreshToken) {
      setAccessToken(accessToken)
      setRefreshToken(refreshToken)
      refresh().then(() => {
        router.replace('/')
      })
    } else {
      router.replace('/auth/login')
    }
  }, [searchParams, router, refresh])

  return (
    <div className="text-center">
      <div className="text-4xl mb-4">⏳</div>
      <p className="text-sm text-zinc-500">Autenticando...</p>
    </div>
  )
}

export default function AuthCallbackPage() {
  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950 flex items-center justify-center">
      <Suspense fallback={<div className="text-sm text-zinc-400">Carregando...</div>}>
        <CallbackInner />
      </Suspense>
    </div>
  )
}
