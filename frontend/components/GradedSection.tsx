import { PriceResult, SourceResult } from '@/lib/types'

function formatPrice(result: PriceResult): string {
  const n = parseFloat(result.price)
  return n.toLocaleString('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
}

const CURRENCY_SYMBOLS: Record<string, string> = {
  BRL: 'R$',
  USD: 'US$',
  EUR: '€',
  JPY: '¥',
}

interface Props {
  sources: SourceResult[]
}

export function GradedSection({ sources }: Props) {
  const ebaySource = sources.find(s => s.source === 'ebay')
  if (!ebaySource) return null

  const graded = ebaySource.results.filter(r => r.condition === 'GRADED')
  if (graded.length === 0 && !ebaySource.error) return null

  return (
    <div className="rounded-xl border border-zinc-200 bg-white overflow-hidden dark:border-zinc-800 dark:bg-zinc-900">
      <div className="flex items-center justify-between border-b border-zinc-100 px-5 py-3 dark:border-zinc-800">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
          Vendas Gradeadas
        </h3>
        <span className="text-xs text-zinc-400">eBay via Scrydex</span>
      </div>

      {ebaySource.error ? (
        <p className="px-5 py-4 text-sm text-red-500">{ebaySource.error}</p>
      ) : graded.length === 0 ? (
        <p className="px-5 py-4 text-sm text-zinc-400">Sem vendas gradeadas encontradas.</p>
      ) : (
        <div className="grid grid-cols-2 gap-px bg-zinc-100 sm:grid-cols-3 lg:grid-cols-4 dark:bg-zinc-800">
          {graded.map((item, i) => (
            <a
              key={i}
              href={item.url}
              target="_blank"
              rel="noopener noreferrer"
              className="flex flex-col gap-1 bg-white px-4 py-3 transition hover:bg-zinc-50 dark:bg-zinc-900 dark:hover:bg-zinc-800"
            >
              <span className="text-xs font-semibold text-purple-600 dark:text-purple-400">{item.title}</span>
              <span className="text-lg font-bold text-zinc-900 dark:text-zinc-50">
                {CURRENCY_SYMBOLS[item.currency] ?? item.currency}{' '}{formatPrice(item)}
              </span>
              {item.raw_condition && (
                <span className="text-xs text-zinc-400">{item.raw_condition}</span>
              )}
            </a>
          ))}
        </div>
      )}
    </div>
  )
}
