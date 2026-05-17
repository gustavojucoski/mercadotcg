'use client'

import { useEffect, useState } from 'react'
import { useParams, useRouter } from 'next/navigation'
import Link from 'next/link'
import {
  fetchAdminCard,
  patchAdminCard,
  uploadCardImage,
  deleteAdminCard,
  fetchCardVariants,
  createAdminVariant,
  patchAdminVariant,
  deleteAdminVariant,
  ConflictDeleteError,
  CatalogCard,
  CatalogVariant,
} from '@/lib/catalog-admin'

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
const btnDangerSm =
  'rounded px-2 py-1 text-xs font-medium text-red-600 hover:text-red-700 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20 disabled:opacity-50 transition-colors'
const alertError =
  'rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400'
const alertSuccess =
  'rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-700 dark:border-green-900/50 dark:bg-green-950/30 dark:text-green-400'

// ── ConfirmDeleteModal ─────────────────────────────────────────────────────

interface ConfirmDeleteModalProps {
  title: string
  description: string
  confirmValue: string
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
  deleting,
  error,
  blockedBy,
  onConfirm,
  onClose,
}: ConfirmDeleteModalProps) {
  const [input, setInput] = useState('')
  const matches = input === confirmValue

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div className="absolute inset-0 bg-black/50" onClick={onClose} aria-hidden="true" />
      <div className="relative w-full max-w-md rounded-xl bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 p-6 shadow-xl">
        <h2 className="text-base font-semibold text-zinc-900 dark:text-zinc-50 mb-2">{title}</h2>
        <p className="text-sm text-zinc-500 mb-4">{description}</p>

        {blockedBy && Object.keys(blockedBy).length > 0 && (
          <div className="rounded-lg border border-amber-200 bg-amber-50 dark:border-amber-900/50 dark:bg-amber-950/30 px-4 py-3 text-sm text-amber-700 dark:text-amber-400 mb-4">
            <p className="font-medium mb-1">Nao e possivel deletar — registros dependentes:</p>
            <ul className="list-disc list-inside space-y-0.5 text-xs">
              {Object.entries(blockedBy).map(([entity, count]) => (
                <li key={entity}>{entity}: {count} registro(s)</li>
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
              para confirmar:
            </p>
            <input
              type="text"
              value={input}
              onChange={e => setInput(e.target.value)}
              className={`${inputCls} mb-4 font-mono`}
              placeholder={confirmValue}
              autoComplete="off"
              autoFocus
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

// ── CardImageUpload ────────────────────────────────────────────────────────

interface CardImageUploadProps {
  label: string
  currentUrl: string
  slot: 'en' | 'pt'
  cardId: string
  onUploaded: (url: string, slot: 'en' | 'pt') => void
}

function CardImageUpload({ label, currentUrl, slot, cardId, onUploaded }: CardImageUploadProps) {
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
      const result = await uploadCardImage(cardId, file, slot)
      const url =
        slot === 'en'
          ? result.image_large_url ?? result.image_small_url
          : result.image_url_pt
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
            className="w-16 h-24 object-contain rounded-lg border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 flex-shrink-0"
          />
        ) : (
          <div className="w-16 h-24 rounded-lg border-2 border-dashed border-zinc-300 dark:border-zinc-700 flex items-center justify-center text-zinc-400 text-xs flex-shrink-0">
            Vazio
          </div>
        )}
        <div className="pt-1">
          <label className="cursor-pointer text-sm text-violet-600 hover:text-violet-700 dark:text-violet-400 font-medium">
            {uploading ? 'Enviando...' : preview ? 'Alterar' : 'Enviar'}
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp"
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

// ── VariantRow ─────────────────────────────────────────────────────────────

interface VariantRowProps {
  variant: CatalogVariant
  onUpdated: (v: CatalogVariant) => void
  onDeleted: (id: string) => void
}

function VariantRow({ variant, onUpdated, onDeleted }: VariantRowProps) {
  const [editing, setEditing] = useState(false)
  const [finish, setFinish] = useState(variant.finish)
  const [label, setLabel] = useState(variant.label)
  const [isPromo, setIsPromo] = useState(variant.is_promo)
  const [notes, setNotes] = useState(variant.notes)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState('')
  const [deleteBlockedBy, setDeleteBlockedBy] = useState<Record<string, number> | undefined>()
  const [showDeleteModal, setShowDeleteModal] = useState(false)

  async function handleSave() {
    setSaving(true)
    setError('')
    try {
      const updated = await patchAdminVariant(variant.id, {
        finish: finish.trim(),
        label: label.trim() || undefined,
        is_promo: isPromo,
        notes: notes.trim() || undefined,
      })
      onUpdated(updated)
      setEditing(false)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Erro ao salvar')
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete() {
    setDeleting(true)
    setDeleteError('')
    setDeleteBlockedBy(undefined)
    try {
      await deleteAdminVariant(variant.id)
      onDeleted(variant.id)
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

  return (
    <>
      {editing ? (
        <tr className="bg-violet-50/50 dark:bg-violet-900/10">
          <td className="px-4 py-2.5">
            <input
              type="text"
              value={finish}
              onChange={e => setFinish(e.target.value)}
              className="w-full rounded border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-2 py-1 text-xs font-mono focus:outline-none focus:ring-1 focus:ring-violet-500"
            />
          </td>
          <td className="px-4 py-2.5">
            <input
              type="text"
              value={label}
              onChange={e => setLabel(e.target.value)}
              className="w-full rounded border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-violet-500"
            />
          </td>
          <td className="px-4 py-2.5 text-center">
            <input
              type="checkbox"
              checked={isPromo}
              onChange={e => setIsPromo(e.target.checked)}
              className="accent-violet-600"
            />
          </td>
          <td className="px-4 py-2.5">
            <input
              type="text"
              value={notes}
              onChange={e => setNotes(e.target.value)}
              className="w-full rounded border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-violet-500"
            />
          </td>
          <td className="px-4 py-2.5 text-right">
            <div className="flex items-center justify-end gap-2">
              <button
                type="button"
                onClick={handleSave}
                disabled={saving}
                className="rounded px-2 py-1 text-xs font-medium text-violet-600 hover:text-violet-700 dark:text-violet-400 hover:bg-violet-50 dark:hover:bg-violet-900/20 disabled:opacity-50 transition-colors"
              >
                {saving ? 'Salvando...' : 'Salvar'}
              </button>
              <button
                type="button"
                onClick={() => setEditing(false)}
                className="rounded px-2 py-1 text-xs font-medium text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors"
              >
                Cancelar
              </button>
            </div>
            {error && <p className="text-xs text-red-500 mt-1">{error}</p>}
          </td>
        </tr>
      ) : (
        <tr className="bg-white dark:bg-zinc-900 hover:bg-zinc-50 dark:hover:bg-zinc-800/50 transition-colors">
          <td className="px-4 py-2.5 font-mono text-xs text-zinc-600 dark:text-zinc-400">
            {variant.finish}
          </td>
          <td className="px-4 py-2.5 text-zinc-700 dark:text-zinc-300 text-xs">
            {variant.label || <span className="italic text-zinc-300 dark:text-zinc-600">—</span>}
          </td>
          <td className="px-4 py-2.5 text-center">
            {variant.is_promo ? (
              <span className="inline-flex items-center rounded-full bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-400 px-2 py-0.5 text-xs font-medium">
                Promo
              </span>
            ) : (
              <span className="text-zinc-300 dark:text-zinc-600 text-xs">—</span>
            )}
          </td>
          <td className="px-4 py-2.5 text-zinc-500 text-xs max-w-[200px] truncate">
            {variant.notes || <span className="italic text-zinc-300 dark:text-zinc-600">—</span>}
          </td>
          <td className="px-4 py-2.5 text-right">
            <div className="flex items-center justify-end gap-2">
              <button
                type="button"
                onClick={() => setEditing(true)}
                className="rounded px-2 py-1 text-xs font-medium text-violet-600 hover:text-violet-700 dark:text-violet-400 hover:bg-violet-50 dark:hover:bg-violet-900/20 transition-colors"
              >
                Editar
              </button>
              <button
                type="button"
                onClick={() => { setDeleteError(''); setDeleteBlockedBy(undefined); setShowDeleteModal(true) }}
                className={btnDangerSm}
              >
                Deletar
              </button>
            </div>
          </td>
        </tr>
      )}
      {showDeleteModal && (
        <ConfirmDeleteModal
          title="Deletar variante"
          description={`Deletar a variante "${variant.finish}"? Esta acao nao pode ser desfeita.`}
          confirmValue={variant.finish}
          deleting={deleting}
          error={deleteError}
          blockedBy={deleteBlockedBy}
          onConfirm={handleDelete}
          onClose={() => setShowDeleteModal(false)}
        />
      )}
    </>
  )
}

// ── NewVariantRow ──────────────────────────────────────────────────────────

interface NewVariantRowProps {
  cardId: string
  onCreated: (v: CatalogVariant) => void
}

function NewVariantRow({ cardId, onCreated }: NewVariantRowProps) {
  const [finish, setFinish] = useState('')
  const [label, setLabel] = useState('')
  const [isPromo, setIsPromo] = useState(false)
  const [notes, setNotes] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!finish.trim()) return
    setSaving(true)
    setError('')
    try {
      const created = await createAdminVariant(cardId, {
        finish: finish.trim(),
        label: label.trim() || undefined,
        is_promo: isPromo,
        notes: notes.trim() || undefined,
      })
      onCreated(created)
      setFinish('')
      setLabel('')
      setIsPromo(false)
      setNotes('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Erro ao criar variante')
    } finally {
      setSaving(false)
    }
  }

  return (
    <tr className="bg-zinc-50/80 dark:bg-zinc-800/30">
      <td className="px-4 py-2.5">
        <input
          type="text"
          value={finish}
          onChange={e => setFinish(e.target.value)}
          placeholder="ex: holo"
          className="w-full rounded border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-2 py-1 text-xs font-mono focus:outline-none focus:ring-1 focus:ring-violet-500"
          required
        />
      </td>
      <td className="px-4 py-2.5">
        <input
          type="text"
          value={label}
          onChange={e => setLabel(e.target.value)}
          placeholder="Label display"
          className="w-full rounded border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-violet-500"
        />
      </td>
      <td className="px-4 py-2.5 text-center">
        <input
          type="checkbox"
          checked={isPromo}
          onChange={e => setIsPromo(e.target.checked)}
          className="accent-violet-600"
        />
      </td>
      <td className="px-4 py-2.5">
        <input
          type="text"
          value={notes}
          onChange={e => setNotes(e.target.value)}
          placeholder="Notas opcionais"
          className="w-full rounded border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-violet-500"
        />
      </td>
      <td className="px-4 py-2.5 text-right">
        <div className="flex items-center justify-end gap-2">
          <button
            type="button"
            onClick={handleSubmit}
            disabled={saving || !finish.trim()}
            className="rounded px-2 py-1 text-xs font-medium text-violet-600 hover:text-violet-700 dark:text-violet-400 hover:bg-violet-50 dark:hover:bg-violet-900/20 disabled:opacity-50 transition-colors"
          >
            {saving ? 'Criando...' : 'Adicionar'}
          </button>
        </div>
        {error && <p className="text-xs text-red-500 mt-1">{error}</p>}
      </td>
    </tr>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────

export default function EditCardPage() {
  const { id } = useParams<{ id: string }>()
  const router = useRouter()

  const [card, setCard] = useState<CatalogCard | null>(null)
  const [loadError, setLoadError] = useState('')

  // Edit form
  const [name, setName] = useState('')
  const [namePt, setNamePt] = useState('')
  const [collectorNumber, setCollectorNumber] = useState('')
  const [rarity, setRarity] = useState('')
  const [supertype, setSupertype] = useState('')
  const [subtypes, setSubtypes] = useState('')
  const [types, setTypes] = useState('')
  const [hp, setHp] = useState('')
  const [illustrator, setIllustrator] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [saveSuccess, setSaveSuccess] = useState('')

  // Variants
  const [variants, setVariants] = useState<CatalogVariant[]>([])
  const [variantsLoading, setVariantsLoading] = useState(false)
  const [variantsError, setVariantsError] = useState('')

  // Delete card
  const [showDeleteCard, setShowDeleteCard] = useState(false)
  const [deleteError, setDeleteError] = useState('')
  const [deleteBlockedBy, setDeleteBlockedBy] = useState<Record<string, number> | undefined>()
  const [deleting, setDeleting] = useState(false)

  // Load card
  useEffect(() => {
    fetchAdminCard(id)
      .then(c => {
        setCard(c)
        setName(c.name)
        setNamePt(c.name_pt || '')
        setCollectorNumber(c.collector_number)
        setRarity(c.rarity || '')
        setSupertype(c.supertype || '')
        setSubtypes(Array.isArray(c.subtypes) ? c.subtypes.join(', ') : '')
        setTypes(Array.isArray(c.types) ? c.types.join(', ') : '')
        setHp(c.hp ? String(c.hp) : '')
        setIllustrator(c.illustrator || '')
      })
      .catch(e => setLoadError(e.message))
  }, [id])

  // Load variants
  useEffect(() => {
    setVariantsLoading(true)
    fetchCardVariants(id)
      .then(setVariants)
      .catch(e => setVariantsError(e.message))
      .finally(() => setVariantsLoading(false))
  }, [id])

  async function handleSave(e: React.FormEvent) {
    e.preventDefault()
    if (!card) return
    setSaving(true)
    setSaveError('')
    setSaveSuccess('')
    try {
      const updated = await patchAdminCard(card.id, {
        name: name.trim(),
        name_pt: namePt.trim() || undefined,
        collector_number: collectorNumber.trim(),
        rarity: rarity.trim() || undefined,
        supertype: supertype.trim() || undefined,
        subtypes: subtypes.trim()
          ? subtypes.split(',').map(s => s.trim()).filter(Boolean)
          : undefined,
        types: types.trim()
          ? types.split(',').map(s => s.trim()).filter(Boolean)
          : undefined,
        hp: hp ? Number(hp) : undefined,
        illustrator: illustrator.trim() || undefined,
      })
      setCard(updated)
      setSaveSuccess('Carta atualizada com sucesso.')
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : 'Erro ao salvar')
    } finally {
      setSaving(false)
    }
  }

  function handleImageUploaded(url: string, slot: 'en' | 'pt') {
    setCard(prev => {
      if (!prev) return prev
      if (slot === 'en') return { ...prev, image_large_url: url }
      return { ...prev, image_url_pt: url }
    })
  }

  async function handleDeleteCard() {
    if (!card) return
    setDeleting(true)
    setDeleteError('')
    setDeleteBlockedBy(undefined)
    try {
      await deleteAdminCard(card.id, card.collector_number)
      router.push(`/admin/catalogo/sets/${card.set_id}`)
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
        <Link href="/admin/catalogo/cards" className="mt-3 inline-block text-sm text-violet-600 hover:underline">
          Voltar para busca
        </Link>
      </div>
    )
  }

  if (!card) {
    return (
      <div className="min-h-[60vh] flex items-center justify-center">
        <div className="animate-pulse text-zinc-400 text-sm">Carregando carta...</div>
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
            <Link href="/admin/catalogo/cards" className="hover:text-zinc-900 dark:hover:text-zinc-100">
              Cartas
            </Link>
            <span>/</span>
            <Link
              href={`/admin/catalogo/sets/${card.set_id}`}
              className="hover:text-zinc-900 dark:hover:text-zinc-100"
            >
              Set
            </Link>
            <span>/</span>
            <span className="text-zinc-900 dark:text-zinc-50 font-mono truncate max-w-[100px]">
              #{card.collector_number}
            </span>
          </div>
          <div className="flex items-start justify-between gap-4">
            <div className="flex items-center gap-4">
              {card.image_small_url && (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={card.image_small_url}
                  alt={card.name}
                  className="w-12 h-16 object-contain rounded border border-zinc-200 dark:border-zinc-700 flex-shrink-0"
                />
              )}
              <div>
                <h1 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50">{card.name}</h1>
                <p className="text-xs text-zinc-400 mt-0.5 font-mono">
                  #{card.collector_number}
                  {card.rarity && ` · ${card.rarity}`}
                </p>
              </div>
            </div>
            <button
              type="button"
              onClick={() => { setDeleteError(''); setDeleteBlockedBy(undefined); setShowDeleteCard(true) }}
              className={btnDanger}
            >
              Deletar carta
            </button>
          </div>
        </div>

        <div className="space-y-6">
          {/* Edit form */}
          <form onSubmit={handleSave}>
            <div className={sectionCls}>
              <h2 className={sectionTitleCls}>Dados da carta</h2>
              <div className="grid grid-cols-2 gap-4">
                <div>
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
                  <label className={labelCls}>Numero coletor</label>
                  <input
                    type="text"
                    value={collectorNumber}
                    onChange={e => setCollectorNumber(e.target.value)}
                    className={`${inputCls} font-mono`}
                  />
                </div>
                <div>
                  <label className={labelCls}>Raridade</label>
                  <input
                    type="text"
                    value={rarity}
                    onChange={e => setRarity(e.target.value)}
                    className={inputCls}
                    placeholder="Rare Holo"
                  />
                </div>
                <div>
                  <label className={labelCls}>Supertipo</label>
                  <input
                    type="text"
                    value={supertype}
                    onChange={e => setSupertype(e.target.value)}
                    className={inputCls}
                    placeholder="Pokemon"
                  />
                </div>
                <div>
                  <label className={labelCls}>HP</label>
                  <input
                    type="number"
                    min="0"
                    value={hp}
                    onChange={e => setHp(e.target.value)}
                    className={inputCls}
                  />
                </div>
                <div>
                  <label className={labelCls}>Subtipos</label>
                  <input
                    type="text"
                    value={subtypes}
                    onChange={e => setSubtypes(e.target.value)}
                    className={inputCls}
                    placeholder="Basic, Stage 1 (separados por virgula)"
                  />
                </div>
                <div>
                  <label className={labelCls}>Tipos</label>
                  <input
                    type="text"
                    value={types}
                    onChange={e => setTypes(e.target.value)}
                    className={inputCls}
                    placeholder="Lightning, Fire (separados por virgula)"
                  />
                </div>
                <div className="col-span-2">
                  <label className={labelCls}>Ilustrador</label>
                  <input
                    type="text"
                    value={illustrator}
                    onChange={e => setIllustrator(e.target.value)}
                    className={inputCls}
                    placeholder="Mitsuhiro Arita"
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
              <CardImageUpload
                label="Imagem EN (grande)"
                currentUrl={card.image_large_url}
                slot="en"
                cardId={card.id}
                onUploaded={handleImageUploaded}
              />
              <CardImageUpload
                label="Imagem PT"
                currentUrl={card.image_url_pt}
                slot="pt"
                cardId={card.id}
                onUploaded={handleImageUploaded}
              />
            </div>
          </div>

          {/* Variants */}
          <div className={sectionCls}>
            <h2 className={sectionTitleCls}>
              Variantes ({variants.length})
            </h2>

            {variantsLoading && (
              <p className="text-sm text-zinc-400 py-2">Carregando variantes...</p>
            )}
            {variantsError && <div className={alertError}>{variantsError}</div>}

            {!variantsLoading && (
              <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 overflow-hidden">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="bg-zinc-50 dark:bg-zinc-900 border-b border-zinc-200 dark:border-zinc-800">
                      <th className="px-4 py-3 text-left font-medium text-zinc-500">Finish</th>
                      <th className="px-4 py-3 text-left font-medium text-zinc-500">Label</th>
                      <th className="px-4 py-3 text-left font-medium text-zinc-500 text-center">Promo</th>
                      <th className="px-4 py-3 text-left font-medium text-zinc-500">Notas</th>
                      <th className="px-4 py-3 text-right font-medium text-zinc-500">Acoes</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-zinc-100 dark:divide-zinc-800">
                    {variants.map(v => (
                      <VariantRow
                        key={v.id}
                        variant={v}
                        onUpdated={updated =>
                          setVariants(prev => prev.map(x => x.id === updated.id ? updated : x))
                        }
                        onDeleted={deletedId =>
                          setVariants(prev => prev.filter(x => x.id !== deletedId))
                        }
                      />
                    ))}
                    {/* New variant row */}
                    <NewVariantRow
                      cardId={card.id}
                      onCreated={v => setVariants(prev => [...prev, v])}
                    />
                  </tbody>
                </table>
              </div>
            )}
          </div>
        </div>
      </div>

      {showDeleteCard && (
        <ConfirmDeleteModal
          title="Deletar carta"
          description={`Esta acao e irreversivel e deletara a carta "${card.name}" (${card.collector_number}) e todos os seus dados.`}
          confirmValue={card.collector_number}
          deleting={deleting}
          error={deleteError}
          blockedBy={deleteBlockedBy}
          onConfirm={handleDeleteCard}
          onClose={() => setShowDeleteCard(false)}
        />
      )}
    </>
  )
}
