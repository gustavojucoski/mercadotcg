import { Condition, PriceResult, SourceResult } from '@/lib/types'

const CONDITIONS: Condition[] = ['NM', 'LP', 'MP', 'HP', 'DMG']

const CONDITION_LABELS: Record<Condition, string> = {
  NM: 'Near Mint',
  LP: 'Lightly Played',
  MP: 'Mod. Played',
  HP: 'Heavily Played',
  DMG: 'Damaged',
  GRADED: 'Graded',
}

const SOURCES = [
  { key: 'ligapokemon' as const, label: 'LigaPokemon', currency: 'R$', color: 'text-green-600 dark:text-green-400' },
  { key: 'tcgplayer' as const, label: 'TCGPlayer', currency: 'US$', color: 'text-blue-600 dark:text-blue-400' },
  { key: 'cardmarket' as const, label: 'Cardmarket', currency: '€', color: 'text-purple-600 dark:text-purple-400' },
]

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

function cheapest(results: PriceResult[], condition: Condition): PriceResult | null {
  const matches = results.filter(r => r.condition === condition)
  if (matches.length === 0) return null
  return matches.reduce((a, b) => parseFloat(a.price) <= parseFloat(b.price) ? a : b)
}

interface Props {
  sources: SourceResult[]
}

export function PriceMatrix({ sources }: Props) {
  const bySource = Object.fromEntries(sources.map(s => [s.source, s.results]))

  const hasAnyData = SOURCES.some(src => {
    const results = bySource[src.key] ?? []
    return CONDITIONS.some(cond => cheapest(results, cond) !== null)
  })

  if (!hasAnyData) return null

  return (
    <div className="rounded-xl border border-zinc-200 bg-white overflow-hidden dark:border-zinc-800 dark:bg-zinc-900">
      <div className="border-b border-zinc-100 px-5 py-3 dark:border-zinc-800">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
          Preços por Condição
        </h3>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-zinc-100 dark:border-zinc-800">
              <th className="px-5 py-3 text-left text-xs font-medium text-zinc-400 dark:text-zinc-500">
                Condição
              </th>
              {SOURCES.map(src => (
                <th key={src.key} className={`px-5 py-3 text-right text-xs font-semibold ${src.color}`}>
                  {src.label}
                  <span className="ml-1 font-normal text-zinc-400">({src.currency})</span>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {CONDITIONS.map((cond, i) => (
              <tr
                key={cond}
                className={`border-b border-zinc-50 last:border-0 dark:border-zinc-800/50 ${
                  i % 2 === 0 ? '' : 'bg-zinc-50/50 dark:bg-zinc-800/20'
                }`}
              >
                <td className="px-5 py-3">
                  <div className="flex flex-col">
                    <span className="font-semibold text-zinc-800 dark:text-zinc-200">{cond}</span>
                    <span className="text-xs text-zinc-400">{CONDITION_LABELS[cond]}</span>
                  </div>
                </td>
                {SOURCES.map(src => {
                  const results = bySource[src.key] ?? []
                  const best = cheapest(results, cond)
                  return (
                    <td key={src.key} className="px-5 py-3 text-right">
                      {best ? (
                        <a
                          href={best.url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="font-medium text-zinc-900 transition hover:text-blue-600 dark:text-zinc-100 dark:hover:text-blue-400"
                        >
                          {formatPrice(best)}
                        </a>
                      ) : (
                        <span className="text-zinc-300 dark:text-zinc-600">—</span>
                      )}
                    </td>
                  )
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
