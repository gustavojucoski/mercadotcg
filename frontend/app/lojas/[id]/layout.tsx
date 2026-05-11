'use client'

import { useEffect, useState } from 'react'
import { useParams, usePathname, useRouter } from 'next/navigation'
import Link from 'next/link'
import { getStorePublic, getMyRole, AdminStore } from '@/lib/stores-admin'
import { useAuth } from '@/components/AuthProvider'

const tabs = [
  { label: 'Perfil', seg: 'perfil' },
  { label: 'Membros', seg: 'membros' },
  { label: 'Selados', seg: 'selados' },
  { label: 'Singles', seg: 'singles' },
]

const ROLE_LABELS: Record<string, string> = {
  admin: 'Administrador',
  stock_manager: 'Gestor de estoque',
  viewer: 'Visualizador',
}

export default function StoreLayout({ children }: { children: React.ReactNode }) {
  const { id } = useParams<{ id: string }>()
  const pathname = usePathname()
  const router = useRouter()
  const { user, loading: authLoading } = useAuth()

  const [store, setStore] = useState<AdminStore | null>(null)
  const [role, setRole] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [denied, setDenied] = useState(false)

  useEffect(() => {
    if (authLoading) return
    if (!user) {
      router.replace('/auth/login')
      return
    }
    Promise.all([getStorePublic(id), getMyRole(id)])
      .then(([s, r]) => {
        setStore(s)
        if (!r) {
          setDenied(true)
        } else {
          setRole(r)
        }
      })
      .catch(() => setDenied(true))
      .finally(() => setLoading(false))
  }, [authLoading, user, id, router])

  if (authLoading || loading) {
    return (
      <div className="flex items-center justify-center py-24 text-zinc-400 text-sm">
        Carregando...
      </div>
    )
  }

  if (denied) {
    return (
      <div className="mx-auto max-w-2xl px-4 py-16 text-center space-y-4">
        <p className="text-zinc-500 text-sm">Você não tem acesso a esta loja.</p>
        <Link href="/" className="text-sm text-violet-600 hover:underline">
          ← Voltar ao início
        </Link>
      </div>
    )
  }

  return (
    <div className="min-h-screen">
      <div className="border-b border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900">
        <div className="mx-auto max-w-6xl px-4">
          <div className="flex items-center gap-3 py-3">
            <Link
              href="/"
              className="text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200 shrink-0 text-lg leading-none"
              title="Início"
            >
              ←
            </Link>

            {store?.logo_url && (
              <img
                src={store.logo_url}
                alt={store.name}
                className="w-8 h-8 rounded-lg object-cover shrink-0"
              />
            )}
            <div className="min-w-0">
              <p className="text-sm font-semibold text-zinc-900 dark:text-zinc-50 truncate">
                {store?.name}
              </p>
              {store?.trade_name && (
                <p className="text-xs text-zinc-400 truncate">{store.trade_name}</p>
              )}
            </div>
            <span className="ml-auto text-xs text-zinc-400 capitalize shrink-0">
              {role ? (ROLE_LABELS[role] ?? role) : ''}
            </span>
          </div>

          <nav className="flex gap-1 -mb-px">
            {tabs.map(tab => {
              const href = `/lojas/${id}/${tab.seg}`
              const active = pathname === href
              return (
                <Link
                  key={href}
                  href={href}
                  className={`px-4 py-2.5 text-sm font-medium border-b-2 transition-colors ${
                    active
                      ? 'border-violet-600 text-violet-600 dark:text-violet-400 dark:border-violet-400'
                      : 'border-transparent text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300'
                  }`}
                >
                  {tab.label}
                </Link>
              )
            })}
          </nav>
        </div>
      </div>

      <div>{children}</div>
    </div>
  )
}
