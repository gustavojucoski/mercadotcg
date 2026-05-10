'use client'

import { useState, useRef, useEffect } from 'react'
import { useSets } from '@/lib/sets'
import { PokemonSet } from '@/lib/types'

interface Props {
  value: string
  onChange: (code: string) => void
}

export function SetCombobox({ value, onChange }: Props) {
  const { sets, loading } = useSets()
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const selected = sets.find(s => s.ptcgoCode === value)

  const filtered = query.trim() === ''
    ? sets
    : sets.filter(s =>
        s.name.toLowerCase().includes(query.toLowerCase()) ||
        s.ptcgoCode.toLowerCase().includes(query.toLowerCase())
      )

  useEffect(() => {
    function onMouseDown(e: MouseEvent) {
      if (!containerRef.current?.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onMouseDown)
    return () => document.removeEventListener('mousedown', onMouseDown)
  }, [])

  function select(set: PokemonSet) {
    onChange(set.ptcgoCode)
    setQuery('')
    setOpen(false)
  }

  function handleButtonClick() {
    setOpen(prev => !prev)
    if (!open) setTimeout(() => inputRef.current?.focus(), 0)
  }

  return (
    <div ref={containerRef} className="relative w-full sm:w-72">
      <button
        type="button"
        onClick={handleButtonClick}
        className="flex w-full items-center justify-between gap-2 rounded-lg border border-zinc-300 bg-white px-3 py-2 text-sm text-zinc-800 shadow-xs transition hover:border-zinc-400 focus:outline-none dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100 dark:hover:border-zinc-500"
      >
        <span className="truncate">
          {selected
            ? `${selected.name} (${selected.ptcgoCode})`
            : loading
              ? 'Carregando sets...'
              : 'Selecione um set...'}
        </span>
        <svg className="size-4 shrink-0 text-zinc-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {open && (
        <div className="absolute z-20 mt-1 w-full rounded-lg border border-zinc-200 bg-white shadow-lg dark:border-zinc-700 dark:bg-zinc-900">
          <div className="border-b border-zinc-100 p-2 dark:border-zinc-800">
            <input
              ref={inputRef}
              type="text"
              value={query}
              onChange={e => setQuery(e.target.value)}
              placeholder="Filtrar por nome ou código..."
              className="w-full rounded border border-zinc-200 bg-zinc-50 px-2 py-1.5 text-sm text-zinc-800 placeholder-zinc-400 outline-none focus:border-zinc-400 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-100"
            />
          </div>
          <ul className="max-h-64 overflow-y-auto py-1">
            {filtered.length === 0 && (
              <li className="px-3 py-2 text-sm text-zinc-400">Nenhum resultado</li>
            )}
            {filtered.map(s => (
              <li key={s.id}>
                <button
                  type="button"
                  onClick={() => select(s)}
                  className={`flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition hover:bg-zinc-50 dark:hover:bg-zinc-800 ${
                    s.ptcgoCode === value ? 'bg-zinc-50 font-medium dark:bg-zinc-800' : ''
                  }`}
                >
                  <span className="flex-1 truncate text-zinc-800 dark:text-zinc-100">{s.name}</span>
                  <span className="shrink-0 rounded bg-zinc-100 px-1.5 py-0.5 text-xs font-mono text-zinc-500 dark:bg-zinc-700 dark:text-zinc-400">
                    {s.ptcgoCode}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  )
}
