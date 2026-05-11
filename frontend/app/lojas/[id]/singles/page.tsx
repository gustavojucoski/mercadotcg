'use client'

import { useCallback, useEffect, useRef, useState } from 'react'
import { useParams } from 'next/navigation'
import {
  listStoreStock,
  registerPurchase,
  registerSale,
  lookupCards,
  getMyRole,
  StockItemWithSignal,
  StockItem,
  CardLookupMatch,
  CardVariant,
  SaleInput,
} from '@/lib/stores-admin'

// ---- constants -------------------------------------------------------------

const CONDITIONS = ['NM', 'LP', 'MP', 'HP', 'DMG'] as const
type StockCondition = typeof CONDITIONS[number]

const LANGUAGES = ['PT', 'EN', 'JP', 'ES', 'DE', 'FR', 'IT', 'KO'] as const
type StockLanguage = typeof LANGUAGES[number]

const CONDITION_LABELS: Record<string, string> = {
  NM: 'Near Mint',
  LP: 'Lightly Played',
  MP: 'Moderately Played',
  HP: 'Heavily Played',
  DMG: 'Damaged',
}

const CONDITION_COLORS: Record<string, string> = {
  NM: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400',
  LP: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
  MP: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400',
  HP: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400',
  DMG: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400',
}

const LANGUAGE_LABELS: Record<string, string> = {
  PT: 'Português',
  EN: 'English',
  JP: '日本語',
  ES: 'Español',
  DE: 'Deutsch',
  FR: 'Français',
  IT: 'Italiano',
  KO: '한국어',
}

// ---- helpers ---------------------------------------------------------------

function formatBRL(value: string | undefined): string {
  if (!value) return '—'
  const num = parseFloat(value)
  if (isNaN(num)) return '—'
  return num.toLocaleString('pt-BR', { style: 'currency', currency: 'BRL' })
}

function variantLabel(v: CardVariant): string {
  if (v.label) return v.label
  const finish = v.finish.replace(/_/g, ' ')
  return finish.charAt(0).toUpperCase() + finish.slice(1)
}

function bestSignalAvg(item: StockItemWithSignal): string | undefined {
  if (!item.signal?.sources?.length) return undefined
  // prefer weighted_avg from any source that has it
  for (const src of item.signal.sources) {
    if (src.weighted_avg_brl) return src.weighted_avg_brl
  }
  return undefined
}

// ---- PurchaseModal ---------------------------------------------------------

interface PurchaseModalProps {
  storeId: string
  onSuccess: () => void
  onClose: () => void
}

function PurchaseModal({ storeId, onSuccess, onClose }: PurchaseModalProps) {
  const [query, setQuery] = useState('')
  const [matches, setMatches] = useState<CardLookupMatch[]>([])
  const [searching, setSearching] = useState(false)
  const [searchError, setSearchError] = useState<string | null>(null)

  const [selectedMatch, setSelectedMatch] = useState<CardLookupMatch | null>(null)
  const [selectedVariant, setSelectedVariant] = useState<CardVariant | null>(null)

  const [condition, setCondition] = useState<StockCondition>('NM')
  const [language, setLanguage] = useState<StockLanguage>('EN')
  const [quantity, setQuantity] = useState('1')
  const [unitCost, setUnitCost] = useState('')
  const [notes, setNotes] = useState('')

  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  const searchTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)

  function handleQueryChange(value: string) {
    setQuery(value)
    setSelectedMatch(null)
    setSelectedVariant(null)
    setMatches([])
    setSearchError(null)

    if (searchTimeout.current) clearTimeout(searchTimeout.current)
    if (value.trim().length < 2) return

    searchTimeout.current = setTimeout(async () => {
      setSearching(true)
      try {
        const resp = await lookupCards(value.trim())
        setMatches(resp.matches ?? [])
      } catch (e) {
        setSearchError(e instanceof Error ? e.message : 'Erro ao buscar')
      } finally {
        setSearching(false)
      }
    }, 350)
  }

  function selectMatch(m: CardLookupMatch) {
    setSelectedMatch(m)
    setMatches([])
    setQuery(`${m.card.name} — ${m.set.name} #${m.card.number}`)
    // auto-select first variant if only one
    if (m.variants.length === 1) {
      setSelectedVariant(m.variants[0].variant)
    } else {
      setSelectedVariant(null)
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!selectedVariant) return
    const qty = parseInt(quantity, 10)
    if (isNaN(qty) || qty <= 0) return
    if (!unitCost || isNaN(parseFloat(unitCost))) return

    setSaving(true)
    setSaveError(null)
    try {
      await registerPurchase(storeId, {
        variant_id: selectedVariant.id,
        condition,
        language,
        quantity: qty,
        unit_cost_brl: parseFloat(unitCost).toFixed(2),
        notes: notes.trim() || undefined,
      })
      onSuccess()
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Erro ao registrar')
    } finally {
      setSaving(false)
    }
  }

  const canSubmit = !!selectedVariant && parseInt(quantity, 10) > 0 && !!unitCost && !saving

  return (
    <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center p-4 bg-black/40 backdrop-blur-sm">
      <div className="w-full max-w-lg bg-white dark:bg-zinc-900 rounded-2xl shadow-xl border border-zinc-200 dark:border-zinc-800">
        <div className="flex items-center justify-between px-5 pt-5 pb-4 border-b border-zinc-100 dark:border-zinc-800">
          <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-50">Registrar compra</h2>
          <button
            onClick={onClose}
            className="text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200 text-xl leading-none"
            aria-label="Fechar"
          >
            ×
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-5 space-y-4">
          {/* Card search */}
          <div className="relative">
            <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">
              Carta
            </label>
            <input
              type="text"
              value={query}
              onChange={e => handleQueryChange(e.target.value)}
              placeholder="Buscar por nome (ex.: Charizard, Pikachu...)"
              autoComplete="off"
              className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500"
            />
            {searching && (
              <p className="mt-1 text-xs text-zinc-400">Buscando...</p>
            )}
            {searchError && (
              <p className="mt-1 text-xs text-red-500">{searchError}</p>
            )}
            {matches.length > 0 && (
              <ul className="absolute z-10 mt-1 w-full max-h-56 overflow-auto rounded-lg border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 shadow-lg text-sm">
                {matches.map(m => (
                  <li key={m.card.id}>
                    <button
                      type="button"
                      onClick={() => selectMatch(m)}
                      className="w-full text-left px-3 py-2.5 hover:bg-zinc-50 dark:hover:bg-zinc-800 flex items-start gap-3"
                    >
                      {m.card.image_small_url && (
                        // eslint-disable-next-line @next/next/no-img-element
                        <img
                          src={m.card.image_small_url}
                          alt={m.card.name}
                          className="w-8 h-11 object-contain shrink-0 rounded"
                        />
                      )}
                      <span className="min-w-0">
                        <span className="font-medium text-zinc-900 dark:text-zinc-100 block truncate">
                          {m.card.name}
                        </span>
                        <span className="text-xs text-zinc-400 block truncate">
                          {m.set.name} · #{m.card.number}
                          {m.card.rarity ? ` · ${m.card.rarity}` : ''}
                        </span>
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          {/* Variant selector */}
          {selectedMatch && selectedMatch.variants.length > 1 && (
            <div>
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">
                Variante
              </label>
              <select
                value={selectedVariant?.id ?? ''}
                onChange={e => {
                  const found = selectedMatch.variants.find(v => v.variant.id === e.target.value)
                  setSelectedVariant(found?.variant ?? null)
                }}
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500"
              >
                <option value="">Selecione a variante</option>
                {selectedMatch.variants.map(({ variant: v }) => (
                  <option key={v.id} value={v.id}>
                    {variantLabel(v)}
                    {v.is_promo ? ' (Promo)' : ''}
                  </option>
                ))}
              </select>
            </div>
          )}

          {/* Condition + Language */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">
                Condição
              </label>
              <select
                value={condition}
                onChange={e => setCondition(e.target.value as StockCondition)}
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500"
              >
                {CONDITIONS.map(c => (
                  <option key={c} value={c}>{c} — {CONDITION_LABELS[c]}</option>
                ))}
              </select>
            </div>
            <div>
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">
                Idioma
              </label>
              <select
                value={language}
                onChange={e => setLanguage(e.target.value as StockLanguage)}
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500"
              >
                {LANGUAGES.map(l => (
                  <option key={l} value={l}>{l} — {LANGUAGE_LABELS[l]}</option>
                ))}
              </select>
            </div>
          </div>

          {/* Quantity + Unit cost */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">
                Quantidade
              </label>
              <input
                type="number" min="1" step="1"
                value={quantity}
                onChange={e => setQuantity(e.target.value)}
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">
                Custo unitário (R$)
              </label>
              <input
                type="number" min="0" step="0.01"
                value={unitCost}
                onChange={e => setUnitCost(e.target.value)}
                placeholder="0,00"
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500"
              />
            </div>
          </div>

          {/* Notes */}
          <div>
            <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">
              Observações <span className="font-normal text-zinc-400">(opcional)</span>
            </label>
            <input
              type="text"
              value={notes}
              onChange={e => setNotes(e.target.value)}
              placeholder="Ex.: lote feira, OLX..."
              className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500"
            />
          </div>

          {saveError && (
            <p className="text-xs text-red-600 dark:text-red-400">{saveError}</p>
          )}

          <div className="flex justify-end gap-3 pt-1">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-zinc-200 dark:border-zinc-700 px-4 py-2 text-sm font-medium text-zinc-600 dark:text-zinc-400 hover:bg-zinc-50 dark:hover:bg-zinc-800 transition-colors"
            >
              Cancelar
            </button>
            <button
              type="submit"
              disabled={!canSubmit}
              className="rounded-lg bg-violet-600 px-5 py-2 text-sm font-semibold text-white hover:bg-violet-700 disabled:opacity-50 transition-colors"
            >
              {saving ? 'Registrando...' : 'Registrar compra'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// ---- SaleModal -------------------------------------------------------------

interface SaleModalProps {
  storeId: string
  item: StockItem
  onSuccess: () => void
  onClose: () => void
}

function SaleModal({ storeId, item, onSuccess, onClose }: SaleModalProps) {
  const displayName =
    item.card_name
      ? `${item.card_name}${item.card_number ? ` #${item.card_number}` : ''}${item.set_name ? ` — ${item.set_name}` : ''}`
      : `${item.variant_id.slice(0, 8)}…`

  const [quantity, setQuantity] = useState('1')
  const [unitPrice, setUnitPrice] = useState('')
  const [notes, setNotes] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    const qty = parseInt(quantity, 10)
    if (isNaN(qty) || qty <= 0) return
    if (!unitPrice || isNaN(parseFloat(unitPrice))) return

    setSaving(true)
    setSaveError(null)
    try {
      const input: SaleInput = {
        quantity: qty,
        unit_price_brl: parseFloat(unitPrice).toFixed(2),
        notes: notes.trim() || undefined,
      }
      await registerSale(storeId, item.id, input)
      onSuccess()
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Erro ao registrar venda')
    } finally {
      setSaving(false)
    }
  }

  const maxQty = item.quantity
  const qty = parseInt(quantity, 10)
  const canSubmit = !isNaN(qty) && qty > 0 && qty <= maxQty && !!unitPrice && !isNaN(parseFloat(unitPrice)) && !saving

  return (
    <div className="fixed inset-0 z-50 flex items-end sm:items-center justify-center p-4 bg-black/40 backdrop-blur-sm">
      <div className="w-full max-w-md bg-white dark:bg-zinc-900 rounded-2xl shadow-xl border border-zinc-200 dark:border-zinc-800">
        <div className="flex items-center justify-between px-5 pt-5 pb-4 border-b border-zinc-100 dark:border-zinc-800">
          <div className="min-w-0">
            <h2 className="text-sm font-semibold text-zinc-900 dark:text-zinc-50">Registrar venda</h2>
            <p className="text-xs text-zinc-400 truncate mt-0.5" title={displayName}>{displayName}</p>
          </div>
          <button
            onClick={onClose}
            className="ml-4 shrink-0 text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200 text-xl leading-none"
            aria-label="Fechar"
          >
            ×
          </button>
        </div>

        <form onSubmit={handleSubmit} className="p-5 space-y-4">
          {/* Condition + Language info (read-only) */}
          <div className="flex gap-2 flex-wrap">
            <span className={`inline-block rounded-md px-2 py-0.5 text-xs font-medium ${CONDITION_COLORS[item.condition] ?? 'bg-zinc-100 text-zinc-600'}`}>
              {item.condition}
            </span>
            <span className="inline-block rounded-md px-2 py-0.5 text-xs font-medium bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400">
              {item.language.toUpperCase()}
            </span>
            {item.grade && (
              <span className="inline-block rounded-md px-2 py-0.5 text-xs font-medium bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-400">
                {item.grade}
              </span>
            )}
            <span className="inline-block rounded-md px-2 py-0.5 text-xs bg-zinc-50 text-zinc-400 dark:bg-zinc-800/50">
              {item.quantity} em estoque
            </span>
          </div>

          {/* Quantity + Unit price */}
          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">
                Quantidade
              </label>
              <input
                type="number"
                min="1"
                max={maxQty}
                step="1"
                value={quantity}
                onChange={e => setQuantity(e.target.value)}
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-amber-500"
              />
              {qty > maxQty && (
                <p className="mt-1 text-xs text-red-500">Máximo: {maxQty}</p>
              )}
            </div>
            <div>
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">
                Preço unitário (R$)
              </label>
              <input
                type="number"
                min="0"
                step="0.01"
                value={unitPrice}
                onChange={e => setUnitPrice(e.target.value)}
                placeholder="0,00"
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-amber-500"
              />
              {item.cost_avg_brl && (
                <p className="mt-1 text-xs text-zinc-400">
                  Custo médio: {formatBRL(item.cost_avg_brl)}
                </p>
              )}
            </div>
          </div>

          {/* Notes */}
          <div>
            <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">
              Observações <span className="font-normal text-zinc-400">(opcional)</span>
            </label>
            <input
              type="text"
              value={notes}
              onChange={e => setNotes(e.target.value)}
              placeholder="Ex.: venda presencial, Mercado Livre..."
              className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-amber-500"
            />
          </div>

          {saveError && (
            <p className="text-xs text-red-600 dark:text-red-400">{saveError}</p>
          )}

          <div className="flex justify-end gap-3 pt-1">
            <button
              type="button"
              onClick={onClose}
              className="rounded-lg border border-zinc-200 dark:border-zinc-700 px-4 py-2 text-sm font-medium text-zinc-600 dark:text-zinc-400 hover:bg-zinc-50 dark:hover:bg-zinc-800 transition-colors"
            >
              Cancelar
            </button>
            <button
              type="submit"
              disabled={!canSubmit}
              className="rounded-lg bg-amber-500 px-5 py-2 text-sm font-semibold text-white hover:bg-amber-600 disabled:opacity-50 transition-colors"
            >
              {saving ? 'Registrando...' : 'Registrar venda'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// ---- Main page -------------------------------------------------------------

export default function SinglesPage() {
  const { id } = useParams<{ id: string }>()

  const [items, setItems] = useState<StockItemWithSignal[]>([])
  const [myRole, setMyRole] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [showModal, setShowModal] = useState(false)
  const [saleItem, setSaleItem] = useState<StockItem | null>(null)

  // filters
  const [filterQuery, setFilterQuery] = useState('')
  const [filterCondition, setFilterCondition] = useState<string>('')
  const [filterLanguage, setFilterLanguage] = useState<string>('')

  const canEdit = myRole === 'admin' || myRole === 'stock_manager'

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [data, role] = await Promise.all([
        listStoreStock(id, { withSignal: true }),
        getMyRole(id),
      ])
      setItems(data ?? [])
      setMyRole(role)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Erro ao carregar estoque')
    } finally {
      setLoading(false)
    }
  }, [id])

  useEffect(() => { load() }, [load])

  // client-side filtering (stock list is small, no need to round-trip)
  const filtered = items.filter(item => {
    if (filterCondition && item.condition !== filterCondition) return false
    if (filterLanguage && item.language.toUpperCase() !== filterLanguage.toUpperCase()) return false
    // variant_id search — in a future iteration this could match card name via a joined fetch
    // for now we filter on variant_id substring or skip text filter
    if (filterQuery) {
      const q = filterQuery.toLowerCase()
      const matchesId = item.variant_id.toLowerCase().includes(q)
      const matchesSku = item.sku?.toLowerCase().includes(q)
      const matchesNotes = item.notes?.toLowerCase().includes(q)
      const matchesName = item.card_name?.toLowerCase().includes(q)
      const matchesSet = item.set_name?.toLowerCase().includes(q)
      if (!matchesId && !matchesSku && !matchesNotes && !matchesName && !matchesSet) return false
    }
    return true
  })

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16 text-zinc-400 text-sm">
        Carregando...
      </div>
    )
  }

  return (
    <>
      {showModal && (
        <PurchaseModal
          storeId={id}
          onClose={() => setShowModal(false)}
          onSuccess={() => { setShowModal(false); load() }}
        />
      )}
      {saleItem && (
        <SaleModal
          storeId={id}
          item={saleItem}
          onClose={() => setSaleItem(null)}
          onSuccess={() => { setSaleItem(null); load() }}
        />
      )}

      <div className="mx-auto max-w-6xl px-4 py-6 space-y-4">

        {/* Header row */}
        <div className="flex items-start justify-between gap-4">
          <div>
            <h1 className="text-base font-semibold text-zinc-900 dark:text-zinc-50">Singles</h1>
            <p className="text-sm text-zinc-500 mt-0.5">
              Cartas individuais em estoque — todos os TCGs.
            </p>
          </div>
          {canEdit && (
            <button
              onClick={() => setShowModal(true)}
              className="shrink-0 rounded-lg bg-violet-600 px-4 py-2 text-sm font-semibold text-white hover:bg-violet-700 transition-colors"
            >
              + Registrar Compra
            </button>
          )}
        </div>

        {error && (
          <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400">
            {error}
          </div>
        )}

        {/* Filters */}
        <div className="flex flex-wrap gap-3 items-center">
          <input
            type="text"
            placeholder="Filtrar por nota, SKU..."
            value={filterQuery}
            onChange={e => setFilterQuery(e.target.value)}
            className="rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500 w-56"
          />
          <select
            value={filterCondition}
            onChange={e => setFilterCondition(e.target.value)}
            className="rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500"
          >
            <option value="">Todas as condições</option>
            {CONDITIONS.map(c => (
              <option key={c} value={c}>{c} — {CONDITION_LABELS[c]}</option>
            ))}
          </select>
          <select
            value={filterLanguage}
            onChange={e => setFilterLanguage(e.target.value)}
            className="rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500"
          >
            <option value="">Todos os idiomas</option>
            {LANGUAGES.map(l => (
              <option key={l} value={l}>{l} — {LANGUAGE_LABELS[l]}</option>
            ))}
          </select>
          {(filterQuery || filterCondition || filterLanguage) && (
            <button
              onClick={() => { setFilterQuery(''); setFilterCondition(''); setFilterLanguage('') }}
              className="text-xs text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200 underline"
            >
              Limpar filtros
            </button>
          )}
        </div>

        {/* Table */}
        {items.length === 0 ? (
          <EmptyState canEdit={canEdit} onRegister={() => setShowModal(true)} />
        ) : filtered.length === 0 ? (
          <div className="py-12 text-center text-sm text-zinc-400">
            Nenhum item encontrado com os filtros aplicados.
          </div>
        ) : (
          <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 overflow-hidden">
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="bg-zinc-50 dark:bg-zinc-900 border-b border-zinc-200 dark:border-zinc-800">
                    <th className="px-4 py-3 text-left font-medium text-zinc-500">Variante</th>
                    <th className="px-4 py-3 text-left font-medium text-zinc-500">Condição</th>
                    <th className="px-4 py-3 text-left font-medium text-zinc-500">Idioma</th>
                    <th className="px-4 py-3 text-right font-medium text-zinc-500">Qtd.</th>
                    <th className="px-4 py-3 text-right font-medium text-zinc-500">Custo médio</th>
                    <th className="px-4 py-3 text-right font-medium text-zinc-500">Preço mercado</th>
                    {canEdit && (
                      <th className="px-4 py-3 text-right font-medium text-zinc-500">Ações</th>
                    )}
                  </tr>
                </thead>
                <tbody className="divide-y divide-zinc-100 dark:divide-zinc-800">
                  {filtered.map(item => {
                    const marketAvg = bestSignalAvg(item)
                    return (
                      <tr
                        key={item.id}
                        className="bg-white dark:bg-zinc-900 hover:bg-zinc-50 dark:hover:bg-zinc-800/60 transition-colors"
                      >
                        <td className="px-4 py-3">
                          <p className="text-sm font-medium text-zinc-900 dark:text-zinc-100 truncate max-w-[200px]" title={item.card_name ?? item.variant_id}>
                            {item.card_name ?? `${item.variant_id.slice(0, 8)}…`}
                          </p>
                          {(item.set_name || item.card_number) && (
                            <p className="text-xs text-zinc-400 mt-0.5 truncate max-w-[200px]">
                              {[item.set_name, item.card_number ? `#${item.card_number}` : undefined, item.variant_label].filter(Boolean).join(' · ')}
                            </p>
                          )}
                          {item.grade && (
                            <span className="mt-0.5 inline-block text-xs bg-zinc-100 dark:bg-zinc-800 text-zinc-500 rounded px-1.5 py-0.5">
                              {item.grade}
                            </span>
                          )}
                          {item.notes && (
                            <p className="text-xs text-zinc-400 mt-0.5 truncate max-w-[200px]">{item.notes}</p>
                          )}
                        </td>
                        <td className="px-4 py-3">
                          <span className={`inline-block rounded-md px-2 py-0.5 text-xs font-medium ${CONDITION_COLORS[item.condition] ?? 'bg-zinc-100 text-zinc-600'}`}>
                            {item.condition}
                          </span>
                        </td>
                        <td className="px-4 py-3 text-zinc-600 dark:text-zinc-400">
                          {item.language.toUpperCase()}
                        </td>
                        <td className="px-4 py-3 text-right">
                          <span className={`font-semibold ${item.quantity === 0 ? 'text-zinc-300 dark:text-zinc-600' : 'text-zinc-900 dark:text-zinc-50'}`}>
                            {item.quantity}
                          </span>
                        </td>
                        <td className="px-4 py-3 text-right text-zinc-600 dark:text-zinc-400">
                          {formatBRL(item.cost_avg_brl)}
                        </td>
                        <td className="px-4 py-3 text-right">
                          {marketAvg ? (
                            <span className="text-violet-600 dark:text-violet-400 font-medium">
                              {formatBRL(marketAvg)}
                            </span>
                          ) : (
                            <span className="text-zinc-300 dark:text-zinc-600">—</span>
                          )}
                        </td>
                        {canEdit && (
                          <td className="px-4 py-3 text-right">
                            <button
                              onClick={() => setSaleItem(item)}
                              disabled={item.quantity === 0}
                              className="rounded-md bg-amber-50 dark:bg-amber-900/20 border border-amber-200 dark:border-amber-800 px-3 py-1 text-xs font-medium text-amber-700 dark:text-amber-400 hover:bg-amber-100 dark:hover:bg-amber-900/40 disabled:opacity-40 disabled:cursor-not-allowed transition-colors"
                            >
                              Vender
                            </button>
                          </td>
                        )}
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
            <div className="px-4 py-2.5 border-t border-zinc-100 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900/50">
              <p className="text-xs text-zinc-400">
                {filtered.length} {filtered.length === 1 ? 'item' : 'itens'}
                {filtered.length !== items.length && ` de ${items.length}`}
              </p>
            </div>
          </div>
        )}
      </div>
    </>
  )
}

// ---- EmptyState ------------------------------------------------------------

interface EmptyStateProps {
  canEdit: boolean
  onRegister: () => void
}

function EmptyState({ canEdit, onRegister }: EmptyStateProps) {
  return (
    <div className="rounded-xl border border-dashed border-zinc-300 dark:border-zinc-700 py-16 text-center">
      <div className="inline-flex items-center justify-center w-12 h-12 rounded-xl bg-zinc-100 dark:bg-zinc-800 mb-4">
        <svg className="w-6 h-6 text-zinc-400" fill="none" viewBox="0 0 24 24" strokeWidth={1.5} stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z" />
        </svg>
      </div>
      <h2 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300 mb-1">
        Estoque vazio
      </h2>
      <p className="text-sm text-zinc-500 max-w-xs mx-auto mb-4">
        Nenhum single registrado ainda. Registre sua primeira compra para começar a controlar o estoque.
      </p>
      {canEdit && (
        <button
          onClick={onRegister}
          className="inline-flex items-center gap-1.5 rounded-lg bg-violet-600 px-4 py-2 text-sm font-semibold text-white hover:bg-violet-700 transition-colors"
        >
          + Registrar Compra
        </button>
      )}
    </div>
  )
}
