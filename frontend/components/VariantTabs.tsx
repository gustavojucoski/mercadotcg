'use client'

import { useState } from 'react'
import type { CardVariantDetail } from '@/lib/types'
import { useLang } from '@/lib/locale'

const FINISH_LABEL: Record<string, string> = {
  normal: 'Normal',
  holo: 'Holo',
  reverse_holo: 'Reverse Holo',
  cosmos_holo: 'Cosmos Holo',
  galaxy_holo: 'Galaxy Holo',
  textured: 'Textured',
  gold_etched: 'Gold Etched',
  master_ball_mirror: 'Master Ball Mirror',
  poke_ball_mirror: 'Poke Ball Mirror',
  first_edition: '1ª Edição',
  shadowless: 'Shadowless',
  unlimited: 'Unlimited',
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

interface VariantTabsProps {
  variants: CardVariantDetail[]
  imageSrc: string
  imageSrcPt?: string
  imageAlt: string
}

export function VariantTabs({ variants, imageSrc, imageSrcPt, imageAlt }: VariantTabsProps) {
  const [activeIdx, setActiveIdx] = useState(0)
  const { lang } = useLang()
  const active = variants[activeIdx]
  const resolvedImageSrc = lang === 'pt' && imageSrcPt && imageSrcPt.length > 0
    ? imageSrcPt
    : imageSrc

  return (
    <div>
      <div className="sticky top-6">
        <div className="flex justify-center mb-4">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={resolvedImageSrc}
            alt={imageAlt}
            className="max-h-96 w-auto rounded-xl shadow-lg object-contain"
          />
        </div>

        {variants.length > 1 && (
          <div className="flex flex-wrap justify-center gap-2 mt-4">
            {variants.map((v, i) => (
              <button
                key={v.id}
                onClick={() => setActiveIdx(i)}
                className={`rounded-full px-3 py-1 text-xs font-medium border transition-colors ${
                  i === activeIdx
                    ? 'bg-violet-600 border-violet-600 text-white'
                    : 'border-zinc-200 dark:border-zinc-700 text-zinc-600 dark:text-zinc-300 hover:border-violet-400'
                }`}
              >
                {finishLabel(v.finish, v.label)}
              </button>
            ))}
          </div>
        )}

        {active && (
          <div className="mt-6 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-4">
            <h3 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-3">
              {finishLabel(active.finish, active.label)}
              {active.is_promo && (
                <span className="ml-2 inline-block rounded bg-yellow-100 dark:bg-yellow-950/40 text-yellow-700 dark:text-yellow-400 text-[10px] font-medium px-1.5 py-0.5">
                  PROMO
                </span>
              )}
            </h3>
            {active.price_summary ? (
              <div className="grid grid-cols-3 gap-3 text-center">
                <div>
                  <p className="text-xs text-zinc-400 mb-1">Mínimo</p>
                  <p className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
                    {formatBRL(active.price_summary.min_brl)}
                  </p>
                </div>
                <div>
                  <p className="text-xs text-zinc-400 mb-1">Média</p>
                  <p className="text-sm font-semibold text-violet-600 dark:text-violet-400">
                    {formatBRL(active.price_summary.avg_brl)}
                  </p>
                </div>
                <div>
                  <p className="text-xs text-zinc-400 mb-1">Máximo</p>
                  <p className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
                    {formatBRL(active.price_summary.max_brl)}
                  </p>
                </div>
                <p className="col-span-3 text-xs text-zinc-400 mt-1">
                  Atualizado em{' '}
                  {new Date(active.price_summary.last_updated).toLocaleDateString('pt-BR')}
                </p>
              </div>
            ) : (
              <p className="text-sm text-zinc-400">Sem preço disponível</p>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
