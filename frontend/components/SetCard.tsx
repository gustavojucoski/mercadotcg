'use client'

import Link from 'next/link'
import type { TCGSet } from '@/lib/types'
import { useLang } from '@/lib/locale'

interface SetCardProps {
  set: TCGSet
}

export function SetCard({ set }: SetCardProps) {
  const { t } = useLang()
  const displayName = t(set.name, set.name_pt)
  const displaySeries = t(set.series, set.series_pt)
  const lanSuffix = set.language && set.language !== 'en' ? `?lan=${set.language}` : ''

  return (
    <Link
      href={`/sets/${set.tcg}/${set.code}${lanSuffix}`}
      className="group flex flex-col rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-4 hover:border-violet-300 dark:hover:border-violet-700 hover:shadow-md transition-all"
    >
      <div className="flex items-center justify-center h-16 mb-3 relative">
        {set.image_url ? (
          // Using plain img because pokemontcg.io domain may not be in next.config remotePatterns
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={set.image_url}
            alt={displayName}
            className="max-h-full max-w-full object-contain"
            loading="lazy"
          />
        ) : (
          <div className="h-12 w-20 rounded bg-zinc-100 dark:bg-zinc-800" />
        )}
        {set.symbol_url && (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={set.symbol_url}
            alt=""
            aria-hidden="true"
            className="absolute bottom-0 right-0 h-5 w-5 object-contain opacity-70"
            loading="lazy"
          />
        )}
      </div>
      <p className="text-sm font-semibold text-zinc-900 dark:text-zinc-100 group-hover:text-violet-600 dark:group-hover:text-violet-400 transition-colors leading-snug">
        {displayName}
      </p>
      <p className="text-xs text-zinc-400 mt-0.5">{displaySeries}</p>
      <p className="text-xs text-zinc-400 mt-1">
        {set.total_cards} cartas
        {set.release_date && (
          <> &middot; {new Date(set.release_date).getFullYear()}</>
        )}
      </p>
    </Link>
  )
}
