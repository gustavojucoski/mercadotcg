'use client'

import { useEffect, useRef, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import Link from 'next/link'
import {
  fetchAdminSet,
  patchAdminSet,
  uploadSetImage,
  deleteAdminSet,
  createAdminCard,
  ConflictDeleteError,
  CatalogSet,
} from '@/lib/catalog-admin'

// Public endpoint for cards-in-set is fetched without auth
const API_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080'

// ── Types ──────────────────────────────────────────────────────────────────

interface PublicCard {
  id: string
  collector_number: string
  name: string
  name_pt?: string
  rarity?: string
  supertype?: string
  image_small_url?: string
}

// ── Style tokens ───────────────────────────────────────────────────────────

const inputCls =
  'w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500'
const labelCls = 'block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1.5'
const sectionCls = 'rounded-xl border border-zinc-200 dark:border-zinc-800 p-5 space-y-4'
const sectionTitleCls = 'text-xs font-semibold uppercase tracking-widest text-zinc-400'
const btnPrimary =
  'rounded-lg bg-violet-600 px-4 py-2 text-sm font-semibold text-white hover:bg-violet-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors'
const btnSecondary =
  'rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-4 py-2 text-sm font-medium text-zinc-700 dark:text-zinc-200 hover:bg-zinc-50 dark:hover:bg-zinc-700 disabled:opacity-50 transition-colors'
const btnDanger =
  'rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors'
const alertError =
  'rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400'
const alertSuccess =
  'rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-700 dark:border-green-900/50 dark:bg-green-950/30 dark:text-green-400'

// ── ConfirmDeleteModal ─────────────────────────────────────────────────────

interface ConfirmDeleteModalProps {
  title: string
  description: string
  confirmValue: string
  confirmLabel?: string
  deleting: boolean
  error: string
  blockedBy?: Record<string, number>
  onConfirm: () => void
  onClose: () => void
}

function ConfirmDeleteModal({
  title,
  description,
  confirmValue,
  confirmLabel,
  deleting,
  error,
  blockedBy,
  onConfirm,
  onClose,
}: ConfirmDeleteModalProps) {
  const [input, setInput] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  const matches = input === confirmValue

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div
        className="absolute inset-0 bg-black/50"
        onClick={onClose}
        aria-hidden="true"
      />
      <div className="relative w-full max-w-md rounded-xl bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 p-6 shadow-xl">
        <h2 className="text-base font-semibold text-zinc-900 dark:text-zinc-50 mb-2">{title}</h2>
        <p className="text-sm text-zinc-500 mb-4">{description}</p>

        {blockedBy && Object.keys(blockedBy).length > 0 && (
          <div className="rounded-lg border border-amber-200 bg-amber-50 dark:border-amber-900/50 dark:bg-amber-950/30 px-4 py-3 text-sm text-amber-700 dark:text-amber-400 mb-4">
            <p className="font-medium mb-1">Nao e possivel deletar — registros dependentes:</p>
            <ul className="list-disc list-inside space-y-0.5 text-xs">
              {Object.entries(blockedBy).map(([entity, count]) => (
                <li key={entity}>
                  {entity}: {count} registro(s)
                </li>
              ))}
            </ul>
          </div>
        )}

        {(!blockedBy || Object.keys(blockedBy).length === 0) && (
          <>
            <p className="text-sm text-zinc-600 dark:text-zinc-400 mb-2">
              Digite{' '}
              <code className="rounded bg-zinc-100 dark:bg-zinc-800 px-1.5 py-0.5 text-xs font-mono font-semibold text-zinc-800 dark:text-zinc-200">
                {confirmValue}
              </code>{' '}
              {confirmLabel ?? 'para confirmar'}:
            </p>
            <input
              ref={inputRef}
              type="text"
              value={input}
              onChange={e => setInput(e.target.value)}
              className={`${inputCls} mb-4 font-mono`}
              placeholder={confirmValue}
              autoComplete="off"
            />
          </>
        )}

        {error && <div className={`${alertError} mb-4`}>{error}</div>}

        <div className="flex items-center gap-3 justify-end">
          <button type="button" onClick={onClose} className={btnSecondary}>
            Cancelar
          </button>
          {(!blockedBy || Object.keys(blockedBy).length === 0) && (
            <button
              type="button"
              onClick={onConfirm}
              disabled={!matches || deleting}
              className={btnDanger}
            >
              {deleting ? 'Deletando...' : 'Deletar'}
            </button>
          )}
        </div>
      </div>
    </div>
  )
}

// ── CreateCardModal ────────────────────────────────────────────────────────

interface CreateCardModalProps {
  setId: string
  onCreated: (card: PublicCard) => void
  onClose: () => void
}

function CreateCardModal({ setId, onCreated, onClose }: CreateCardModalProps) {
  const [form, setFormState] = useState({
    collector_number: '',
    name: '',
    name_pt: '',
    rarity: '',
    supertype: '',
    subtypes: '',
    types: '',
    hp: '',
    illustrator: '',
  })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  function setField<K extends keyof typeof form>(key: K, value: string) {
    setFormState(prev => ({ ...prev, [key]: value }))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError('')
    try {
      const created = await createAdminCard({
        set_id: setId,
        collector_number: form.collector_number.trim(),
        name: form.name.trim(),
        name_pt: form.name_pt.trim() || undefined,
        rarity: form.rarity.trim() || undefined,
        supertype: form.supertype.trim() || undefined,
        subtypes: form.subtypes
          ? form.subtypes.split(',').map(s => s.trim()).filter(Boolean)
          : undefined,
        types: form.types
          ? form.types.split(',').map(s => s.trim()).filter(Boolean)
          : undefined,
        hp: form.hp ? Number(form.hp) : undefined,
        illustrator: form.illustrator.trim() || undefined,
      })
      onCreated({
        id: created.id,
        collector_number: created.collector_number,
        name: created.name,
        name_pt: created.name_pt,
        rarity: created.rarity,
        supertype: created.supertype,
        image_small_url: created.image_small_url,
      })
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Erro ao criar carta')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div
        className="absolute inset-0 bg-black/50"
        onClick={onClose}
        aria-hidden="true"
      />
      <div className="relative w-full max-w-lg rounded-xl bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 p-6 shadow-xl max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between mb-5">
          <h2 className="text-base font-semibold text-zinc-900 dark:text-zinc-50">Criar carta</h2>
          <button
            type="button"
            onClick={onClose}
            className="text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-200 transition-colors"
          >
            <svg className="size-5" fill="none" viewBox="0 0 24 24" strokeWidth={2} stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" d="M6 18 18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className={labelCls}>
                Numero coletor <span className="text-red-500">*</span>
              </label>
              <input
                type="text"
                required
                value={form.collector_number}
                onChange={e => setField('collector_number', e.target.value)}
                placeholder="ex: 001"
                className={`${inputCls} font-mono`}
              />
            </div>
            <div>
              <label className={labelCls}>
                Nome <span className="text-red-500">*</span>
              </label>
              <input
                type="text"
                required
                value={form.name}
                onChange={e => setField('name', e.target.value)}
                placeholder="Pikachu"
                className={inputCls}
              />
            </div>
          </div>

          <div>
            <label className={labelCls}>Nome PT</label>
            <input
              type="text"
              value={form.name_pt}
              onChange={e => setField('name_pt', e.target.value)}
              placeholder="Nome em portugues"
              className={inputCls}
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className={labelCls}>Raridade</label>
              <input
                type="text"
                value={form.rarity}
                onChange={e => setField('rarity', e.target.value)}
                placeholder="Rare Holo"
                className={inputCls}
              />
            </div>
            <div>
              <label className={labelCls}>Supertipo</label>
              <input
                type="text"
                value={form.supertype}
                onChange={e => setField('supertype', e.target.value)}
                placeholder="Pokemon"
                className={inputCls}
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className={labelCls}>Subtipos</label>
              <input
                type="text"
                value={form.subtypes}
                onChange={e => setField('subtypes', e.target.value)}
                placeholder="Basic, Stage 1 (separados por virgula)"
                className={inputCls}
              />
            </div>
            <div>
              <label className={labelCls}>Tipos</label>
              <input
                type="text"
                value={form.types}
                onChange={e => setField('types', e.target.value)}
                placeholder="Lightning, Fire (separados por virgula)"
                className={inputCls}
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className={labelCls}>HP</label>
              <input
                type="number"
                min="0"
                value={form.hp}
                onChange={e => setField('hp', e.target.value)}
                placeholder="60"
                className={inputCls}
              />
            </div>
            <div>
              <label className={labelCls}>Ilustrador</label>
              <input
                type="text"
                value={form.illustrator}
                onChange={e => setField('illustrator', e.target.value)}
                placeholder="Mitsuhiro Arita"
                className={inputCls}
              />
            </div>
          </div>

          {error && <div className={alertError}>{error}</div>}

          <div className="flex items-center gap-3 justify-end pt-2 border-t border-zinc-100 dark:border-zinc-800">
            <button type="button" onClick={onClose} className={btnSecondary}>
              Cancelar
            </button>
            <button type="submit" disabled={saving} className={btnPrimary}>
              {saving ? 'Criando...' : 'Criar carta'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

// ── ImageUpload ────────────────────────────────────────────────────────────

interface ImageUploadProps {
  label: string
  currentUrl: string
  slot: 'image' | 'symbol'
  setId: string
  onUploaded: (url: string, slot: 'image' | 'symbol') => void
}

function ImageUploadArea({ label, currentUrl, slot, setId, onUploaded }: ImageUploadProps) {
  const [uploading, setUploading] = useState(false)
  const [preview, setPreview] = useState<string | null>(currentUrl || null)
  const [error, setError] = useState('')

  async function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (!file) return
    setPreview(URL.createObjectURL(file))
    setUploading(true)
    setError('')
    try {
      const result = await uploadSetImage(setId, file, slot)
      const url = slot === 'image' ? result.image_url : result.symbol_url
      if (url) {
        setPreview(url)
        onUploaded(url, slot)
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Erro ao enviar imagem')
      setPreview(currentUrl || null)
    } finally {
      setUploading(false)
    }
  }

  return (
    <div>
      <p className={`${labelCls} mb-2`}>{label}</p>
      <div className="flex items-start gap-4">
        {preview ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={preview}
            alt={label}
            className="w-20 h-20 rounded-lg border border-zinc-200 dark:border-zinc-700 object-contain bg-zinc-50 dark:bg-zinc-800 flex-shrink-0"
          />
        ) : (
          <div className="w-20 h-20 rounded-lg border-2 border-dashed border-zinc-300 dark:border-zinc-700 flex items-center justify-center text-zinc-400 text-xs flex-shrink-0">
            Vazio
          </div>
        )}
        <div className="pt-1">
          <label className="cursor-pointer text-sm text-violet-600 hover:text-violet-700 dark:text-violet-400 font-medium">
            {uploading ? 'Enviando...' : preview ? 'Alterar' : 'Enviar'}
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp,image/gif"
              onChange={handleChange}
              disabled={uploading}
              className="sr-only"
            />
          </label>
          <p className="text-xs text-zinc-400 mt-1">JPG, PNG, WebP</p>
          {error && <p className="text-xs text-red-500 mt-1">{error}</p>}
        </div>
      </div>
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────

export default function EditSetPage() {
  const { id } = useParams<{ id: string }>()
  const router = useRouter()

  const [set, setSet] = useState<CatalogSet | null>(null)
  const [loadError, setLoadError] = useState('')

  // Edit form state
  const [name, setName] = useState('')
  const [namePt, setNamePt] = useState('')
  const [nameEn, setNameEn] = useState('')
  const [seriesId, setSeriesId] = useState('')
  const [releaseDate, setReleaseDate] = useState('')
  const [totalCards, setTotalCards] = useState('')
  const [printedTotal, setPrintedTotal] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [saveSuccess, setSaveSuccess] = useState('')

  // Series picker
  const [seriesList, setSeriesList] = useState<Array<{id: string; name: string; name_pt: string}>>([])

  // Cards table
  const [cards, setCards] = useState<PublicCard[]>([])
  const [cardsLoading, setCardsLoading] = useState(false)
  const [cardsError, setCardsError] = useState('')

  // Modals
  const [showCreateCard, setShowCreateCard] = useState(false)
  const [showDeleteSet, setShowDeleteSet] = useState(false)
  const [deleteError, setDeleteError] = useState('')
  const [deleteBlockedBy, setDeleteBlockedBy] = useState<Record<string, number> | undefined>()
  const [deleting, setDeleting] = useState(false)

  // Load set
  useEffect(() => {
    fetchAdminSet(id)
      .then(s => {
        setSet(s)
        setName(s.name)
        setNamePt(s.name_pt || '')
        setNameEn(s.name_en || '')
        setSeriesId(s.series_id || '')
        setReleaseDate(s.release_date ? s.release_date.slice(0, 10) : '')
        setTotalCards(s.total_cards ? String(s.total_cards) : '')
        setPrintedTotal(s.printed_total ? String(s.printed_total) : '')
      })
      .catch(e => setLoadError(e.message))
  }, [id])

  // Load series list for picker
  useEffect(() => {
    if (!set) return
    fetch(`${API_URL}/api/v1/series?tcg=${encodeURIComponent(set.tcg)}`)
      .then(r => r.json())
      .then((data: Array<{id: string; name: string; name_pt: string}>) => setSeriesList(data))
      .catch(() => {})
  }, [set])

  // Load cards (public endpoint)
  useEffect(() => {
    if (!set) return
    setCardsLoading(true)
    setCardsError('')
    fetch(
      `${API_URL}/api/v1/sets/${encodeURIComponent(set.tcg)}/${encodeURIComponent(set.code)}/cards?page=1&limit=300`,
    )
      .then(res => {
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        return res.json()
      })
      .then(data => {
        setCards((data.cards as PublicCard[]) ?? [])
      })
      .catch(e => setCardsError(e.message))
      .finally(() => setCardsLoading(false))
  }, [set])

  async function handleSave(e: React.FormEvent) {
    e.preventDefault()
    if (!set) return
    if (!name.trim()) {
      setSaveError('Nome é obrigatório')
      return
    }
    setSaving(true)
    setSaveError('')
    setSaveSuccess('')
    try {
      const updated = await patchAdminSet(set.id, {
        name: name.trim(),
        name_pt: namePt.trim() || undefined,
        name_en: nameEn.trim() || undefined,
        series_id: seriesId.trim() || undefined,
        release_date: releaseDate || undefined,
        total_cards: totalCards ? Number(totalCards) : undefined,
        printed_total: printedTotal ? Number(printedTotal) : undefined,
      })
      setSet(updated)
      setSaveSuccess('Set atualizado com sucesso.')
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Erro ao salvar')
    } finally {
      setSaving(false)
    }
  }

  function handleImageUploaded(url: string, slot: 'image' | 'symbol') {
    if (!set) return
    setSet(prev => prev ? { ...prev, [slot === 'image' ? 'image_url' : 'symbol_url']: url } : prev)
  }

  async function handleDeleteSet() {
    if (!set) return
    setDeleting(true)
    setDeleteError('')
    setDeleteBlockedBy(undefined)
    try {
      await deleteAdminSet(set.id, set.code)
      router.push(`/admin/catalogo/sets?tcg=${set.tcg}`)
    } catch (e) {
      if (e instanceof ConflictDeleteError) {
        setDeleteBlockedBy(e.blockedBy)
        setDeleteError(e.message)
      } else {
        setDeleteError(e instanceof Error ? e.message : 'Erro ao deletar')
      }
    } finally {
      setDeleting(false)
    }
  }

  if (loadError) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-6">
        <div className={alertError}>{loadError}</div>
        <Link href="/admin/catalogo/sets?tcg=pokemon" className="mt-3 inline-block text-sm text-violet-600 hover:underline">
          Voltar para sets
        </Link>
      </div>
    )
  }

  if (!set) {
    return (
      <div className="min-h-[60vh] flex items-center justify-center">
        <div className="animate-pulse text-zinc-400 text-sm">Carregando set...</div>
      </div>
    )
  }

  return (
    <>
      <div className="mx-auto max-w-4xl px-4 py-6">
        {/* Breadcrumb + header */}
        <div className="mb-6">
          <div className="flex items-center gap-2 text-sm text-zinc-500 mb-1">
            <Link href="/admin/catalogo" className="hover:text-zinc-900 dark:hover:text-zinc-100">
              Catalogo
            </Link>
            <span>/</span>
            <Link
              href={`/admin/catalogo/sets?tcg=${set.tcg}`}
              className="hover:text-zinc-900 dark:hover:text-zinc-100"
            >
              Sets
            </Link>
            <span>/</span>
            <span className="text-zinc-900 dark:text-zinc-50 font-mono truncate max-w-[160px]">
              {set.code}
            </span>
          </div>
          <div className="flex items-start justify-between gap-4">
            <div>
              <h1 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50">{set.name}</h1>
              <p className="text-xs text-zinc-400 mt-0.5">
                {set.tcg} · {set.code}
                {set.series && <span className="font-normal"> · {set.series}</span>}
              </p>
            </div>
            <button
              type="button"
              onClick={() => { setDeleteError(''); setDeleteBlockedBy(undefined); setShowDeleteSet(true) }}
              className={btnDanger}
            >
              Deletar set
            </button>
          </div>
        </div>

        <div className="space-y-6">
          {/* Edit form */}
          <form onSubmit={handleSave}>
            <div className={sectionCls}>
              <h2 className={sectionTitleCls}>Dados do set</h2>
              <div className="grid grid-cols-2 gap-4">
                <div className="col-span-2">
                  <label className={labelCls}>Nome</label>
                  <input
                    type="text"
                    value={name}
                    onChange={e => setName(e.target.value)}
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className={labelCls}>Nome PT</label>
                  <input
                    type="text"
                    value={namePt}
                    onChange={e => setNamePt(e.target.value)}
                    className={inputCls}
                    placeholder="Nome em portugues"
                  />
                </div>
                <div>
                  <label className={labelCls}>Nome EN alternativo</label>
                  <input
                    type="text"
                    value={nameEn}
                    onChange={e => setNameEn(e.target.value)}
                    className={inputCls}
                    placeholder="Mesmo que name se vazio"
                  />
                </div>
                <div>
                  <label className={labelCls}>Série</label>
                  <select
                    value={seriesId}
                    onChange={e => setSeriesId(e.target.value)}
                    className={inputCls}
                  >
                    <option value="">— Sem série —</option>
                    {seriesList.map(s => (
                      <option key={s.id} value={s.id}>
                        {s.name_pt ? `${s.name} / ${s.name_pt}` : s.name}
                      </option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className={labelCls}>Data de lancamento</label>
                  <input
                    type="date"
                    value={releaseDate}
                    onChange={e => setReleaseDate(e.target.value)}
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className={labelCls}>Total de cartas</label>
                  <input
                    type="number"
                    min="0"
                    value={totalCards}
                    onChange={e => setTotalCards(e.target.value)}
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className={labelCls}>Total impresso</label>
                  <input
                    type="number"
                    min="0"
                    value={printedTotal}
                    onChange={e => setPrintedTotal(e.target.value)}
                    className={inputCls}
                  />
                </div>
              </div>

              {saveError && <div className={alertError}>{saveError}</div>}
              {saveSuccess && <div className={alertSuccess}>{saveSuccess}</div>}

              <div>
                <button type="submit" disabled={saving} className={btnPrimary}>
                  {saving ? 'Salvando...' : 'Salvar alteracoes'}
                </button>
              </div>
            </div>
          </form>

          {/* Images */}
          <div className={sectionCls}>
            <h2 className={sectionTitleCls}>Imagens</h2>
            <div className="grid grid-cols-2 gap-6">
              <ImageUploadArea
                label="Imagem do set"
                currentUrl={set.image_url}
                slot="image"
                setId={set.id}
                onUploaded={handleImageUploaded}
              />
              <ImageUploadArea
                label="Simbolo do set"
                currentUrl={set.symbol_url}
                slot="symbol"
                setId={set.id}
                onUploaded={handleImageUploaded}
              />
            </div>
          </div>

          {/* Cards table */}
          <div className={sectionCls}>
            <div className="flex items-center justify-between">
              <h2 className={sectionTitleCls}>
                Cartas ({cards.length})
              </h2>
              <button
                type="button"
                onClick={() => setShowCreateCard(true)}
                className={btnPrimary}
              >
                + Criar carta
              </button>
            </div>

            {cardsLoading && (
              <p className="text-sm text-zinc-400 py-4 text-center">Carregando cartas...</p>
            )}
            {cardsError && <div className={alertError}>{cardsError}</div>}

            {!cardsLoading && cards.length === 0 && !cardsError && (
              <p className="text-sm text-zinc-400 text-center py-6">
                Nenhuma carta neste set ainda.
              </p>
            )}

            {cards.length > 0 && (
              <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 overflow-hidden">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="bg-zinc-50 dark:bg-zinc-900 border-b border-zinc-200 dark:border-zinc-800">
                      <th className="px-4 py-3 text-left font-medium text-zinc-500">#</th>
                      <th className="px-4 py-3 text-left font-medium text-zinc-500">Nome</th>
                      <th className="px-4 py-3 text-left font-medium text-zinc-500">Nome PT</th>
                      <th className="px-4 py-3 text-left font-medium text-zinc-500">Raridade</th>
                      <th className="px-4 py-3 text-right font-medium text-zinc-500">Acoes</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-zinc-100 dark:divide-zinc-800">
                    {cards.map(c => (
                      <tr
                        key={c.id}
                        className="bg-white dark:bg-zinc-900 hover:bg-zinc-50 dark:hover:bg-zinc-800/50 transition-colors"
                      >
                        <td className="px-4 py-2.5 font-mono text-xs text-zinc-500">
                          {c.collector_number}
                        </td>
                        <td className="px-4 py-2.5 text-zinc-900 dark:text-zinc-100 font-medium">
                          {c.name}
                        </td>
                        <td className="px-4 py-2.5 text-zinc-500">
                          {c.name_pt || <span className="text-zinc-300 dark:text-zinc-600 italic">—</span>}
                        </td>
                        <td className="px-4 py-2.5 text-zinc-500 text-xs">
                          {c.rarity || '—'}
                        </td>
                        <td className="px-4 py-2.5 text-right">
                          <Link
                            href={`/admin/catalogo/cards/${c.id}`}
                            className="text-xs text-violet-600 hover:text-violet-700 dark:text-violet-400 font-medium"
                          >
                            Editar
                          </Link>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Modals */}
      {showCreateCard && (
        <CreateCardModal
          setId={set.id}
          onCreated={card => {
            setCards(prev => [...prev, card])
            setShowCreateCard(false)
          }}
          onClose={() => setShowCreateCard(false)}
        />
      )}

      {showDeleteSet && (
        <ConfirmDeleteModal
          title="Deletar set"
          description={`Esta acao e irreversivel e deletara o set "${set.name}" e todos os seus dados.`}
          confirmValue={set.code}
          confirmLabel="para confirmar a exclusao"
          deleting={deleting}
          error={deleteError}
          blockedBy={deleteBlockedBy}
          onConfirm={handleDeleteSet}
          onClose={() => setShowDeleteSet(false)}
        />
      )}
    </>
  )
}
