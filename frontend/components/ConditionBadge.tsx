import { Condition } from '@/lib/types'

const styles: Record<Condition, string> = {
  NM: 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300',
  LP: 'bg-lime-100 text-lime-800 dark:bg-lime-900/40 dark:text-lime-300',
  MP: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900/40 dark:text-yellow-300',
  HP: 'bg-orange-100 text-orange-800 dark:bg-orange-900/40 dark:text-orange-300',
  DMG: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300',
  GRADED: 'bg-purple-100 text-purple-800 dark:bg-purple-900/40 dark:text-purple-300',
}

const labels: Record<Condition, string> = {
  NM: 'NM',
  LP: 'LP',
  MP: 'MP',
  HP: 'HP',
  DMG: 'DMG',
  GRADED: 'Graded',
}

export function ConditionBadge({ condition }: { condition: Condition }) {
  const cls = styles[condition] ?? 'bg-zinc-100 text-zinc-700'
  return (
    <span className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-semibold ${cls}`}>
      {labels[condition] ?? condition}
    </span>
  )
}
