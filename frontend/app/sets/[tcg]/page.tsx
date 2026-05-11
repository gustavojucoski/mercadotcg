import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { SiteHeader } from '@/components/SiteHeader'
import { SetFilter } from '@/components/SetFilter'
import { fetchSets } from '@/lib/catalog'

export const revalidate = 86400

const SUPPORTED_TCGS: Record<string, string> = {
  pokemon: 'Pokémon TCG',
}

interface Props {
  params: Promise<{ tcg: string }>
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { tcg } = await params
  const label = SUPPORTED_TCGS[tcg]
  if (!label) return { title: 'Not Found' }
  return {
    title: `${label} — Sets | MercadoTCG`,
    description: `Explore todos os sets de ${label} disponíveis no MercadoTCG.`,
  }
}

export default async function TCGSetsPage({ params }: Props) {
  const { tcg } = await params
  const label = SUPPORTED_TCGS[tcg]
  if (!label) notFound()

  const data = await fetchSets(tcg, 1, 200).catch(() => null)
  const sets = data?.sets ?? []

  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950">
      <SiteHeader />
      <main className="mx-auto max-w-6xl px-4 py-10">
        <div className="mb-8">
          <nav className="text-sm text-zinc-400 mb-3 flex items-center gap-1.5">
            <a href="/sets" className="hover:text-zinc-600 dark:hover:text-zinc-300 transition-colors">
              Sets
            </a>
            <svg className="size-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden="true">
              <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
            </svg>
            <span className="text-zinc-600 dark:text-zinc-300">{label}</span>
          </nav>
          <h1 className="text-3xl font-bold text-zinc-900 dark:text-zinc-50">
            {label}
          </h1>
          {sets.length > 0 && (
            <p className="text-zinc-500 mt-1">{sets.length} sets disponíveis</p>
          )}
        </div>

        {sets.length === 0 ? (
          <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-12 text-center">
            <p className="text-zinc-400 text-sm">
              Nenhum set disponível. O catálogo será importado em breve.
            </p>
          </div>
        ) : (
          <SetFilter sets={sets} />
        )}
      </main>
    </div>
  )
}
