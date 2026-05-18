'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { searchCard } from '@/lib/api'
import { SearchResult } from '@/lib/types'
import { Modal } from '@/components/Modal'
import { PriceMatrix } from '@/components/PriceMatrix'
import { GradedSection } from '@/components/GradedSection'
import { SourceCard } from '@/components/SourceCard'

interface ExternalPriceSearchModalProps {
  open: boolean
  onClose: () => void
  collectorNumber: string
  setCode: string
  cardName: string
  setName: string
}

export function ExternalPriceSearchModal({
  open,
  onClose,
  collectorNumber,
  setCode,
  cardName,
  setName,
}: ExternalPriceSearchModalProps) {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<SearchResult | null>(null)
  const requestSeq = useRef(0)

  const runSearch = useCallback(() => {
    const seq = ++requestSeq.current
    setLoading(true)
    setError(null)
    setResult(null)

    searchCard(collectorNumber, setCode)
      .then((data) => {
        if (seq !== requestSeq.current) return
        setResult(data)
      })
      .catch((e) => {
        if (seq !== requestSeq.current) return
        setError(e instanceof Error ? e.message : 'Erro desconhecido')
      })
      .finally(() => {
        if (seq === requestSeq.current) setLoading(false)
      })
  }, [collectorNumber, setCode])

  useEffect(() => {
    if (!open) {
      // Invalidate any in-flight request before resetting state
      requestSeq.current++
      setLoading(false)
      setError(null)
      setResult(null)
      return
    }

    runSearch()
    // eslint-disable-next-line react-hooks/exhaustive-deps
    // cardName/setName are display-only — changes don't require a new search
  }, [open, runSearch])

  return (
    <Modal open={open} onClose={onClose} title="Preços Externos">
      <div className="space-y-4">
        <div>
          <p className="text-sm text-zinc-500 dark:text-zinc-400">
            {cardName} — {setName} #{collectorNumber}
          </p>
          <p className="mt-1 text-xs text-zinc-400 dark:text-zinc-500">
            Snapshot externo — não persistido no MercadoTCG
          </p>
        </div>

        {loading && (
          <div className="flex flex-col items-center justify-center gap-3 py-16 text-zinc-400">
            <svg className="size-8 animate-spin" viewBox="0 0 24 24" fill="none">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8z" />
            </svg>
            <span className="text-sm">Buscando preços em todas as plataformas...</span>
          </div>
        )}

        {!loading && error && (
          <div className="rounded-xl border border-red-200 bg-red-50 px-5 py-4 dark:border-red-900/50 dark:bg-red-950/30">
            <p className="text-sm text-red-700 dark:text-red-400">{error}</p>
            <button
              type="button"
              onClick={runSearch}
              className="mt-3 rounded-lg border border-red-300 bg-white px-3 py-1.5 text-xs font-medium text-red-700 transition hover:bg-red-50 dark:border-red-800 dark:bg-zinc-900 dark:text-red-400 dark:hover:bg-red-950/30"
            >
              Tentar novamente
            </button>
          </div>
        )}

        {!loading && !error && result && result.sources.length === 0 && (
          <p className="py-8 text-center text-sm text-zinc-400">
            Nenhum resultado encontrado para esta carta.
          </p>
        )}

        {!loading && result && result.sources.length > 0 && (
          <div className="flex flex-col gap-4">
            <PriceMatrix sources={result.sources} />
            <GradedSection sources={result.sources} />
            <div className="mt-2">
              <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-zinc-400">
                Detalhes por fonte
              </p>
              <div className="flex flex-col gap-2">
                {result.sources.map(src => (
                  <SourceCard key={src.source} source={src} />
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    </Modal>
  )
}
