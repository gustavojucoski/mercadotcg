'use client'

import Link from 'next/link'
import { usePathname } from 'next/navigation'
import { UserMenu } from '@/components/UserMenu'
import { useAuth } from '@/components/AuthProvider'

export function SiteHeader() {
  const { user } = useAuth()
  const pathname = usePathname()
  const isAdmin = user?.platform_role === 'platform_admin'

  return (
    <header className="border-b border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
      <div className="mx-auto max-w-6xl px-4 py-3 flex items-center justify-between gap-4">
        <div className="flex items-center gap-6">
          <Link href="/" className="text-base font-bold text-zinc-900 dark:text-zinc-50 shrink-0">
            MercadoTCG
          </Link>

          <nav className="hidden sm:flex items-center gap-1 text-sm">
            <Link
              href="/"
              className={`px-3 py-1.5 rounded-md transition-colors ${
                pathname === '/'
                  ? 'text-zinc-900 dark:text-zinc-50 font-medium'
                  : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100'
              }`}
            >
              Início
            </Link>

            {isAdmin && (
              <>
                <Link
                  href="/admin"
                  className={`px-3 py-1.5 rounded-md transition-colors ${
                    pathname === '/admin'
                      ? 'text-zinc-900 dark:text-zinc-50 font-medium bg-zinc-100 dark:bg-zinc-800'
                      : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-50 dark:hover:bg-zinc-800'
                  }`}
                >
                  Busca Externa
                </Link>
                <Link
                  href="/admin/lojas"
                  className={`px-3 py-1.5 rounded-md transition-colors ${
                    pathname.startsWith('/admin/lojas')
                      ? 'text-zinc-900 dark:text-zinc-50 font-medium bg-zinc-100 dark:bg-zinc-800'
                      : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-50 dark:hover:bg-zinc-800'
                  }`}
                >
                  Lojas
                </Link>
              </>
            )}
          </nav>
        </div>

        <UserMenu />
      </div>
    </header>
  )
}
