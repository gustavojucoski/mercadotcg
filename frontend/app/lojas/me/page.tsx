'use client'

import { useEffect } from 'react'
import { useRouter } from 'next/navigation'
import { getMyStores } from '@/lib/stores-admin'
import { useAuth } from '@/components/AuthProvider'

export default function MyStorePage() {
  const router = useRouter()
  const { user, loading: authLoading } = useAuth()

  useEffect(() => {
    if (authLoading) return
    if (!user) {
      router.replace('/auth/login')
      return
    }
    getMyStores()
      .then(stores => {
        if (stores.length > 0) {
          router.replace(`/lojas/${stores[0].id}/perfil`)
        }
        // If no stores, stays on this page (rendered below)
      })
      .catch(() => {
        // Stays on page showing the empty state
      })
  }, [authLoading, user, router])

  if (authLoading) {
    return (
      <div className="flex items-center justify-center py-24 text-zinc-400 text-sm">
        Carregando...
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-2xl px-4 py-16 text-center">
      <p className="text-zinc-500 text-sm">
        Você ainda não tem lojas cadastradas na plataforma.
      </p>
      <p className="text-zinc-400 text-sm mt-2">
        Entre em contato com o administrador para cadastrar sua loja.
      </p>
    </div>
  )
}
