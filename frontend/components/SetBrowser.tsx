'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import type { SetListResponse, TCGSet } from '@/lib/types'
import { SetCard } from '@/components/SetCard'
import { useLang } from '@/lib/locale'
import { API_URL } from '@/lib/config'

interface SetGroup {
  seriesId: string
  seriesName: string
  seriesNamePt: string
  latestRelease: string
  sets: TCGSet[]
}

function groupSetsBySeries(sets: TCGSet[]): SetGroup[] {
  const map = new Map<string, SetGroup>()

  for (const s of sets) {
    const key = s.series_id || s.series
    if (!map.has(key)) {
      map.set(key, {
        seriesId: key,
        seriesName: s.series,
        seriesNamePt: s.series_pt,
        latestRelease: s.release_date ?? '',
        sets: [],
      })
    }
    const group = map.get(key)!
    group.sets.push(s)
    if (s.release_date && s.release_date > group.latestRelease) {
      group.latestRelease = s.release_date
    }
  }

  return Array.from(map.values()).sort((a, b) =>
    b.latestRelease.localeCompare(a.latestRelease),
  )
}

async function fetchSetsClient(
  tcg: string,
  page: number,
  limit: number,
  q: string,
): Promise<SetListResponse> {
  const params = new URLSearchParams({
    page: String(page),
    limit: String(limit),
  })
  if (q.trim()) params.set('q', q.trim())
  const res = await fetch(
    `${API_URL}/api/v1/sets/${encodeURIComponent(tcg)}?${params}`,
    { cache: 'no-store' },
  )
  if (!res.ok) throw new Error(`fetchSets: HTTP ${res.status}`)
  return res.json() as Promise<SetListResponse>
}

export interface SetBrowserProps {
  tcg: string
  initialData: SetListResponse | null
  initialQ: string
}

export function SetBrowser({ tcg, initialData, initialQ }: SetBrowserProps) {
  const router = useRouter()
  const { t } = useLang()

  const [query, setQuery] = useState(initialQ)
  const [sets, setSets] = useState<TCGSet[]>(initialData?.sets ?? [])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(initialData?.total ?? 0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(false)

  const requestSeq = useRef(0)
  const sentinelRef = useRef<HTMLDivElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const hasMore = sets.length < total

  const doSearch = useCallback(
    async (q: string, targetPage: number, reset: boolean) => {
      const seq = ++requestSeq.current
      setLoading(true)
      setError(false)
      try {
        const data = await fetchSetsClient(tcg, targetPage, 24, q)
        if (seq !== requestSeq.current) return
        if (reset) {
          setSets(data.sets)
        } else {
          setSets(prev => [...prev, ...data.sets])
        }
        setTotal(data.total)
        setPage(targetPage)
      } catch {
        if (seq !== requestSeq.current) return
        setError(true)
      } finally {
        if (seq === requestSeq.current) setLoading(false)
      }
    },
    [tcg],
  )

  const handleQueryChange = (value: string) => {
    setQuery(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      const params = new URLSearchParams()
      if (value.trim()) params.set('q', value.trim())
      const qs = params.toString()
      router.replace(qs ? `?${qs}` : '?', { scroll: false })
      doSearch(value.trim(), 1, true)
    }, 300)
  }

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!sentinel) return
    const observer = new IntersectionObserver(
      entries => {
        if (entries[0].isIntersecting && hasMore && !loading) {
          doSearch(query.trim(), page + 1, false)
        }
      },
      { threshold: 0.1 },
    )
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [hasMore, loading, page, query, doSearch])

  const groups = groupSetsBySeries(sets)

  return (
    <div>
      <div className="relative mb-8">
        <svg
          className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-zinc-400"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
          aria-hidden="true"
        >
          <path
            strokeLinecap="round"
            strokeLinejoin="round"
            d="M21 21l-4.35-4.35m0 0A7.5 7.5 0 104.5 4.5a7.5 7.5 0 0012.15 12.15z"
          />
        </svg>
        <input
          type="search"
          value={query}
          onChange={e => handleQueryChange(e.target.value)}
          placeholder="Filtrar por nome ou sigla..."
          className="w-full sm:max-w-sm rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 pl-10 pr-4 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder:text-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500 focus:border-transparent transition-colors"
        />
      </div>

      {error && (
        <p className="text-red-500 text-sm py-4">
          Erro ao carregar sets. Tente novamente.
        </p>
      )}

      {!loading && sets.length === 0 && query.trim() && (
        <p className="text-zinc-400 text-sm py-8 text-center">
          Nenhum set encontrado para &ldquo;{query}&rdquo;.
        </p>
      )}

      {!loading && sets.length === 0 && !query.trim() && (
        <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-12 text-center">
          <p className="text-zinc-400 text-sm">Nenhum set disponível.</p>
        </div>
      )}

      {groups.length > 0 && (
        <div className="space-y-10">
          {groups.map(group => {
            const seriesLabel = t(group.seriesName, group.seriesNamePt)
            return (
              <section key={group.seriesId}>
                <h2 className="text-base font-semibold text-zinc-900 dark:text-zinc-100 mb-4 flex items-center gap-2">
                  {seriesLabel}
                  <span className="text-xs font-normal text-zinc-400">
                    {group.sets.length}{' '}
                    {group.sets.length === 1 ? 'set' : 'sets'}
                  </span>
                </h2>
                <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3">
                  {group.sets.map(s => (
                    <SetCard key={s.id} set={s} />
                  ))}
                </div>
              </section>
            )
          })}
        </div>
      )}

      <div ref={sentinelRef} className="py-4 text-center">
        {loading && <p className="text-zinc-400 text-sm">Carregando...</p>}
        {!loading && hasMore && (
          <button
            onClick={() => doSearch(query.trim(), page + 1, false)}
            className="text-sm text-violet-600 hover:underline"
          >
            Carregar mais
          </button>
        )}
        {!loading && !hasMore && sets.length > 0 && (
          <p className="text-zinc-300 text-xs">Todos os sets carregados</p>
        )}
      </div>
    </div>
  )
}
