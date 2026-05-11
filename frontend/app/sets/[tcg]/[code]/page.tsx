import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { SiteHeader } from '@/components/SiteHeader'
import { Breadcrumb } from '@/components/Breadcrumb'
import { CardGridFilter } from '@/components/CardGridFilter'
import { fetchAllSetCards, fetchSet, fetchSets } from '@/lib/catalog'

export const revalidate = 3600

const SUPPORTED_TCGS: Record<string, string> = {
  pokemon: 'Pokémon TCG',
}

interface Props {
  params: Promise<{ tcg: string; code: string }>
}

export async function generateStaticParams() {
  try {
    const data = await fetchSets('pokemon', 1, 20)
    return data.sets.map(s => ({ tcg: 'pokemon', code: s.code }))
  } catch {
    return []
  }
}

export async function generateMetadata({ params }: { params: Promise<{ tcg: string; code: string }> }): Promise<Metadata> {
  const { tcg, code } = await params
  try {
    const set = await fetchSet(tcg, code)
    const tcgLabel = SUPPORTED_TCGS[tcg] ?? tcg
    const name = set.name_pt && set.name_pt.length > 0 ? set.name_pt : set.name
    return {
      title: `${name} — ${tcgLabel} | MercadoTCG`,
      description: `${set.total_cards} cartas do set ${name}. Explore preços e variantes.`,
    }
  } catch {
    return { title: 'Set | MercadoTCG' }
  }
}

export default async function SetDetailPage({ params }: Props) {
  const { tcg, code } = await params

  const tcgLabel = SUPPORTED_TCGS[tcg]
  if (!tcgLabel) notFound()

  const [set, cards] = await Promise.all([
    fetchSet(tcg, code).catch(() => null),
    fetchAllSetCards(tcg, code).catch(() => []),
  ])

  if (!set) notFound()

  const setDisplayName = set.name_pt && set.name_pt.length > 0 ? set.name_pt : set.name
  const seriesDisplayName = set.series_pt && set.series_pt.length > 0 ? set.series_pt : set.series

  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950">
      <SiteHeader />
      <main className="mx-auto max-w-6xl px-4 py-10">
        <Breadcrumb
          items={[
            { label: 'MercadoTCG', href: '/' },
            { label: 'Sets', href: '/sets' },
            { label: tcgLabel, href: `/sets/${tcg}` },
            { label: setDisplayName },
          ]}
        />

        <div className="mt-6 mb-10 flex flex-col sm:flex-row gap-6 items-start">
          {set.image_url && (
            <div className="shrink-0">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={set.image_url}
                alt={setDisplayName}
                className="h-16 object-contain"
              />
            </div>
          )}
          <div>
            <h1 className="text-3xl font-bold text-zinc-900 dark:text-zinc-50">{setDisplayName}</h1>
            {set.name_pt && set.name_pt.length > 0 && (
              <p className="text-zinc-400 text-sm mt-0.5">{set.name}</p>
            )}
            <div className="flex items-center gap-4 mt-2 text-sm text-zinc-500 flex-wrap">
              <span>
                Série:{' '}
                <a
                  href={`/sets/${tcg}`}
                  className="text-violet-600 dark:text-violet-400 hover:underline"
                >
                  {seriesDisplayName}
                </a>
              </span>
              <span>Código: <code className="font-mono text-xs bg-zinc-100 dark:bg-zinc-800 px-1.5 py-0.5 rounded">{set.code.toUpperCase()}</code></span>
              {set.release_date && (
                <span>Lançamento: {new Date(set.release_date).toLocaleDateString('pt-BR')}</span>
              )}
              <span>{set.total_cards} cartas</span>
            </div>
          </div>
        </div>

        <CardGridFilter
          cards={cards}
          setCode={set.code}
        />
      </main>
    </div>
  )
}
