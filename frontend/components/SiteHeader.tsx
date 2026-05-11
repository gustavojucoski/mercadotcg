'use client'

import { useState, useRef, useEffect } from 'react'
import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { UserMenu } from '@/components/UserMenu'
import { useAuth } from '@/components/AuthProvider'
import { GlobalSearch } from '@/components/GlobalSearch'
import { getMyStores } from '@/lib/stores-admin'

const LOJA_TABS = [
  { label: 'Perfil', seg: 'perfil' },
  { label: 'Membros', seg: 'membros' },
  { label: 'Selados', seg: 'selados' },
  { label: 'Singles', seg: 'singles' },
]

export function SiteHeader() {
  const { user } = useAuth()
  const pathname = usePathname()
  const isAdmin = user?.platform_role === 'platform_admin'
  const isLoggedIn = !!user

  const [open, setOpen] = useState<string | null>(null)
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const [hasStores, setHasStores] = useState(false)

  useEffect(() => {
    if (!user) { setHasStores(false); return }
    getMyStores().then(stores => setHasStores(stores.length > 0)).catch(() => setHasStores(false))
  }, [user])

  const openMenu = (name: string) => {
    if (closeTimer.current) clearTimeout(closeTimer.current)
    setOpen(name)
  }
  const scheduleClose = () => {
    closeTimer.current = setTimeout(() => setOpen(null), 120)
  }

  const lojaMatch = pathname.match(/^\/lojas\/([^/]+)/)
  const currentLojaId = lojaMatch?.[1]
  const lojaActive = pathname.startsWith('/lojas/')
  const adminActive = pathname.startsWith('/admin')

  const dropdownBase =
    'absolute top-full left-0 w-52 bg-white dark:bg-zinc-900 rounded-lg shadow-lg border border-zinc-200 dark:border-zinc-800 py-1 z-50'

  const item = (href: string, label: string, active?: boolean) => (
    <Link
      key={href}
      href={href}
      onClick={() => setOpen(null)}
      className={`block px-4 py-2 text-sm transition-colors hover:bg-zinc-50 dark:hover:bg-zinc-800 ${
        active
          ? 'text-violet-600 dark:text-violet-400 font-medium'
          : 'text-zinc-700 dark:text-zinc-300'
      }`}
    >
      {label}
    </Link>
  )

  const triggerClass = (active: boolean) =>
    `flex items-center gap-1 px-3 py-1.5 rounded-md text-sm transition-colors select-none ${
      active
        ? 'text-zinc-900 dark:text-zinc-50 font-medium bg-zinc-100 dark:bg-zinc-800'
        : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-50 dark:hover:bg-zinc-800'
    }`

  const setsActive = pathname.startsWith('/sets') || pathname.startsWith('/cards')

  return (
    <header className="border-b border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
      <div className="mx-auto max-w-6xl px-4 py-3 flex items-center gap-4">
        <div className="flex items-center gap-4 shrink-0">
          <Link href="/" className="text-base font-bold text-zinc-900 dark:text-zinc-50">
            MercadoTCG
          </Link>

          <nav className="hidden sm:flex items-center gap-1 text-sm">
            <Link
              href="/"
              className={`px-3 py-1.5 rounded-md text-sm transition-colors ${
                pathname === '/'
                  ? 'text-zinc-900 dark:text-zinc-50 font-medium bg-zinc-100 dark:bg-zinc-800'
                  : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-50 dark:hover:bg-zinc-800'
              }`}
            >
              Início
            </Link>

            <Link
              href="/sets"
              className={`px-3 py-1.5 rounded-md text-sm transition-colors ${
                setsActive
                  ? 'text-zinc-900 dark:text-zinc-50 font-medium bg-zinc-100 dark:bg-zinc-800'
                  : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-50 dark:hover:bg-zinc-800'
              }`}
            >
              Sets
            </Link>

            {isLoggedIn && hasStores && (
              <div
                className="relative"
                onMouseEnter={() => openMenu('loja')}
                onMouseLeave={scheduleClose}
              >
                <button className={triggerClass(lojaActive)}>
                  Minha Loja <span className="text-xs opacity-60">▾</span>
                </button>
                {open === 'loja' && (
                  <div className={dropdownBase} onMouseEnter={() => openMenu('loja')} onMouseLeave={scheduleClose}>
                    {currentLojaId ? (
                      <>
                        {LOJA_TABS.map(t =>
                          item(
                            `/lojas/${currentLojaId}/${t.seg}`,
                            t.label,
                            pathname === `/lojas/${currentLojaId}/${t.seg}`,
                          )
                        )}
                        <div className="my-1 border-t border-zinc-100 dark:border-zinc-800" />
                        {item('/lojas/me', 'Trocar loja')}
                      </>
                    ) : (
                      item('/lojas/me', 'Ir para minha loja')
                    )}
                  </div>
                )}
              </div>
            )}

            {isAdmin && (
              <div
                className="relative"
                onMouseEnter={() => openMenu('admin')}
                onMouseLeave={scheduleClose}
              >
                <button className={triggerClass(adminActive)}>
                  Admin <span className="text-xs opacity-60">▾</span>
                </button>
                {open === 'admin' && (
                  <div className={dropdownBase} onMouseEnter={() => openMenu('admin')} onMouseLeave={scheduleClose}>
                    {item('/admin', 'Busca Externa', pathname === '/admin')}
                    {item('/admin/lojas', 'Lojas', pathname.startsWith('/admin/lojas'))}
                  </div>
                )}
              </div>
            )}
          </nav>
        </div>

        <div className="flex-1 flex justify-center px-2">
          <GlobalSearch />
        </div>

        <UserMenu />
      </div>
    </header>
  )
}
