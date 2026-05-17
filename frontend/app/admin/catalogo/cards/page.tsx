'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { authedFetch } from '@/lib/api'

const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

interface AutocompleteCard {
  id: string
  collector_number: string
  name: string
  name_pt?: string
  set_code?: string
  set_name?: string
  image_small_url?: string
}

const inputCls =
  'w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500'

export default function CardsSearchPage() {
  const [q, setQ] = useState('')
  const [results, setResults] = useState<AutocompleteCard[]>([])
  const [loading, setLoading] = useState(false)
  const [searched, setSearched] = useState(false)

  useEffect(() => {
    if (q.length < 2) {
      setResults([])
      setSearched(false)
      return
    }
    const t = setTimeout(async () => {
      setLoading(true)
      try {
        const res = await authedFetch(
          `${API_URL}/api/v1/cards/autocomplete?q=${encodeURIComponent(q)}&limit=20`,
        )
        if (res.ok) {
          const data = await res.json()
          setResults(data ?? [])
        } else {
          setResults([])
        }
      } catch {
        setResults([])
      } finally {
        setLoading(false)
        setSearched(true)
      }
    }, 350)
    return () => clearTimeout(t)
  }, [q])

  return (
    <div className="mx-auto max-w-4xl px-4 py-6">
      <div className="mb-6">
        <div className="flex items-center gap-2 text-sm text-zinc-500 mb-1">
          <Link href="/admin/catalogo" className="hover:text-zinc-900 dark:hover:text-zinc-100">
            Catalogo
          </Link>
          <span>/</span>
          <span className="text-zinc-900 dark:text-zinc-50">Cartas</span>
        </div>
        <h1 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50">Buscar carta</h1>
        <p className="text-sm text-zinc-500 mt-1">
          Busque uma carta pelo nome ou numero para editar.
        </p>
      </div>

      <div className="mb-6">
        <input
          type="text"
          value={q}
          onChange={e => setQ(e.target.value)}
          placeholder="Nome da carta ou numero (ex: Pikachu, 025, 025/217)..."
          className={inputCls}
          autoFocus
        />
        {loading && (
          <p className="mt-2 text-xs text-zinc-400">Buscando...</p>
        )}
      </div>

      {!loading && searched && results.length === 0 && (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <p className="text-zinc-400 text-sm">Nenhuma carta encontrada para &ldquo;{q}&rdquo;.</p>
        </div>
      )}

      {results.length > 0 && (
        <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-zinc-50 dark:bg-zinc-900 border-b border-zinc-200 dark:border-zinc-800">
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Carta</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Nome PT</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Set</th>
                <th className="px-4 py-3 text-right font-medium text-zinc-500">Acoes</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-100 dark:divide-zinc-800">
              {results.map(c => (
                <tr
                  key={c.id}
                  className="bg-white dark:bg-zinc-900 hover:bg-zinc-50 dark:hover:bg-zinc-800/50 transition-colors"
                >
                  <td className="px-4 py-2.5">
                    <div className="flex items-center gap-3">
                      {c.image_small_url && (
                        // eslint-disable-next-line @next/next/no-img-element
                        <img
                          src={c.image_small_url}
                          alt={c.name}
                          className="w-8 h-11 object-contain rounded flex-shrink-0"
                        />
                      )}
                      <div>
                        <p className="font-medium text-zinc-900 dark:text-zinc-100">{c.name}</p>
                        {c.collector_number && (
                          <p className="text-xs text-zinc-400 font-mono">#{c.collector_number}</p>
                        )}
                      </div>
                    </div>
                  </td>
                  <td className="px-4 py-2.5 text-zinc-500">
                    {c.name_pt || <span className="italic text-zinc-300 dark:text-zinc-600">—</span>}
                  </td>
                  <td className="px-4 py-2.5 text-zinc-500 text-xs font-mono">
                    {c.set_code || '—'}
                  </td>
                  <td className="px-4 py-2.5 text-right">
                    <Link
                      href={`/admin/catalogo/cards/${c.id}`}
                      className="text-xs text-violet-600 hover:text-violet-700 dark:text-violet-400 font-medium"
                    >
                      Editar
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {!q && (
        <div className="flex flex-col items-center justify-center py-20 text-center">
          <p className="text-zinc-400 text-sm">
            Digite pelo menos 2 caracteres para buscar.
          </p>
        </div>
      )}
    </div>
  )
}
