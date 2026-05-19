'use client'

import { useState } from 'react'
import { useAuth } from '@/components/AuthProvider'
import { ExternalPriceSearchModal } from '@/components/ExternalPriceSearchModal'

interface ExternalPriceSearchButtonProps {
  collectorNumber: string
  setCode: string
  cardName: string
  setName: string
}

export function ExternalPriceSearchButton({
  collectorNumber,
  setCode,
  cardName,
  setName,
}: ExternalPriceSearchButtonProps) {
  const { user } = useAuth()
  const [open, setOpen] = useState(false)

  if (user?.platform_role !== 'platform_admin') return null
  if (!collectorNumber || !setCode) return null

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="flex items-center gap-1.5 rounded-lg border border-zinc-200 bg-white px-3 py-1.5 text-xs font-medium text-zinc-600 transition hover:border-zinc-300 hover:bg-zinc-50 hover:text-zinc-900 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-400 dark:hover:border-zinc-600 dark:hover:bg-zinc-700 dark:hover:text-zinc-200"
      >
        <svg className="size-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-4.35-4.35M17 11A6 6 0 1 1 5 11a6 6 0 0 1 12 0z" />
        </svg>
        Buscar preços externos
      </button>

      <ExternalPriceSearchModal
        open={open}
        onClose={() => setOpen(false)}
        collectorNumber={collectorNumber}
        setCode={setCode}
        cardName={cardName}
        setName={setName}
      />
    </>
  )
}
