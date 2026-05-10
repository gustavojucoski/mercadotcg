'use client'

import { useState } from 'react'
import { SetCombobox } from './SetCombobox'

interface Props {
  onSearch: (number: string, set: string) => void
  loading: boolean
}

export function SearchForm({ onSearch, loading }: Props) {
  const [set, setSet] = useState('')
  const [number, setNumber] = useState('')

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!set || !number.trim()) return
    onSearch(number.trim(), set)
  }

  return (
    <form onSubmit={handleSubmit} className="flex flex-wrap items-end gap-3">
      <div className="flex flex-col gap-1.5">
        <label className="text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wide">
          Set
        </label>
        <SetCombobox value={set} onChange={setSet} />
      </div>

      <div className="flex flex-col gap-1.5">
        <label className="text-xs font-medium text-zinc-500 dark:text-zinc-400 uppercase tracking-wide">
          Número
        </label>
        <input
          type="text"
          value={number}
          onChange={e => setNumber(e.target.value)}
          placeholder="ex: 276"
          className="w-28 rounded-lg border border-zinc-300 bg-white px-3 py-2 text-sm text-zinc-800 shadow-xs outline-none transition placeholder-zinc-400 hover:border-zinc-400 focus:border-zinc-500 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100"
        />
      </div>

      <button
        type="submit"
        disabled={loading || !set || !number.trim()}
        className="flex h-9 items-center gap-2 rounded-lg bg-zinc-900 px-4 text-sm font-medium text-white transition hover:bg-zinc-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
      >
        {loading ? (
          <>
            <svg className="size-4 animate-spin" viewBox="0 0 24 24" fill="none">
              <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
              <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8v8z" />
            </svg>
            Buscando...
          </>
        ) : 'Buscar'}
      </button>
    </form>
  )
}
