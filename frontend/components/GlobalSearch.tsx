'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { useRouter } from 'next/navigation'
import { autocompleteCards } from '@/lib/catalog'
import type { AutocompleteItem } from '@/lib/types'
import { useLang } from '@/lib/locale'

function useDebounce<T>(value: T, delay: number): T {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(t)
  }, [value, delay])
  return debounced
}

export function GlobalSearch() {
  const router = useRouter()
  const { t } = useLang()
  const [query, setQuery] = useState('')
  const [items, setItems] = useState<AutocompleteItem[]>([])
  const [loading, setLoading] = useState(false)
  const [open, setOpen] = useState(false)
  const [activeIdx, setActiveIdx] = useState(-1)

  const containerRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const debouncedQuery = useDebounce(query, 300)

  useEffect(() => {
    if (debouncedQuery.length < 2) {
      setItems([])
      setOpen(false)
      return
    }
    let cancelled = false
    setLoading(true)
    autocompleteCards(debouncedQuery).then(results => {
      if (!cancelled) {
        setItems(results)
        setOpen(true)
        setActiveIdx(-1)
        setLoading(false)
      }
    }).catch(() => {
      if (!cancelled) setLoading(false)
    })
    return () => { cancelled = true }
  }, [debouncedQuery])

  useEffect(() => {
    function onMouseDown(e: MouseEvent) {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', onMouseDown)
    return () => document.removeEventListener('mousedown', onMouseDown)
  }, [])

  const selectItem = useCallback((item: AutocompleteItem) => {
    setQuery('')
    setOpen(false)
    setItems([])
    router.push(`/cards/${item.slug}`)
  }, [router])

  function navigateToSearch() {
    const q = query.trim()
    if (!q) return
    setQuery('')
    setOpen(false)
    setItems([])
    router.push(`/search?q=${encodeURIComponent(q)}`)
  }

  function onKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'ArrowDown') {
      if (!open) return
      e.preventDefault()
      setActiveIdx(i => Math.min(i + 1, items.length - 1))
    } else if (e.key === 'ArrowUp') {
      if (!open) return
      e.preventDefault()
      setActiveIdx(i => Math.max(i - 1, -1))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (open && activeIdx >= 0) {
        selectItem(items[activeIdx])
      } else {
        navigateToSearch()
      }
    } else if (e.key === 'Escape') {
      setOpen(false)
    }
  }

  const displayName = (item: AutocompleteItem) => t(item.name, item.name_pt)

  return (
    <div ref={containerRef} className="relative flex-1 max-w-sm">
      <div className="relative">
        <button
          type="button"
          onClick={navigateToSearch}
          className="absolute left-2.5 top-1/2 -translate-y-1/2 text-zinc-400 hover:text-violet-500 transition-colors focus:outline-none"
          aria-label="Buscar"
          tabIndex={-1}
        >
          <svg
            className="size-4"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
            aria-hidden="true"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M21 21l-4.35-4.35m0 0A7.5 7.5 0 104.5 4.5a7.5 7.5 0 0012.15 12.15z"
            />
          </svg>
        </button>
        <input
          ref={inputRef}
          type="search"
          value={query}
          onChange={e => setQuery(e.target.value)}
          onKeyDown={onKeyDown}
          onFocus={() => {
            if (items.length > 0) setOpen(true)
          }}
          placeholder="Buscar cartas..."
          autoComplete="off"
          className="w-full rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 pl-9 pr-3 py-1.5 text-sm text-zinc-900 dark:text-zinc-100 placeholder:text-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500 focus:border-transparent transition-colors"
          aria-label="Buscar cartas"
          aria-autocomplete="list"
          aria-expanded={open}
          aria-haspopup="listbox"
          role="combobox"
          aria-controls="global-search-listbox"
          aria-activedescendant={activeIdx >= 0 ? `gsi-${activeIdx}` : undefined}
        />
        {loading && (
          <div className="absolute right-2.5 top-1/2 -translate-y-1/2">
            <svg
              className="size-4 animate-spin text-zinc-400"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                className="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                strokeWidth="4"
              />
              <path
                className="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8v4l3-3-3-3v4a8 8 0 010 16v-4l-3 3 3 3v-4a8 8 0 01-8-8z"
              />
            </svg>
          </div>
        )}
      </div>

      {open && (
        <ul
          id="global-search-listbox"
          role="listbox"
          className="absolute top-full left-0 right-0 mt-1 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 shadow-xl z-50 py-1 overflow-hidden"
        >
          {items.length === 0 ? (
            <li className="px-4 py-3 text-sm text-zinc-400 text-center select-none">
              Nenhuma carta encontrada
            </li>
          ) : (
            items.map((item, idx) => (
              <li
                key={item.id}
                id={`gsi-${idx}`}
                role="option"
                aria-selected={idx === activeIdx}
                onMouseEnter={() => setActiveIdx(idx)}
                onMouseDown={e => {
                  e.preventDefault()
                  selectItem(item)
                }}
                className={`flex items-center gap-3 px-3 py-2 cursor-pointer transition-colors ${
                  idx === activeIdx
                    ? 'bg-violet-50 dark:bg-violet-950/40'
                    : 'hover:bg-zinc-50 dark:hover:bg-zinc-800'
                }`}
              >
                {item.image_small_url ? (
                  // Using a plain img here because next/image requires configured domains
                  // and autocomplete images come from external CDN (pokemontcg.io).
                  // eslint-disable-next-line @next/next/no-img-element
                  <img
                    src={item.image_small_url}
                    alt=""
                    width={28}
                    height={39}
                    className="rounded shrink-0 object-contain"
                    loading="lazy"
                  />
                ) : (
                  <div className="w-7 h-[39px] rounded bg-zinc-100 dark:bg-zinc-800 shrink-0" />
                )}
                <div className="min-w-0">
                  <p className="text-sm font-medium text-zinc-900 dark:text-zinc-100 truncate">
                    {displayName(item)}
                  </p>
                  <p className="text-xs text-zinc-400 truncate">
                    {item.set_name} &middot; #{item.collector_number}
                  </p>
                </div>
              </li>
            ))
          )}
        </ul>
      )}
    </div>
  )
}
