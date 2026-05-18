'use client'

import { useEffect, useState, useCallback } from 'react'
import Link from 'next/link'
import { useRouter, useSearchParams } from 'next/navigation'
import { fetchAdminSets, SetWithSeries } from '@/lib/catalog-admin'

const TCG_OPTIONS = [
  { value: 'pokemon', label: 'Pokémon' },
  { value: 'pokemon-pocket', label: 'Pokémon Pocket' },
  { value: 'magic', label: 'Magic' },
  { value: 'yugioh', label: 'Yu-Gi-Oh!' },
  { value: 'onepiece', label: 'One Piece' },
  { value: 'lorcana', label: 'Lorcana' },
  { value: 'fab', label: 'Flesh and Blood' },
]

const IMPORT_SOURCE_COLORS: Record<string, string> = {
  scrydex: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
  tcgdex_only: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  tcgdex_legacy: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400',
  manual: 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400',
}

function importSourceBadge(source: string) {
  const cls = IMPORT_SOURCE_COLORS[source] ?? 'bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400'
  return (
    <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${cls}`}>
      {source}
    </span>
  )
}

const inputCls =
  'rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500'

const LIMIT = 30

export default function SetsListPage() {
  const searchParams = useSearchParams()
  const router = useRouter()

  const [tcg, setTcg] = useState(searchParams.get('tcg') || 'pokemon')
  const [q, setQ] = useState(searchParams.get('q') || '')
  const [page, setPage] = useState(Number(searchParams.get('page') || '1'))

  const [sets, setSets] = useState<SetWithSeries[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Debounced search input
  const [debouncedQ, setDebouncedQ] = useState(q)
  useEffect(() => {
    const t = setTimeout(() => setDebouncedQ(q), 350)
    return () => clearTimeout(t)
  }, [q])

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const res = await fetchAdminSets({ tcg, q: debouncedQ || undefined, page, limit: LIMIT })
      setSets(res.items)
      setTotal(res.total)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Erro ao carregar sets')
    } finally {
      setLoading(false)
    }
  }, [tcg, debouncedQ, page])

  useEffect(() => { load() }, [load])

  // Sync URL params
  useEffect(() => {
    const params = new URLSearchParams()
    params.set('tcg', tcg)
    if (debouncedQ) params.set('q', debouncedQ)
    if (page > 1) params.set('page', String(page))
    router.replace(`/admin/catalogo/sets?${params.toString()}`, { scroll: false })
  }, [tcg, debouncedQ, page, router])

  function handleTcgChange(v: string) {
    setTcg(v)
    setPage(1)
  }

  function handleQChange(v: string) {
    setQ(v)
    setPage(1)
  }

  const totalPages = Math.ceil(total / LIMIT)

  return (
    <div className="mx-auto max-w-6xl px-4 py-6">
      <div className="mb-6 flex items-center justify-between gap-4">
        <div>
          <div className="flex items-center gap-2 text-sm text-zinc-500 mb-1">
            <Link href="/admin/catalogo" className="hover:text-zinc-900 dark:hover:text-zinc-100">
              Catalogo
            </Link>
            <span>/</span>
            <span className="text-zinc-900 dark:text-zinc-50">Sets</span>
          </div>
          <h1 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50">Sets</h1>
        </div>
        <Link
          href="/admin/catalogo/sets/novo"
          className="rounded-lg bg-violet-600 px-4 py-2 text-sm font-semibold text-white hover:bg-violet-700 transition-colors whitespace-nowrap"
        >
          + Novo Set
        </Link>
      </div>

      {/* Filters */}
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <select
          value={tcg}
          onChange={e => handleTcgChange(e.target.value)}
          className={`${inputCls} min-w-[160px]`}
        >
          {TCG_OPTIONS.map(opt => (
            <option key={opt.value} value={opt.value}>{opt.label}</option>
          ))}
        </select>
        <input
          type="text"
          placeholder="Buscar por nome ou codigo..."
          value={q}
          onChange={e => handleQChange(e.target.value)}
          className={`${inputCls} min-w-[240px] flex-1`}
        />
        {loading && (
          <span className="text-xs text-zinc-400">Carregando...</span>
        )}
      </div>

      {error && (
        <div className="rounded-xl border border-red-200 bg-red-50 px-5 py-4 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400 mb-4">
          {error}
        </div>
      )}

      {!loading && !error && sets.length === 0 && (
        <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
          <p className="text-zinc-400 text-sm">Nenhum set encontrado.</p>
          <Link
            href="/admin/catalogo/sets/novo"
            className="text-sm text-violet-600 hover:underline"
          >
            Criar o primeiro set
          </Link>
        </div>
      )}

      {sets.length > 0 && (
        <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-zinc-50 dark:bg-zinc-900 border-b border-zinc-200 dark:border-zinc-800">
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Codigo</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Nome</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Nome PT</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500 text-center">Cartas</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Lancamento</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Fonte</th>
                <th className="px-4 py-3 text-right font-medium text-zinc-500">Acoes</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-100 dark:divide-zinc-800">
              {sets.map(s => (
                <tr
                  key={s.id}
                  className="bg-white dark:bg-zinc-900 hover:bg-zinc-50 dark:hover:bg-zinc-800/50 transition-colors"
                >
                  <td className="px-4 py-3 font-mono text-xs text-zinc-600 dark:text-zinc-400">
                    {s.code}
                  </td>
                  <td className="px-4 py-3 font-medium text-zinc-900 dark:text-zinc-100 max-w-[180px] truncate">
                    {s.name}
                  </td>
                  <td className="px-4 py-3 text-zinc-500 max-w-[160px] truncate">
                    {s.name_pt || <span className="text-zinc-300 dark:text-zinc-600 italic">—</span>}
                  </td>
                  <td className="px-4 py-3 text-center text-zinc-500">
                    {s.total_cards > 0 ? s.total_cards : '—'}
                  </td>
                  <td className="px-4 py-3 text-zinc-500">
                    {s.release_date
                      ? new Date(s.release_date).toLocaleDateString('pt-BR')
                      : <span className="text-zinc-300 dark:text-zinc-600 italic">—</span>
                    }
                  </td>
                  <td className="px-4 py-3">
                    {importSourceBadge(s.import_source)}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <Link
                      href={`/admin/catalogo/sets/${s.id}`}
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

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="mt-4 flex items-center justify-between text-sm">
          <p className="text-zinc-500">
            {total} sets no total &middot; pagina {page} de {totalPages}
          </p>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setPage(p => Math.max(1, p - 1))}
              disabled={page <= 1}
              className="rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-1.5 text-sm text-zinc-700 dark:text-zinc-200 hover:bg-zinc-50 dark:hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              Anterior
            </button>
            <button
              onClick={() => setPage(p => Math.min(totalPages, p + 1))}
              disabled={page >= totalPages}
              className="rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-1.5 text-sm text-zinc-700 dark:text-zinc-200 hover:bg-zinc-50 dark:hover:bg-zinc-700 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
            >
              Proximo
            </button>
          </div>
        </div>
      )}
    </div>
  )
}
