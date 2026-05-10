'use client'

import { Suspense, useEffect, useState } from 'react'
import { useSearchParams } from 'next/navigation'
import Link from 'next/link'

const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

function VerifyEmailInner() {
  const searchParams = useSearchParams()
  const token = searchParams.get('token')
  const [status, setStatus] = useState<'loading' | 'success' | 'error'>('loading')
  const [message, setMessage] = useState('')

  useEffect(() => {
    if (!token) {
      setStatus('error')
      setMessage('Token de verificação ausente.')
      return
    }
    fetch(`${API_URL}/api/v1/auth/verify-email`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token }),
    })
      .then(async res => {
        if (res.ok) {
          setStatus('success')
        } else {
          const body = await res.json().catch(() => ({}))
          setStatus('error')
          setMessage(body.error || 'Token inválido ou expirado.')
        }
      })
      .catch(() => {
        setStatus('error')
        setMessage('Erro ao conectar com o servidor.')
      })
  }, [token])

  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950 flex items-center justify-center p-4">
      <div className="w-full max-w-sm">
        <div className="bg-white dark:bg-zinc-900 rounded-xl border border-zinc-200 dark:border-zinc-800 p-8 shadow-sm text-center">
          {status === 'loading' && (
            <>
              <div className="text-4xl mb-4">⏳</div>
              <h2 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50">Verificando...</h2>
            </>
          )}
          {status === 'success' && (
            <>
              <div className="text-4xl mb-4">✅</div>
              <h2 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50 mb-2">Email verificado!</h2>
              <p className="text-sm text-zinc-500 mb-6">Sua conta foi ativada com sucesso.</p>
              <Link
                href="/auth/login"
                className="inline-block rounded-lg bg-violet-600 px-4 py-2.5 text-sm font-semibold text-white hover:bg-violet-700 transition-colors"
              >
                Fazer login
              </Link>
            </>
          )}
          {status === 'error' && (
            <>
              <div className="text-4xl mb-4">❌</div>
              <h2 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50 mb-2">Erro na verificação</h2>
              <p className="text-sm text-zinc-500 mb-6">{message}</p>
              <Link
                href="/auth/login"
                className="inline-block text-sm text-violet-600 hover:underline"
              >
                Voltar para o login
              </Link>
            </>
          )}
        </div>
      </div>
    </div>
  )
}

export default function VerifyEmailPage() {
  return (
    <Suspense fallback={<div className="min-h-screen bg-zinc-50 dark:bg-zinc-950 flex items-center justify-center"><div className="text-sm text-zinc-400">Carregando...</div></div>}>
      <VerifyEmailInner />
    </Suspense>
  )
}
