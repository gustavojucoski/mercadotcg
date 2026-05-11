import type { Metadata } from 'next'
import Link from 'next/link'
import { SiteHeader } from '@/components/SiteHeader'

export const revalidate = 86400

export const metadata: Metadata = {
  title: 'Sets de TCG | MercadoTCG',
  description: 'Explore sets de cartas de todos os TCGs disponíveis no MercadoTCG.',
}

const TCG_LIST = [
  {
    id: 'pokemon',
    label: 'Pokémon TCG',
    description: 'Mais de 170 sets desde Base Set até as expansões mais recentes.',
    href: '/sets/pokemon',
    sets: '170+',
    color: 'from-yellow-400 to-red-500',
    icon: (
      <svg viewBox="0 0 24 24" className="size-10 text-white" fill="currentColor" aria-hidden="true">
        <circle cx="12" cy="12" r="10" opacity="0.2" />
        <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm0 18c-4.41 0-8-3.59-8-8s3.59-8 8-8 8 3.59 8 8-3.59 8-8 8zm-1-13h2v6h-2zm0 8h2v2h-2z" />
      </svg>
    ),
  },
]

export default function SetsHubPage() {
  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950">
      <SiteHeader />
      <main className="mx-auto max-w-6xl px-4 py-10">
        <div className="mb-8">
          <h1 className="text-3xl font-bold text-zinc-900 dark:text-zinc-50 mb-2">Sets de TCG</h1>
          <p className="text-zinc-500">Explore cartas por jogo de cartas colecionável.</p>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-5">
          {TCG_LIST.map(tcg => (
            <Link
              key={tcg.id}
              href={tcg.href}
              className="group relative rounded-2xl overflow-hidden border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 hover:shadow-lg transition-all hover:border-violet-300 dark:hover:border-violet-700"
            >
              <div className={`h-28 bg-gradient-to-br ${tcg.color} flex items-center justify-center`}>
                {tcg.icon}
              </div>
              <div className="p-5">
                <h2 className="text-lg font-semibold text-zinc-900 dark:text-zinc-100 group-hover:text-violet-600 dark:group-hover:text-violet-400 transition-colors">
                  {tcg.label}
                </h2>
                <p className="text-sm text-zinc-500 mt-1 leading-relaxed">{tcg.description}</p>
                <div className="mt-4 flex items-center justify-between">
                  <span className="text-xs text-zinc-400">{tcg.sets} sets</span>
                  <span className="text-xs font-medium text-violet-600 dark:text-violet-400 group-hover:underline">
                    Ver sets
                  </span>
                </div>
              </div>
            </Link>
          ))}
        </div>
      </main>
    </div>
  )
}
