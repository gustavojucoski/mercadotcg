'use client'

import { useState } from 'react'
import type { CardVariantDetail } from '@/lib/types'
import { useLang } from '@/lib/locale'
import { finishLabel } from '@/lib/variants'

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
                {finishLabel(v.finish, v.label, v.is_promo)}
              </button>
            ))}
          </div>
        )}

        {active && (
          <div className="mt-6 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-4">
            <h3 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100 mb-3">
              {finishLabel(active.finish, active.label, active.is_promo)}
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
