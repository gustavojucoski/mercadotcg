'use client'

import { Suspense, useEffect, useState } from 'react'
import { useRouter } from 'next/navigation'
import { useSearchParams } from 'next/navigation'
import Link from 'next/link'
import { verifyEmailWithSetup } from '@/lib/auth'

function VerifyEmailInner() {
  const searchParams = useSearchParams()
  const router = useRouter()
  const token = searchParams.get('token')

  const [phase, setPhase] = useState<'form' | 'success'>('form')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  // Redirect after success
  useEffect(() => {
    if (phase !== 'success') return
    const timer = setTimeout(() => {
      router.replace('/')
    }, 2000)
    return () => clearTimeout(timer)
  }, [phase, router])

  if (!token) {
    return (
      <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950 flex items-center justify-center p-4">
        <div className="w-full max-w-sm">
          <div className="bg-white dark:bg-zinc-900 rounded-xl border border-zinc-200 dark:border-zinc-800 p-8 shadow-sm text-center">
            <div className="text-4xl mb-4">❌</div>
            <h2 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50 mb-2">Link inválido</h2>
            <p className="text-sm text-zinc-500 mb-6">Token de verificação ausente. Use o link enviado para o seu email.</p>
            <Link
              href="/auth/login"
              className="inline-block text-sm text-violet-600 hover:underline"
            >
              Voltar para o login
            </Link>
          </div>
        </div>
      </div>
    )
  }

  if (phase === 'success') {
    return (
      <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950 flex items-center justify-center p-4">
        <div className="w-full max-w-sm">
          <div className="bg-white dark:bg-zinc-900 rounded-xl border border-zinc-200 dark:border-zinc-800 p-8 shadow-sm text-center">
            <div className="text-4xl mb-4">✅</div>
            <h2 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50 mb-2">Conta ativada!</h2>
            <p className="text-sm text-zinc-500">Redirecionando...</p>
          </div>
        </div>
      </div>
    )
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)

    if (password.length < 8) {
      setError('A senha deve ter no mínimo 8 caracteres.')
      return
    }
    if (password !== confirmPassword) {
      setError('As senhas não coincidem.')
      return
    }

    setLoading(true)
    try {
      const tokens = await verifyEmailWithSetup(token!, password, displayName)
      // Redirect based on role; AuthProvider hydrates on next render via the stored tokens
      if (tokens.user.platform_role === 'platform_admin') {
        router.replace('/admin')
      } else {
        setPhase('success')
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Erro ao ativar conta')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950 flex items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="text-center mb-8">
          <h1 className="text-2xl font-bold text-zinc-900 dark:text-zinc-50">MercadoTCG</h1>
          <p className="text-sm text-zinc-500 mt-1">Defina seu nome e senha para ativar a conta</p>
        </div>

        <div className="bg-white dark:bg-zinc-900 rounded-xl border border-zinc-200 dark:border-zinc-800 p-6 shadow-sm">
          <h2 className="text-base font-semibold text-zinc-900 dark:text-zinc-50 mb-4">Complete seu cadastro</h2>

          <form onSubmit={handleSubmit} className="flex flex-col gap-4">
            {error && (
              <div className="rounded-lg bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900/50 px-4 py-3 text-sm text-red-700 dark:text-red-400">
                {error}
              </div>
            )}

            <div className="flex flex-col gap-1.5">
              <label className="text-sm font-medium text-zinc-700 dark:text-zinc-300" htmlFor="display_name">
                Nome
              </label>
              <input
                id="display_name"
                type="text"
                value={displayName}
                onChange={e => setDisplayName(e.target.value)}
                required
                autoFocus
                autoComplete="name"
                className="rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder:text-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500"
                placeholder="Seu nome"
              />
            </div>

            <div className="flex flex-col gap-1.5">
              <label className="text-sm font-medium text-zinc-700 dark:text-zinc-300" htmlFor="password">
                Senha
              </label>
              <input
                id="password"
                type="password"
                value={password}
                onChange={e => setPassword(e.target.value)}
                required
                minLength={8}
                autoComplete="new-password"
                className="rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder:text-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500"
                placeholder="Mínimo 8 caracteres"
              />
            </div>

            <div className="flex flex-col gap-1.5">
              <label className="text-sm font-medium text-zinc-700 dark:text-zinc-300" htmlFor="confirm_password">
                Confirmar senha
              </label>
              <input
                id="confirm_password"
                type="password"
                value={confirmPassword}
                onChange={e => setConfirmPassword(e.target.value)}
                required
                autoComplete="new-password"
                className="rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder:text-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500"
                placeholder="Repita a senha"
              />
            </div>

            <button
              type="submit"
              disabled={loading}
              className="rounded-lg bg-violet-600 px-4 py-2.5 text-sm font-semibold text-white hover:bg-violet-700 disabled:opacity-50 transition-colors"
            >
              {loading ? 'Ativando...' : 'Ativar conta'}
            </button>
          </form>
        </div>

        <p className="mt-4 text-center text-sm text-zinc-500">
          <Link href="/auth/login" className="text-violet-600 hover:underline">
            Voltar para o login
          </Link>
        </p>
      </div>
    </div>
  )
}

export default function VerifyEmailPage() {
  return (
    <Suspense fallback={
      <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950 flex items-center justify-center">
        <div className="text-sm text-zinc-400">Carregando...</div>
      </div>
    }>
      <VerifyEmailInner />
    </Suspense>
  )
}
