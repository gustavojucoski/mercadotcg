import type { Metadata } from 'next'
import { Suspense } from 'react'
import { SiteHeader } from '@/components/SiteHeader'
import { SearchResults } from '@/components/SearchResults'
import { fetchSearchResultsServer } from '@/lib/catalog'
import type { SearchResponse } from '@/lib/types'

export const metadata: Metadata = {
  title: 'Buscar Cartas | MercadoTCG',
  description: 'Busque por cartas de Pokémon TCG, filtre por raridade, idioma e set.',
  robots: { index: false, follow: false },
}

interface Props {
  searchParams: Promise<{
    q?: string
    sort?: string
    order?: string
    tcg?: string
    rarity?: string
    lang?: string
  }>
}

function SearchSkeleton() {
  return (
    <div className="animate-pulse">
      <div className="h-10 bg-zinc-100 dark:bg-zinc-800 rounded-lg mb-4 w-full" />
      <div className="flex gap-3 mb-6">
        <div className="h-9 bg-zinc-100 dark:bg-zinc-800 rounded-lg w-36" />
        <div className="h-9 bg-zinc-100 dark:bg-zinc-800 rounded-lg w-44" />
        <div className="h-9 bg-zinc-100 dark:bg-zinc-800 rounded-lg w-36" />
        <div className="h-9 bg-zinc-100 dark:bg-zinc-800 rounded-lg w-32 ml-auto" />
      </div>
      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-3">
        {Array.from({ length: 24 }).map((_, i) => (
          <div
            key={i}
            className="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 overflow-hidden"
          >
            <div className="aspect-[2.5/3.5] bg-zinc-100 dark:bg-zinc-800" />
            <div className="p-2.5 space-y-1.5">
              <div className="h-3 bg-zinc-100 dark:bg-zinc-800 rounded w-3/4" />
              <div className="h-3 bg-zinc-100 dark:bg-zinc-800 rounded w-1/2" />
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

async function SearchResultsLoader({ q, sort, order, tcg, rarity, lang }: {
  q: string
  sort: string
  order: string
  tcg: string
  rarity: string
  lang: string
}) {
  let initialData: SearchResponse | null = null

  if (q.trim()) {
    initialData = await fetchSearchResultsServer({
      q: q.trim(),
      sort: sort || undefined,
      order: order || undefined,
      tcg: tcg || undefined,
      rarity: rarity || undefined,
      lang: lang || undefined,
      page: 1,
      limit: 24,
    }).catch(() => null)
  }

  return (
    <SearchResults
      key={`${q}|${sort}|${order}|${tcg}|${rarity}|${lang}`}
      initialData={initialData}
      initialQ={q}
      initialSort={sort}
      initialOrder={order}
      initialTcg={tcg}
      initialRarity={rarity}
      initialLang={lang}
    />
  )
}

export default async function SearchPage({ searchParams }: Props) {
  const {
    q = '',
    sort = 'name',
    order = 'asc',
    tcg = '',
    rarity = '',
    lang = 'en',
  } = await searchParams

  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950">
      <SiteHeader />
      <main className="mx-auto max-w-6xl px-4 py-10">
        <div className="mb-8">
          <h1 className="text-3xl font-bold text-zinc-900 dark:text-zinc-50">
            Buscar Cartas
          </h1>
          <p className="text-zinc-500 mt-1 text-sm">
            Pesquise por nome, set ou número de colecionador.
          </p>
        </div>

        <Suspense fallback={<SearchSkeleton />}>
          <SearchResultsLoader
            q={q}
            sort={sort}
            order={order}
            tcg={tcg}
            rarity={rarity}
            lang={lang}
          />
        </Suspense>
      </main>
    </div>
  )
}
