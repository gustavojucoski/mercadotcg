'use client'

import { useState } from 'react'
import { SearchForm } from '@/components/SearchForm'
import { CardInfo } from '@/components/CardInfo'
import { PriceMatrix } from '@/components/PriceMatrix'
import { GradedSection } from '@/components/GradedSection'
import { SourceCard } from '@/components/SourceCard'
import { searchCard } from '@/lib/api'
import { SearchResult } from '@/lib/types'

export default function AdminPage() {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [result, setResult] = useState<SearchResult | null>(null)

  async function handleSearch(number: string, set: string) {
    setLoading(true)
    setError(null)
    setResult(null)
    try {
      const data = await searchCard(number, set)
      setResult(data)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Erro desconhecido')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="mx-auto max-w-5xl px-4 py-6">
      <div className="mb-6">
        <h1 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50">Busca Externa</h1>
        <p className="text-sm text-zinc-500 mt-1">Consulte preços em tempo real em múltiplas plataformas.</p>
      </div>

      <div className="mb-6">
        <SearchForm onSearch={handleSearch} loading={loading} />
      </div>

      {loading && (
        <div className="flex flex-col items-center justify-center gap-3 py-24 text-zinc-400">
          <svg className="size-8 animate-spin" viewBox="0 0 24 24" fill="none">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8z" />
          </svg>
          <span className="text-sm">Buscando preços em todas as plataformas...</span>
        </div>
      )}

      {!loading && error && (
        <div className="rounded-xl border border-red-200 bg-red-50 px-5 py-4 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400">
          {error}
        </div>
      )}

      {!loading && result && (
        <div className="flex flex-col gap-4">
          {result.card && <CardInfo card={result.card} />}
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

      {!loading && !error && !result && (
        <div className="flex flex-col items-center justify-center gap-2 py-24 text-center">
          <p className="text-zinc-400 text-sm">
            Selecione um set e informe o número da carta para buscar preços.
          </p>
        </div>
      )}
    </div>
  )
}
