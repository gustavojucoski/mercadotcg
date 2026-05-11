import Link from 'next/link'
import type { CardInSet } from '@/lib/types'

interface CardThumbnailProps {
  card: CardInSet
  setCode: string
}

const RARITY_COLOR: Record<string, string> = {
  'Common': 'bg-zinc-100 text-zinc-500 dark:bg-zinc-800 dark:text-zinc-400',
  'Uncommon': 'bg-green-50 text-green-700 dark:bg-green-950/40 dark:text-green-400',
  'Rare': 'bg-blue-50 text-blue-700 dark:bg-blue-950/40 dark:text-blue-400',
  'Double Rare': 'bg-violet-50 text-violet-700 dark:bg-violet-950/40 dark:text-violet-400',
  'Ultra Rare': 'bg-orange-50 text-orange-700 dark:bg-orange-950/40 dark:text-orange-400',
  'Illustration Rare': 'bg-pink-50 text-pink-700 dark:bg-pink-950/40 dark:text-pink-400',
  'Special Illustration Rare': 'bg-pink-50 text-pink-700 dark:bg-pink-950/40 dark:text-pink-400',
  'Hyper Rare': 'bg-yellow-50 text-yellow-700 dark:bg-yellow-950/40 dark:text-yellow-400',
}

function rarityClass(rarity: string): string {
  return RARITY_COLOR[rarity] ?? 'bg-zinc-100 text-zinc-500 dark:bg-zinc-800 dark:text-zinc-400'
}

export function CardThumbnail({ card, setCode }: CardThumbnailProps) {
  const slug = `${setCode}-${card.collector_number}`
  const displayName = card.name_pt && card.name_pt.length > 0 ? card.name_pt : card.name

  return (
    <Link
      href={`/cards/${slug}`}
      className="group flex flex-col rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 overflow-hidden hover:border-violet-300 dark:hover:border-violet-700 hover:shadow-md transition-all"
    >
      <div className="bg-zinc-50 dark:bg-zinc-800/50 flex items-center justify-center p-2 aspect-[2.5/3.5]">
        {card.image_small_url ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={card.image_small_url}
            alt={displayName}
            className="max-h-full max-w-full object-contain rounded group-hover:scale-105 transition-transform duration-200"
            loading="lazy"
          />
        ) : (
          <div className="w-full h-full rounded bg-zinc-200 dark:bg-zinc-700 flex items-center justify-center">
            <span className="text-xs text-zinc-400">{card.collector_number}</span>
          </div>
        )}
      </div>
      <div className="p-2.5">
        <p className="text-xs font-medium text-zinc-900 dark:text-zinc-100 leading-snug truncate group-hover:text-violet-600 dark:group-hover:text-violet-400 transition-colors">
          {displayName}
        </p>
        <p className="text-xs text-zinc-400 mt-0.5">#{card.collector_number}</p>
        {card.rarity && (
          <span className={`inline-block mt-1.5 rounded px-1.5 py-0.5 text-[10px] font-medium ${rarityClass(card.rarity)}`}>
            {card.rarity}
          </span>
        )}
      </div>
    </Link>
  )
}
