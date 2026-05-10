import { CardInfo as CardInfoType } from '@/lib/types'

export function CardInfo({ card }: { card: CardInfoType }) {
  return (
    <div className="flex items-center gap-4 rounded-xl border border-zinc-200 bg-white px-5 py-4 dark:border-zinc-800 dark:bg-zinc-900">
      <div className="min-w-0">
        <h2 className="text-xl font-bold text-zinc-900 dark:text-zinc-50">{card.name}</h2>
        <p className="mt-0.5 text-sm text-zinc-500 dark:text-zinc-400">
          {card.set_name}
          <span className="mx-1.5 text-zinc-300 dark:text-zinc-600">·</span>
          <span className="font-mono">{card.set_code} {card.number}</span>
        </p>
      </div>
    </div>
  )
}
