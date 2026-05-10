'use client'

import { useEffect, useRef, useState } from 'react'
import Link from 'next/link'
import { useRouter } from 'next/navigation'
import { logout } from '@/lib/auth'
import { useAuth } from '@/components/AuthProvider'

export function UserMenu() {
  const { user, loading, clearAuth } = useAuth()
  const router = useRouter()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    function onClickOutside(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onClickOutside)
    return () => document.removeEventListener('mousedown', onClickOutside)
  }, [])

  if (loading) {
    return <div className="h-8 w-24 rounded-lg bg-zinc-100 dark:bg-zinc-800 animate-pulse" />
  }

  if (!user) {
    return (
      <div className="relative" ref={ref}>
        <button
          onClick={() => setOpen(v => !v)}
          className="flex items-center gap-1.5 rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-1.5 text-sm font-medium text-zinc-700 dark:text-zinc-200 hover:bg-zinc-50 dark:hover:bg-zinc-700 transition-colors"
        >
          <svg className="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 6a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0zM4.501 20.118a7.5 7.5 0 0114.998 0A17.933 17.933 0 0112 21.75c-2.676 0-5.216-.584-7.499-1.632z" />
          </svg>
          Entrar
          <svg className="size-3 text-zinc-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
            <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
          </svg>
        </button>
        {open && (
          <div className="absolute right-0 mt-1.5 w-44 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-800 shadow-lg py-1 z-50">
            <Link
              href="/auth/login"
              onClick={() => setOpen(false)}
              className="flex items-center px-4 py-2.5 text-sm text-zinc-700 dark:text-zinc-200 hover:bg-zinc-50 dark:hover:bg-zinc-700"
            >
              Fazer login
            </Link>
            <Link
              href="/auth/register"
              onClick={() => setOpen(false)}
              className="flex items-center px-4 py-2.5 text-sm text-zinc-700 dark:text-zinc-200 hover:bg-zinc-50 dark:hover:bg-zinc-700"
            >
              Criar conta
            </Link>
          </div>
        )}
      </div>
    )
  }

  const initials = user.display_name
    ? user.display_name.split(' ').map(n => n[0]).slice(0, 2).join('').toUpperCase()
    : user.email[0].toUpperCase()

  return (
    <div className="relative" ref={ref}>
      <button
        onClick={() => setOpen(v => !v)}
        className="flex items-center gap-2 rounded-lg px-2.5 py-1.5 text-sm hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors"
      >
        <div className="size-7 rounded-full bg-violet-600 flex items-center justify-center text-white text-xs font-bold select-none">
          {initials}
        </div>
        <span className="max-w-[140px] truncate font-medium text-zinc-700 dark:text-zinc-200">
          {user.display_name || user.email}
        </span>
        <svg className="size-3 text-zinc-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2.5}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
        </svg>
      </button>

      {open && (
        <div className="absolute right-0 mt-1.5 w-56 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-800 shadow-lg py-1 z-50">
          <div className="px-4 py-2.5 border-b border-zinc-100 dark:border-zinc-700">
            <p className="text-xs font-medium text-zinc-900 dark:text-zinc-100 truncate">{user.display_name}</p>
            <p className="text-xs text-zinc-400 truncate mt-0.5">{user.email}</p>
          </div>

          <div className="py-1">
            <button
              disabled
              className="w-full flex items-center justify-between px-4 py-2.5 text-sm text-zinc-400 cursor-not-allowed"
            >
              Minha conta
              <span className="text-xs bg-zinc-100 dark:bg-zinc-700 px-1.5 py-0.5 rounded text-zinc-400">em breve</span>
            </button>
          </div>

          <div className="border-t border-zinc-100 dark:border-zinc-700 py-1">
            <button
              onClick={async () => {
                setOpen(false)
                await logout()
                clearAuth()
                router.push('/')
              }}
              className="w-full flex items-center px-4 py-2.5 text-sm text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-950/30 transition-colors"
            >
              Sair
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
