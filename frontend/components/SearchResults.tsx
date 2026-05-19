'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import Image from 'next/image'
import Link from 'next/link'
import { fetchSearchResults } from '@/lib/catalog'
import { POKEMON_RARITIES } from '@/lib/rarities'
import { useLang } from '@/lib/locale'
import type { SearchCardResult, SearchResponse } from '@/lib/types'

const RARITY_COLOR: Record<string, string> = {
  Common: 'bg-zinc-100 text-zinc-500 dark:bg-zinc-800 dark:text-zinc-400',
  Uncommon: 'bg-green-50 text-green-700 dark:bg-green-950/40 dark:text-green-400',
  Rare: 'bg-blue-50 text-blue-700 dark:bg-blue-950/40 dark:text-blue-400',
  'Double Rare': 'bg-violet-50 text-violet-700 dark:bg-violet-950/40 dark:text-violet-400',
  'Illustration Rare': 'bg-pink-50 text-pink-700 dark:bg-pink-950/40 dark:text-pink-400',
  'Special Illustration Rare': 'bg-pink-50 text-pink-700 dark:bg-pink-950/40 dark:text-pink-400',
  'Hyper Rare': 'bg-yellow-50 text-yellow-700 dark:bg-yellow-950/40 dark:text-yellow-400',
}

function rarityClass(rarity: string): string {
  return RARITY_COLOR[rarity] ?? 'bg-zinc-100 text-zinc-500 dark:bg-zinc-800 dark:text-zinc-400'
}

function cardHref(card: SearchCardResult): string {
  const lanSuffix = card.set.language !== 'en' ? `?lan=${card.set.language}` : ''
  return `/cards/${card.set.code}/${card.collector_number}${lanSuffix}`
}

function CardSkeleton() {
  return (
    <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 overflow-hidden animate-pulse">
      <div className="aspect-[2.5/3.5] bg-zinc-100 dark:bg-zinc-800" />
      <div className="p-2.5 space-y-1.5">
        <div className="h-3 bg-zinc-100 dark:bg-zinc-800 rounded w-3/4" />
        <div className="h-3 bg-zinc-100 dark:bg-zinc-800 rounded w-1/2" />
        <div className="h-4 bg-zinc-100 dark:bg-zinc-800 rounded w-1/3 mt-1" />
      </div>
    </div>
  )
}

function SearchCardItem({ card }: { card: SearchCardResult }) {
  const { t } = useLang()
  const displayName = t(card.name, card.name_pt)
  const setName = t(card.set.name, card.set.name_pt)

  return (
    <Link
      href={cardHref(card)}
      className="group flex flex-col rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 overflow-hidden hover:border-violet-300 dark:hover:border-violet-700 hover:shadow-md transition-all"
    >
      <div className="bg-zinc-50 dark:bg-zinc-800/50 flex items-center justify-center p-2 aspect-[2.5/3.5] relative">
        {card.image_url ? (
          <Image
            src={card.image_url}
            alt={displayName}
            fill
            sizes="(max-width: 640px) 45vw, (max-width: 768px) 30vw, (max-width: 1024px) 22vw, 16vw"
            className="object-contain rounded group-hover:scale-105 transition-transform duration-200 p-2"
            loading="lazy"
          />
        ) : (
          <div className="w-full h-full rounded bg-zinc-200 dark:bg-zinc-700 flex items-center justify-center">
            <span className="text-xs text-zinc-400">{card.collector_number}</span>
          </div>
        )}
      </div>
      <div className="p-2.5">
        <p className="text-xs font-medium text-zinc-900 dark:text-zinc-100 leading-snug truncate group-hover:text-violet-600 dark:group-hover:text-violet-400 transition-colors">
          {displayName}
        </p>
        <p className="text-xs text-zinc-400 mt-0.5 truncate">
          {setName} &middot; #{card.collector_number}
        </p>
        {card.rarity && (
          <span
            className={`inline-block mt-1.5 rounded px-1.5 py-0.5 text-[10px] font-medium ${rarityClass(card.rarity)}`}
          >
            {card.rarity}
          </span>
        )}
      </div>
    </Link>
  )
}

export interface SearchResultsProps {
  initialData: SearchResponse | null
  initialQ: string
  initialSort: string
  initialOrder: string
  initialTcg: string
  initialRarity: string
  initialLang: string
}

export function SearchResults({
  initialData,
  initialQ,
  initialSort,
  initialOrder,
  initialTcg,
  initialRarity,
  initialLang,
}: SearchResultsProps) {
  const router = useRouter()
  const { t } = useLang()

  const [query, setQuery] = useState(initialQ)
  const [sort, setSort] = useState(initialSort)
  const [order, setOrder] = useState(initialOrder)
  const [tcg, setTcg] = useState(initialTcg)
  const [rarity, setRarity] = useState(initialRarity)
  const [lang, setLang] = useState(initialLang)

  const [cards, setCards] = useState<SearchCardResult[]>(initialData?.data ?? [])
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(initialData?.total ?? 0)
  const [hasMore, setHasMore] = useState(initialData?.has_more ?? false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState(false)

  const requestSeq = useRef(0)
  const sentinelRef = useRef<HTMLDivElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const syncUrl = useCallback(
    (q: string, s: string, o: string, tc: string, r: string, l: string) => {
      const params = new URLSearchParams()
      if (q.trim()) params.set('q', q.trim())
      if (s && s !== 'name') params.set('sort', s)
      if (o && o !== 'asc') params.set('order', o)
      if (tc) params.set('tcg', tc)
      if (r) params.set('rarity', r)
      if (l && l !== 'en') params.set('lang', l)
      const qs = params.toString()
      router.replace(qs ? `?${qs}` : '/search', { scroll: false })
    },
    [router],
  )

  const doSearch = useCallback(
    async (
      q: string,
      s: string,
      o: string,
      tc: string,
      r: string,
      l: string,
      targetPage: number,
      reset: boolean,
    ) => {
      const seq = ++requestSeq.current
      setLoading(true)
      setError(false)
      try {
        const data = await fetchSearchResults({
          q: q.trim() || undefined,
          sort: s || undefined,
          order: o || undefined,
          tcg: tc || undefined,
          rarity: r || undefined,
          lang: l || undefined,
          page: targetPage,
          limit: 24,
        })
        if (seq !== requestSeq.current) return
        if (reset) {
          setCards(data.data)
        } else {
          setCards(prev => [...prev, ...data.data])
        }
        setTotal(data.total)
        setHasMore(data.has_more)
        setPage(targetPage)
      } catch {
        if (seq !== requestSeq.current) return
        setError(true)
      } finally {
        if (seq === requestSeq.current) setLoading(false)
      }
    },
    [],
  )

  const handleQueryChange = (value: string) => {
    setQuery(value)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => {
      syncUrl(value, sort, order, tcg, rarity, lang)
      doSearch(value, sort, order, tcg, rarity, lang, 1, true)
    }, 300)
  }

  const handleFilterChange = (
    newSort: string,
    newOrder: string,
    newTcg: string,
    newRarity: string,
    newLang: string,
  ) => {
    setSort(newSort)
    setOrder(newOrder)
    setTcg(newTcg)
    setRarity(newRarity)
    setLang(newLang)
    syncUrl(query, newSort, newOrder, newTcg, newRarity, newLang)
    doSearch(query, newSort, newOrder, newTcg, newRarity, newLang, 1, true)
  }

  useEffect(() => {
    const sentinel = sentinelRef.current
    if (!sentinel) return
    const observer = new IntersectionObserver(
      entries => {
        if (entries[0].isIntersecting && hasMore && !loading) {
          doSearch(query, sort, order, tcg, rarity, lang, page + 1, false)
        }
      },
      { threshold: 0.1 },
    )
    observer.observe(sentinel)
    return () => observer.disconnect()
  }, [hasMore, loading, page, query, sort, order, tcg, rarity, lang, doSearch])

  const hasActiveFilters = tcg || rarity || (lang && lang !== 'en')

  return (
    <div>
      <div className="relative mb-4">
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
          placeholder="Buscar cartas por nome..."
          className="w-full rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 pl-10 pr-4 py-2.5 text-sm text-zinc-900 dark:text-zinc-100 placeholder:text-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500 focus:border-transparent transition-colors"
          aria-label="Buscar cartas"
        />
      </div>

      <div className="flex flex-wrap items-center gap-3 mb-6">
        <select
          value={tcg}
          onChange={e => handleFilterChange(sort, order, e.target.value, rarity, lang)}
          className="rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-700 dark:text-zinc-300 focus:outline-none focus:ring-2 focus:ring-violet-500"
          aria-label="Filtrar por TCG"
        >
          <option value="">Todos os TCGs</option>
          <option value="pokemon">Pokémon TCG</option>
          <option value="pokemon-pocket">Pokémon TCG Pocket</option>
        </select>

        <select
          value={rarity}
          onChange={e => handleFilterChange(sort, order, tcg, e.target.value, lang)}
          className="rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-700 dark:text-zinc-300 focus:outline-none focus:ring-2 focus:ring-violet-500"
          aria-label="Filtrar por raridade"
        >
          <option value="">Todas as raridades</option>
          {POKEMON_RARITIES.map(r => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </select>

        <select
          value={lang}
          onChange={e => handleFilterChange(sort, order, tcg, rarity, e.target.value)}
          className="rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-700 dark:text-zinc-300 focus:outline-none focus:ring-2 focus:ring-violet-500"
          aria-label="Filtrar por idioma"
        >
          <option value="">Todos os idiomas</option>
          <option value="en">Inglês</option>
          <option value="ja">Japonês</option>
          <option value="pt">Português</option>
        </select>

        <div className="flex items-center gap-2 ml-auto">
          <select
            value={sort}
            onChange={e => handleFilterChange(e.target.value, order, tcg, rarity, lang)}
            className="rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-700 dark:text-zinc-300 focus:outline-none focus:ring-2 focus:ring-violet-500"
            aria-label="Ordenar por"
          >
            <option value="name">Nome</option>
            <option value="release_date">Lançamento</option>
            <option value="collector_number">Número</option>
          </select>

          <button
            type="button"
            onClick={() =>
              handleFilterChange(sort, order === 'asc' ? 'desc' : 'asc', tcg, rarity, lang)
            }
            className="rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-700 dark:text-zinc-300 hover:border-violet-400 focus:outline-none focus:ring-2 focus:ring-violet-500 transition-colors"
            aria-label={order === 'asc' ? 'Ordem crescente — clique para decrescente' : 'Ordem decrescente — clique para crescente'}
          >
            {order === 'asc' ? (
              <svg className="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true">
                <path strokeLinecap="round" strokeLinejoin="round" d="M3 4h13M3 8h9M3 12h5m10 4V6m0 0l-3 3m3-3l3 3" />
              </svg>
            ) : (
              <svg className="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true">
                <path strokeLinecap="round" strokeLinejoin="round" d="M3 4h13M3 8h9M3 12h5m10-4v14m0 0l-3-3m3 3l3-3" />
              </svg>
            )}
          </button>
        </div>
      </div>

      {query.trim() && !loading && (
        <p className="text-sm text-zinc-500 dark:text-zinc-400 mb-6">
          {total > 0
            ? `${total} ${total === 1 ? 'resultado' : 'resultados'} para "${query.trim()}"`
            : null}
        </p>
      )}

      {error && (
        <div className="rounded-xl border border-red-200 dark:border-red-900/40 bg-red-50 dark:bg-red-950/20 p-6 text-center mb-6">
          <p className="text-sm text-red-600 dark:text-red-400">
            Erro ao carregar resultados. Tente novamente.
          </p>
        </div>
      )}

      {!loading && !error && cards.length === 0 && query.trim() && (
        <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-12 text-center">
          <p className="text-zinc-500 dark:text-zinc-400 text-sm mb-2">
            Nenhum resultado encontrado para &ldquo;{query.trim()}&rdquo;
          </p>
          {hasActiveFilters && (
            <button
              type="button"
              onClick={() => handleFilterChange(sort, order, '', '', '')}
              className="text-xs text-violet-600 dark:text-violet-400 hover:underline mt-1"
            >
              Remover filtros ativos
            </button>
          )}
        </div>
      )}

      {!loading && !error && cards.length === 0 && !query.trim() && (
        <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-12 text-center">
          <svg
            className="mx-auto mb-4 size-10 text-zinc-300 dark:text-zinc-600"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={1.5}
            aria-hidden="true"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M21 21l-4.35-4.35m0 0A7.5 7.5 0 104.5 4.5a7.5 7.5 0 0012.15 12.15z"
            />
          </svg>
          <p className="text-zinc-400 text-sm">
            {t('Type something to search for cards', 'Digite algo para buscar cartas')}
          </p>
        </div>
      )}

      {(cards.length > 0 || loading) && (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3">
          {cards.map(card => (
            <SearchCardItem key={card.id} card={card} />
          ))}
          {loading &&
            Array.from({ length: 12 }).map((_, i) => (
              <CardSkeleton key={`sk-${i}`} />
            ))}
        </div>
      )}

      <div ref={sentinelRef} className="py-4 text-center">
        {!loading && hasMore && (
          <button
            type="button"
            onClick={() => doSearch(query, sort, order, tcg, rarity, lang, page + 1, false)}
            className="text-sm text-violet-600 dark:text-violet-400 hover:underline"
          >
            Carregar mais
          </button>
        )}
        {!loading && !hasMore && cards.length > 0 && (
          <p className="text-zinc-300 dark:text-zinc-600 text-xs">
            {total} {total === 1 ? 'carta' : 'cartas'} carregadas
          </p>
        )}
      </div>
    </div>
  )
}
