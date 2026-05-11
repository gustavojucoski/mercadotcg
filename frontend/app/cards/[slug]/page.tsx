import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { SiteHeader } from '@/components/SiteHeader'
import { Breadcrumb } from '@/components/Breadcrumb'
import { VariantTabs } from '@/components/VariantTabs'
import { fetchCard } from '@/lib/catalog'

export const revalidate = 3600

interface Props {
  params: Promise<{ slug: string }>
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = await params
  try {
    const data = await fetchCard(slug)
    const { card, set } = data
    const name = card.name_pt && card.name_pt.length > 0 ? card.name_pt : card.name
    const setName = set.name_pt && set.name_pt.length > 0 ? set.name_pt : set.name
    const title = `${name} ${card.collector_number} — ${setName} | MercadoTCG`
    const description = `Carta ${name} do set ${setName}. Número ${card.collector_number}, raridade ${card.rarity}. Veja preços e variantes.`
    return {
      title,
      description,
      openGraph: {
        title,
        description,
        images: card.image_small_url ? [{ url: card.image_small_url }] : [],
      },
    }
  } catch {
    return { title: 'Carta | MercadoTCG' }
  }
}

const FINISH_LABEL: Record<string, string> = {
  holo: 'Holo',
  reverse_holo: 'Reverse Holo',
  normal: 'Normal',
  etched: 'Etched',
  master_ball: 'Master Ball',
  poke_ball: 'Poke Ball',
}

function finishLabel(finish: string, label?: string): string {
  if (label && label.length > 0) return label
  return FINISH_LABEL[finish] ?? finish
}

function formatBRL(value: string): string {
  const n = parseFloat(value)
  if (isNaN(n)) return 'R$ --'
  return n.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL' })
}

export default async function CardDetailPage({ params }: Props) {
  const { slug } = await params

  const data = await fetchCard(slug).catch(() => null)
  if (!data) notFound()

  const { card, set, variants } = data
  const cardName = card.name_pt && card.name_pt.length > 0 ? card.name_pt : card.name
  const setName = set.name_pt && set.name_pt.length > 0 ? set.name_pt : set.name
  const seriesName = set.series_pt && set.series_pt.length > 0 ? set.series_pt : set.series
  const tcg = set.tcg

  const lowestPrice = variants
    .map(v => v.price_summary?.min_brl)
    .filter(Boolean)
    .map(p => parseFloat(p!))
    .sort((a, b) => a - b)[0]

  const jsonLd = {
    '@context': 'https://schema.org',
    '@type': 'Product',
    name: cardName,
    image: card.image_url || card.image_small_url,
    description: `Carta ${cardName} do set ${setName}. Número ${card.collector_number}, raridade ${card.rarity}.`,
    offers: variants.map(v => ({
      '@type': 'Offer',
      priceCurrency: 'BRL',
      price: v.price_summary?.min_brl ?? '0',
      availability: 'https://schema.org/InStock',
      name: finishLabel(v.finish, v.label),
    })),
  }

  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950">
      <SiteHeader />

      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }}
      />

      <main className="mx-auto max-w-6xl px-4 py-10">
        <Breadcrumb
          items={[
            { label: 'MercadoTCG', href: '/' },
            { label: 'Sets', href: '/sets' },
            { label: tcg === 'pokemon' ? 'Pokémon TCG' : tcg, href: `/sets/${tcg}` },
            { label: setName, href: `/sets/${tcg}/${set.code}` },
            { label: cardName },
          ]}
        />

        <div className="mt-8 grid grid-cols-1 lg:grid-cols-[340px_1fr] gap-10">
          <div className="lg:sticky lg:top-6 lg:self-start">
            <VariantTabs
              variants={variants}
              imageSrc={card.image_url || card.image_small_url}
              imageAlt={cardName}
            />
          </div>

          <div>
            <h1 className="text-3xl font-bold text-zinc-900 dark:text-zinc-50 leading-tight">
              {cardName}
            </h1>
            {card.name_pt && card.name_pt.length > 0 && (
              <p className="text-zinc-400 mt-1">{card.name}</p>
            )}

            {lowestPrice && (
              <p className="mt-3 text-2xl font-bold text-violet-600 dark:text-violet-400">
                a partir de {formatBRL(String(lowestPrice))}
              </p>
            )}

            <div className="mt-6 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5">
              <h2 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300 mb-4 uppercase tracking-wide">
                Informações da carta
              </h2>
              <dl className="space-y-3">
                <div className="flex items-start gap-2">
                  <dt className="w-32 shrink-0 text-sm text-zinc-400">Set</dt>
                  <dd className="text-sm text-zinc-900 dark:text-zinc-100">
                    <a
                      href={`/sets/${tcg}/${set.code}`}
                      className="text-violet-600 dark:text-violet-400 hover:underline"
                    >
                      {setName}
                    </a>
                  </dd>
                </div>
                <div className="flex items-start gap-2">
                  <dt className="w-32 shrink-0 text-sm text-zinc-400">Série</dt>
                  <dd className="text-sm text-zinc-900 dark:text-zinc-100">
                    <a
                      href={`/sets/${tcg}`}
                      className="text-violet-600 dark:text-violet-400 hover:underline"
                    >
                      {seriesName}
                    </a>
                  </dd>
                </div>
                <div className="flex items-start gap-2">
                  <dt className="w-32 shrink-0 text-sm text-zinc-400">Número</dt>
                  <dd className="text-sm text-zinc-900 dark:text-zinc-100 font-mono">
                    {card.collector_number}
                    {set.total_cards > 0 && <>/{set.total_cards}</>}
                  </dd>
                </div>
                <div className="flex items-start gap-2">
                  <dt className="w-32 shrink-0 text-sm text-zinc-400">Raridade</dt>
                  <dd className="text-sm text-zinc-900 dark:text-zinc-100">{card.rarity}</dd>
                </div>
                <div className="flex items-start gap-2">
                  <dt className="w-32 shrink-0 text-sm text-zinc-400">Supertipo</dt>
                  <dd className="text-sm text-zinc-900 dark:text-zinc-100">{card.supertype}</dd>
                </div>
                {card.subtypes && card.subtypes.length > 0 && (
                  <div className="flex items-start gap-2">
                    <dt className="w-32 shrink-0 text-sm text-zinc-400">Subtipo</dt>
                    <dd className="text-sm text-zinc-900 dark:text-zinc-100">{card.subtypes.join(', ')}</dd>
                  </div>
                )}
                {card.types && card.types.length > 0 && (
                  <div className="flex items-start gap-2">
                    <dt className="w-32 shrink-0 text-sm text-zinc-400">Tipo</dt>
                    <dd className="flex gap-1.5 flex-wrap">
                      {card.types.map(t => (
                        <span
                          key={t}
                          className="rounded-full bg-zinc-100 dark:bg-zinc-800 px-2.5 py-0.5 text-xs font-medium text-zinc-700 dark:text-zinc-300"
                        >
                          {t}
                        </span>
                      ))}
                    </dd>
                  </div>
                )}
              </dl>
            </div>

            <div className="mt-6">
              <h2 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300 mb-3 uppercase tracking-wide">
                Variantes
              </h2>
              <div className="space-y-3">
                {variants.map(v => (
                  <div
                    key={v.id}
                    className="rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-4 flex items-center justify-between gap-4"
                  >
                    <div>
                      <p className="text-sm font-medium text-zinc-900 dark:text-zinc-100 flex items-center gap-2">
                        {finishLabel(v.finish, v.label)}
                        {v.is_promo && (
                          <span className="inline-block rounded bg-yellow-100 dark:bg-yellow-950/40 text-yellow-700 dark:text-yellow-400 text-[10px] font-medium px-1.5 py-0.5">
                            PROMO
                          </span>
                        )}
                      </p>
                      {v.price_summary && (
                        <p className="text-xs text-zinc-400 mt-0.5">
                          Atualizado em{' '}
                          {new Date(v.price_summary.last_updated).toLocaleDateString('pt-BR')}
                        </p>
                      )}
                    </div>
                    {v.price_summary ? (
                      <div className="text-right shrink-0">
                        <p className="text-base font-bold text-violet-600 dark:text-violet-400">
                          {formatBRL(v.price_summary.avg_brl)}
                        </p>
                        <p className="text-xs text-zinc-400">
                          {formatBRL(v.price_summary.min_brl)} – {formatBRL(v.price_summary.max_brl)}
                        </p>
                      </div>
                    ) : (
                      <p className="text-sm text-zinc-400 shrink-0">Sem preço</p>
                    )}
                  </div>
                ))}
              </div>
            </div>
          </div>
        </div>
      </main>
    </div>
  )
}
