'use client'

import { useState } from 'react'
import { PriceResult, SourceResult } from '@/lib/types'
import { ConditionBadge } from './ConditionBadge'

const SOURCE_LABELS: Record<string, string> = {
  ligapokemon: 'LigaPokemon',
  tcgplayer: 'TCGPlayer',
  cardmarket: 'Cardmarket',
  ebay: 'eBay',
}

const SOURCE_COLORS: Record<string, string> = {
  ligapokemon: 'bg-green-500',
  tcgplayer: 'bg-blue-500',
  cardmarket: 'bg-purple-500',
  ebay: 'bg-yellow-500',
}

const CURRENCY_SYMBOLS: Record<string, string> = {
  BRL: 'R$',
  USD: 'US$',
  EUR: '€',
  JPY: '¥',
}

function formatPrice(result: PriceResult): string {
  const sym = CURRENCY_SYMBOLS[result.currency] ?? result.currency
  const n = parseFloat(result.price)
  const formatted = n.toLocaleString('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
  return `${sym} ${formatted}`
}

export function SourceCard({ source }: { source: SourceResult }) {
  const [open, setOpen] = useState(false)
  const label = SOURCE_LABELS[source.source] ?? source.source
  const dot = SOURCE_COLORS[source.source] ?? 'bg-zinc-400'
  const count = source.results.length

  return (
    <div className="rounded-xl border border-zinc-200 bg-white overflow-hidden dark:border-zinc-800 dark:bg-zinc-900">
      <button
        type="button"
        onClick={() => setOpen(o => !o)}
        className="flex w-full items-center gap-3 px-5 py-3.5 text-left transition hover:bg-zinc-50 dark:hover:bg-zinc-800"
      >
        <span className={`size-2 rounded-full shrink-0 ${dot}`} />
        <span className="flex-1 text-sm font-semibold text-zinc-800 dark:text-zinc-200">{label}</span>

        {source.error ? (
          <span className="rounded bg-red-100 px-2 py-0.5 text-xs text-red-600 dark:bg-red-900/30 dark:text-red-400">
            Erro
          </span>
        ) : (
          <span className="text-xs text-zinc-400">
            {count} {count === 1 ? 'resultado' : 'resultados'}
          </span>
        )}

        <svg
          className={`size-4 text-zinc-400 transition-transform ${open ? 'rotate-180' : ''}`}
          fill="none" viewBox="0 0 24 24" stroke="currentColor"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {open && (
        <div className="border-t border-zinc-100 dark:border-zinc-800">
          {source.error ? (
            <p className="px-5 py-4 text-sm text-red-500">{source.error}</p>
          ) : count === 0 ? (
            <p className="px-5 py-4 text-sm text-zinc-400">Sem resultados para esta fonte.</p>
          ) : (
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-zinc-50 dark:border-zinc-800">
                  <th className="px-5 py-2 text-left text-xs font-medium text-zinc-400">Título</th>
                  <th className="px-5 py-2 text-center text-xs font-medium text-zinc-400">Condição</th>
                  <th className="px-5 py-2 text-right text-xs font-medium text-zinc-400">Preço</th>
                  <th className="w-10 px-3 py-2" />
                </tr>
              </thead>
              <tbody>
                {source.results.map((r, i) => (
                  <tr key={i} className="border-b border-zinc-50 last:border-0 dark:border-zinc-800/50">
                    <td className="max-w-[280px] truncate px-5 py-2.5 text-zinc-700 dark:text-zinc-300">
                      {r.title}
                    </td>
                    <td className="px-5 py-2.5 text-center">
                      <ConditionBadge condition={r.condition} />
                    </td>
                    <td className="px-5 py-2.5 text-right font-medium text-zinc-900 dark:text-zinc-100">
                      {formatPrice(r)}
                    </td>
                    <td className="px-3 py-2.5">
                      <a
                        href={r.url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="text-zinc-400 transition hover:text-blue-500"
                        title="Ver listagem"
                      >
                        <svg className="size-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2}
                            d="M10 6H6a2 2 0 00-2 2v10a2 2 0 002 2h10a2 2 0 002-2v-4M14 4h6m0 0v6m0-6L10 14" />
                        </svg>
                      </a>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  )
}
