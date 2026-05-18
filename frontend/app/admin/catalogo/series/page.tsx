'use client'

import { useEffect, useState, useCallback, useRef } from 'react'
import Link from 'next/link'
import {
  fetchAdminSeries,
  createAdminSeries,
  patchAdminSeries,
  deleteAdminSeries,
  CatalogSeries,
  ConflictDeleteError,
} from '@/lib/catalog-admin'

// ── Constants ─────────────────────────────────────────────────────────────────

const TCG_OPTIONS = [
  { value: 'pokemon', label: 'Pokémon' },
  { value: 'pokemon-pocket', label: 'Pokémon Pocket' },
  { value: 'magic', label: 'Magic' },
  { value: 'yugioh', label: 'Yu-Gi-Oh!' },
  { value: 'onepiece', label: 'One Piece' },
  { value: 'lorcana', label: 'Lorcana' },
  { value: 'fab', label: 'Flesh and Blood' },
]

const inputCls =
  'rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500'

// ── Toast ─────────────────────────────────────────────────────────────────────

interface ToastState {
  kind: 'success' | 'error'
  msg: string
}

function Toast({ toast, onDismiss }: { toast: ToastState; onDismiss: () => void }) {
  useEffect(() => {
    const t = setTimeout(onDismiss, 3000)
    return () => clearTimeout(t)
  }, [toast, onDismiss])

  return (
    <div
      className={`fixed bottom-5 right-5 z-50 flex items-center gap-3 rounded-xl px-5 py-3 text-sm font-medium shadow-lg transition-all ${
        toast.kind === 'success'
          ? 'bg-green-600 text-white'
          : 'bg-red-600 text-white'
      }`}
    >
      {toast.msg}
      <button onClick={onDismiss} className="ml-1 opacity-70 hover:opacity-100">
        ×
      </button>
    </div>
  )
}

// ── Backdrop ──────────────────────────────────────────────────────────────────

function Backdrop({ onClose }: { onClose: () => void }) {
  return (
    <div
      className="fixed inset-0 z-40 bg-black/40 backdrop-blur-sm"
      onClick={onClose}
      aria-hidden="true"
    />
  )
}

// ── CreateSeriesModal ─────────────────────────────────────────────────────────

interface CreateSeriesModalProps {
  defaultTcg: string
  onClose: () => void
  onCreated: () => void
  onToast: (t: ToastState) => void
}

function CreateSeriesModal({ defaultTcg, onClose, onCreated, onToast }: CreateSeriesModalProps) {
  const [name, setName] = useState('')
  const [namePT, setNamePT] = useState('')
  const [tcg, setTcg] = useState(defaultTcg)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const nameRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    nameRef.current?.focus()
  }, [])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    setSubmitting(true)
    setError(null)
    try {
      await createAdminSeries({ name: name.trim(), name_pt: namePT.trim() || undefined, tcg })
      onToast({ kind: 'success', msg: 'Série criada com sucesso.' })
      onCreated()
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Erro ao criar série')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <Backdrop onClose={onClose} />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="create-series-title"
        className="fixed inset-0 z-50 flex items-center justify-center p-4"
      >
        <div className="w-full max-w-md rounded-2xl border border-zinc-200 bg-white p-6 shadow-xl dark:border-zinc-700 dark:bg-zinc-900">
          <h2
            id="create-series-title"
            className="mb-5 text-base font-semibold text-zinc-900 dark:text-zinc-50"
          >
            Nova série
          </h2>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="mb-1 block text-xs font-medium text-zinc-600 dark:text-zinc-400">
                Nome EN <span className="text-red-500">*</span>
              </label>
              <input
                ref={nameRef}
                type="text"
                value={name}
                onChange={e => setName(e.target.value)}
                required
                disabled={submitting}
                placeholder="Ex: Scarlet & Violet"
                className={`${inputCls} w-full`}
              />
            </div>

            <div>
              <label className="mb-1 block text-xs font-medium text-zinc-600 dark:text-zinc-400">
                Nome PT
              </label>
              <input
                type="text"
                value={namePT}
                onChange={e => setNamePT(e.target.value)}
                disabled={submitting}
                placeholder="Ex: Escarlate & Violeta"
                className={`${inputCls} w-full`}
              />
            </div>

            <div>
              <label className="mb-1 block text-xs font-medium text-zinc-600 dark:text-zinc-400">
                TCG <span className="text-red-500">*</span>
              </label>
              <select
                value={tcg}
                onChange={e => setTcg(e.target.value)}
                disabled={submitting}
                className={`${inputCls} w-full`}
              >
                {TCG_OPTIONS.map(opt => (
                  <option key={opt.value} value={opt.value}>
                    {opt.label}
                  </option>
                ))}
              </select>
            </div>

            {error && (
              <p className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400">
                {error}
              </p>
            )}

            <div className="flex items-center justify-end gap-3 pt-1">
              <button
                type="button"
                onClick={onClose}
                disabled={submitting}
                className="rounded-lg border border-zinc-300 px-4 py-2 text-sm text-zinc-600 hover:bg-zinc-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800 disabled:opacity-50 transition-colors"
              >
                Cancelar
              </button>
              <button
                type="submit"
                disabled={submitting || !name.trim()}
                className="rounded-lg bg-violet-600 px-4 py-2 text-sm font-semibold text-white hover:bg-violet-700 disabled:opacity-50 transition-colors"
              >
                {submitting ? 'Criando...' : 'Criar série'}
              </button>
            </div>
          </form>
        </div>
      </div>
    </>
  )
}

// ── EditSeriesModal ───────────────────────────────────────────────────────────

interface EditSeriesModalProps {
  series: CatalogSeries
  onClose: () => void
  onSaved: () => void
  onToast: (t: ToastState) => void
}

function EditSeriesModal({ series, onClose, onSaved, onToast }: EditSeriesModalProps) {
  const [name, setName] = useState(series.name)
  const [namePT, setNamePT] = useState(series.name_pt || '')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const nameRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    nameRef.current?.focus()
  }, [])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    setSubmitting(true)
    setError(null)
    try {
      await patchAdminSeries(series.id, {
        name: name.trim(),
        name_pt: namePT.trim() || undefined,
      })
      onToast({ kind: 'success', msg: 'Série atualizada com sucesso.' })
      onSaved()
      onClose()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Erro ao atualizar série')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <Backdrop onClose={onClose} />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="edit-series-title"
        className="fixed inset-0 z-50 flex items-center justify-center p-4"
      >
        <div className="w-full max-w-md rounded-2xl border border-zinc-200 bg-white p-6 shadow-xl dark:border-zinc-700 dark:bg-zinc-900">
          <h2
            id="edit-series-title"
            className="mb-5 text-base font-semibold text-zinc-900 dark:text-zinc-50"
          >
            Editar série
          </h2>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="mb-1 block text-xs font-medium text-zinc-600 dark:text-zinc-400">
                Nome EN <span className="text-red-500">*</span>
              </label>
              <input
                ref={nameRef}
                type="text"
                value={name}
                onChange={e => setName(e.target.value)}
                required
                disabled={submitting}
                className={`${inputCls} w-full`}
              />
            </div>

            <div>
              <label className="mb-1 block text-xs font-medium text-zinc-600 dark:text-zinc-400">
                Nome PT
              </label>
              <input
                type="text"
                value={namePT}
                onChange={e => setNamePT(e.target.value)}
                disabled={submitting}
                placeholder="—"
                className={`${inputCls} w-full`}
              />
            </div>

            <div>
              <label className="mb-1 block text-xs font-medium text-zinc-600 dark:text-zinc-400">
                TCG
              </label>
              <input
                type="text"
                value={series.tcg}
                disabled
                className={`${inputCls} w-full cursor-not-allowed opacity-60`}
              />
              <p className="mt-1 text-xs text-zinc-400">O TCG não pode ser alterado após a criação.</p>
            </div>

            {error && (
              <p className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400">
                {error}
              </p>
            )}

            <div className="flex items-center justify-end gap-3 pt-1">
              <button
                type="button"
                onClick={onClose}
                disabled={submitting}
                className="rounded-lg border border-zinc-300 px-4 py-2 text-sm text-zinc-600 hover:bg-zinc-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800 disabled:opacity-50 transition-colors"
              >
                Cancelar
              </button>
              <button
                type="submit"
                disabled={submitting || !name.trim()}
                className="rounded-lg bg-violet-600 px-4 py-2 text-sm font-semibold text-white hover:bg-violet-700 disabled:opacity-50 transition-colors"
              >
                {submitting ? 'Salvando...' : 'Salvar alterações'}
              </button>
            </div>
          </form>
        </div>
      </div>
    </>
  )
}

// ── DeleteSeriesModal ─────────────────────────────────────────────────────────

interface DeleteSeriesModalProps {
  series: CatalogSeries
  onClose: () => void
  onDeleted: () => void
  onToast: (t: ToastState) => void
}

function DeleteSeriesModal({ series, onClose, onDeleted, onToast }: DeleteSeriesModalProps) {
  const [submitting, setSubmitting] = useState(false)
  const [conflictMsg, setConflictMsg] = useState<string | null>(null)

  async function handleDelete() {
    setSubmitting(true)
    setConflictMsg(null)
    try {
      await deleteAdminSeries(series.id)
      onToast({ kind: 'success', msg: 'Série excluída com sucesso.' })
      onDeleted()
      onClose()
    } catch (err) {
      if (err instanceof ConflictDeleteError) {
        const setsCount = err.blockedBy['sets_count'] ?? 0
        setConflictMsg(
          `Não é possível excluir. Esta série tem ${setsCount} set${setsCount !== 1 ? 's' : ''} vinculado${setsCount !== 1 ? 's' : ''}. Remova os sets primeiro.`,
        )
      } else {
        onToast({
          kind: 'error',
          msg: err instanceof Error ? err.message : 'Erro ao excluir série',
        })
        onClose()
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <Backdrop onClose={onClose} />
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="delete-series-title"
        className="fixed inset-0 z-50 flex items-center justify-center p-4"
      >
        <div className="w-full max-w-sm rounded-2xl border border-zinc-200 bg-white p-6 shadow-xl dark:border-zinc-700 dark:bg-zinc-900">
          <h2
            id="delete-series-title"
            className="mb-3 text-base font-semibold text-zinc-900 dark:text-zinc-50"
          >
            Excluir série
          </h2>

          <p className="mb-5 text-sm text-zinc-600 dark:text-zinc-300">
            Excluir a série <strong className="text-zinc-900 dark:text-zinc-50">{series.name}</strong>?
            Esta ação não pode ser desfeita.
          </p>

          {conflictMsg && (
            <p className="mb-4 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-950/30 dark:text-amber-300">
              {conflictMsg}
            </p>
          )}

          <div className="flex items-center justify-end gap-3">
            <button
              type="button"
              onClick={onClose}
              disabled={submitting}
              className="rounded-lg border border-zinc-300 px-4 py-2 text-sm text-zinc-600 hover:bg-zinc-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800 disabled:opacity-50 transition-colors"
            >
              Cancelar
            </button>
            {!conflictMsg && (
              <button
                type="button"
                onClick={handleDelete}
                disabled={submitting}
                className="rounded-lg bg-red-600 px-4 py-2 text-sm font-semibold text-white hover:bg-red-700 disabled:opacity-50 transition-colors"
              >
                {submitting ? 'Excluindo...' : 'Excluir'}
              </button>
            )}
          </div>
        </div>
      </div>
    </>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function SeriesListPage() {
  const [tcg, setTcg] = useState('pokemon')
  const [series, setSeries] = useState<CatalogSeries[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [createOpen, setCreateOpen] = useState(false)
  const [editing, setEditing] = useState<CatalogSeries | null>(null)
  const [deleting, setDeleting] = useState<CatalogSeries | null>(null)
  const [toast, setToast] = useState<ToastState | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await fetchAdminSeries(tcg)
      setSeries(data)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Erro ao carregar séries')
    } finally {
      setLoading(false)
    }
  }, [tcg])

  useEffect(() => {
    load()
  }, [load])

  return (
    <div className="mx-auto max-w-4xl px-4 py-6">
      {/* Header */}
      <div className="mb-6 flex items-center justify-between gap-4">
        <div>
          <div className="mb-1 flex items-center gap-2 text-sm text-zinc-500">
            <Link href="/admin/catalogo" className="hover:text-zinc-900 dark:hover:text-zinc-100">
              Catálogo
            </Link>
            <span>/</span>
            <span className="text-zinc-900 dark:text-zinc-50">Séries</span>
          </div>
          <h1 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50">Séries</h1>
        </div>
        <button
          onClick={() => setCreateOpen(true)}
          className="rounded-lg bg-violet-600 px-4 py-2 text-sm font-semibold text-white hover:bg-violet-700 transition-colors whitespace-nowrap"
        >
          + Nova série
        </button>
      </div>

      {/* TCG filter */}
      <div className="mb-4 flex items-center gap-3">
        <select
          value={tcg}
          onChange={e => {
            setTcg(e.target.value)
          }}
          className={`${inputCls} min-w-[160px]`}
        >
          {TCG_OPTIONS.map(opt => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
        {loading && <span className="text-xs text-zinc-400">Carregando...</span>}
      </div>

      {/* Error */}
      {error && (
        <div className="mb-4 rounded-xl border border-red-200 bg-red-50 px-5 py-4 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400">
          {error}
        </div>
      )}

      {/* Empty state */}
      {!loading && !error && series.length === 0 && (
        <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
          <p className="text-sm text-zinc-400">Nenhuma série encontrada.</p>
          <button
            onClick={() => setCreateOpen(true)}
            className="text-sm text-violet-600 hover:underline"
          >
            Criar a primeira série
          </button>
        </div>
      )}

      {/* Table */}
      {series.length > 0 && (
        <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-zinc-200 bg-zinc-50 dark:border-zinc-800 dark:bg-zinc-900">
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Nome EN</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Nome PT</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500">TCG</th>
                <th className="px-4 py-3 text-right font-medium text-zinc-500">Ações</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-100 dark:divide-zinc-800">
              {series.map(s => (
                <tr
                  key={s.id}
                  className="bg-white transition-colors hover:bg-zinc-50 dark:bg-zinc-900 dark:hover:bg-zinc-800/50"
                >
                  <td className="max-w-[240px] truncate px-4 py-3 font-medium text-zinc-900 dark:text-zinc-100">
                    {s.name}
                  </td>
                  <td className="max-w-[200px] truncate px-4 py-3 text-zinc-500">
                    {s.name_pt || (
                      <span className="italic text-zinc-300 dark:text-zinc-600">—</span>
                    )}
                  </td>
                  <td className="px-4 py-3 font-mono text-xs text-zinc-400">{s.tcg}</td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-3">
                      <button
                        onClick={() => setEditing(s)}
                        className="text-xs font-medium text-violet-600 hover:text-violet-700 dark:text-violet-400"
                      >
                        Editar
                      </button>
                      <button
                        onClick={() => setDeleting(s)}
                        className="text-xs font-medium text-red-500 hover:text-red-600 dark:text-red-400"
                      >
                        Excluir
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Modals */}
      {createOpen && (
        <CreateSeriesModal
          defaultTcg={tcg}
          onClose={() => setCreateOpen(false)}
          onCreated={load}
          onToast={setToast}
        />
      )}

      {editing && (
        <EditSeriesModal
          series={editing}
          onClose={() => setEditing(null)}
          onSaved={load}
          onToast={setToast}
        />
      )}

      {deleting && (
        <DeleteSeriesModal
          series={deleting}
          onClose={() => setDeleting(null)}
          onDeleted={load}
          onToast={setToast}
        />
      )}

      {/* Toast */}
      {toast && <Toast toast={toast} onDismiss={() => setToast(null)} />}
    </div>
  )
}
