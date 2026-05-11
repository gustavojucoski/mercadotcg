'use client'

import { useMemo, useState } from 'react'
import Link from 'next/link'
import type { CardInSet } from '@/lib/types'
import { CardThumbnail } from '@/components/CardThumbnail'

interface CardGridFilterProps {
  cards: CardInSet[]
  setCode: string
}

function normalizeNum(s: string): string {
  const n = parseInt(s, 10)
  return isNaN(n) ? s.toLowerCase() : String(n)
}

export function CardGridFilter({ cards, setCode }: CardGridFilterProps) {
  const [query, setQuery] = useState('')
  const [selectedRarities, setSelectedRarities] = useState<Set<string>>(new Set())
  const [view, setView] = useState<'grid' | 'list'>('grid')

  const availableRarities = useMemo(() => {
    const seen = new Set<string>()
    for (const c of cards) if (c.rarity) seen.add(c.rarity)
    return Array.from(seen).sort()
  }, [cards])

  const filtered = useMemo(() => {
    let result = cards
    if (query.trim()) {
      const q = query.trim()
      const qNorm = normalizeNum(q)
      result = result.filter(
        c =>
          c.name.toLowerCase().includes(q) ||
          (c.name_pt && c.name_pt.toLowerCase().includes(q)) ||
          normalizeNum(c.collector_number).includes(qNorm),
      )
    }
    if (selectedRarities.size > 0) {
      result = result.filter(c => selectedRarities.has(c.rarity))
    }
    return result
  }, [cards, query, selectedRarities])

  function toggleRarity(r: string) {
    setSelectedRarities(prev => {
      const next = new Set(prev)
      if (next.has(r)) next.delete(r)
      else next.add(r)
      return next
    })
  }

  return (
    <div>
      <div className="flex flex-col sm:flex-row gap-3 mb-6">
        <div className="relative flex-1 sm:max-w-xs">
          <svg
            className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-4 text-zinc-400"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
            aria-hidden="true"
          >
            <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-4.35-4.35m0 0A7.5 7.5 0 104.5 4.5a7.5 7.5 0 0012.15 12.15z" />
          </svg>
          <input
            type="search"
            value={query}
            onChange={e => setQuery(e.target.value)}
            placeholder="Filtrar por nome ou número..."
            className="w-full rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 pl-10 pr-4 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder:text-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500 focus:border-transparent transition-colors"
          />
        </div>

        <div className="flex items-center gap-2 flex-wrap">
          {availableRarities.map(r => (
            <button
              key={r}
              onClick={() => toggleRarity(r)}
              className={`rounded-full px-3 py-1 text-xs font-medium border transition-colors ${
                selectedRarities.has(r)
                  ? 'bg-violet-600 border-violet-600 text-white'
                  : 'border-zinc-200 dark:border-zinc-700 text-zinc-600 dark:text-zinc-300 hover:border-violet-400 dark:hover:border-violet-600'
              }`}
            >
              {r}
            </button>
          ))}
        </div>

        <div className="flex items-center gap-1 ml-auto">
          <button
            onClick={() => setView('grid')}
            aria-label="Grade"
            className={`p-1.5 rounded-md transition-colors ${
              view === 'grid'
                ? 'bg-zinc-100 dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100'
                : 'text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200'
            }`}
          >
            <svg className="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z" />
            </svg>
          </button>
          <button
            onClick={() => setView('list')}
            aria-label="Lista"
            className={`p-1.5 rounded-md transition-colors ${
              view === 'list'
                ? 'bg-zinc-100 dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100'
                : 'text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200'
            }`}
          >
            <svg className="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M8.25 6.75h12M8.25 12h12m-12 5.25h12M3.75 6.75h.007v.008H3.75V6.75zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zM3.75 12h.007v.008H3.75V12zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0zm-.375 5.25h.007v.008H3.75v-.008zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z" />
            </svg>
          </button>
        </div>
      </div>

      <p className="text-xs text-zinc-400 mb-4">
        {query || selectedRarities.size > 0
          ? `${filtered.length} de ${cards.length} cartas`
          : `${cards.length} cartas`}
      </p>

      {view === 'grid' ? (
        <div className="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 xl:grid-cols-7 gap-3">
          {filtered.map(card => (
            <CardThumbnail key={card.id} card={card} setCode={setCode} />
          ))}
        </div>
      ) : (
        <div className="space-y-2">
          {filtered.map(card => {
            const slug = `${setCode}-${card.collector_number}`
            const displayName = card.name_pt && card.name_pt.length > 0 ? card.name_pt : card.name
            return (
              <Link
                key={card.id}
                href={`/cards/${slug}`}
                className="flex items-center gap-3 rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-4 py-3 hover:border-violet-300 dark:hover:border-violet-700 transition-colors group"
              >
                {card.image_small_url ? (
                  // eslint-disable-next-line @next/next/no-img-element
                  <img
                    src={card.image_small_url}
                    alt={displayName}
                    width={32}
                    height={44}
                    className="rounded object-contain shrink-0"
                    loading="lazy"
                  />
                ) : (
                  <div className="w-8 h-11 rounded bg-zinc-100 dark:bg-zinc-800 shrink-0" />
                )}
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium text-zinc-900 dark:text-zinc-100 group-hover:text-violet-600 dark:group-hover:text-violet-400 transition-colors truncate">
                    {displayName}
                  </p>
                  <p className="text-xs text-zinc-400 mt-0.5">{card.rarity}</p>
                </div>
                <p className="text-xs text-zinc-400 shrink-0">#{card.collector_number}</p>
              </Link>
            )
          })}
        </div>
      )}

    </div>
  )
}
