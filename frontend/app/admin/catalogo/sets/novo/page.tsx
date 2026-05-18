'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { createAdminSet } from '@/lib/catalog-admin'

const TCG_OPTIONS = [
  { value: 'pokemon', label: 'Pokemon' },
  { value: 'pokemon-pocket', label: 'Pokemon Pocket' },
  { value: 'magic', label: 'Magic' },
  { value: 'yugioh', label: 'Yu-Gi-Oh!' },
  { value: 'onepiece', label: 'One Piece' },
  { value: 'lorcana', label: 'Lorcana' },
  { value: 'fab', label: 'Flesh and Blood' },
]

const LANG_OPTIONS = [
  { value: 'en', label: 'Ingles (EN)' },
  { value: 'pt', label: 'Portugues (PT)' },
  { value: 'ja', label: 'Japones (JA)' },
  { value: 'es', label: 'Espanhol (ES)' },
  { value: 'fr', label: 'Frances (FR)' },
  { value: 'de', label: 'Alemao (DE)' },
  { value: 'it', label: 'Italiano (IT)' },
  { value: 'ko', label: 'Coreano (KO)' },
  { value: 'zh-hant', label: 'Chines Tradicional' },
  { value: 'zh-hans', label: 'Chines Simplificado' },
]

const inputCls =
  'w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500'
const labelCls = 'block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1.5'
const sectionCls = 'rounded-xl border border-zinc-200 dark:border-zinc-800 p-5 space-y-4'
const sectionTitleCls = 'text-xs font-semibold uppercase tracking-widest text-zinc-400'

export default function NovoSetPage() {
  const router = useRouter()

  const [form, setForm] = useState({
    code: '',
    name: '',
    name_pt: '',
    name_en: '',
    tcg: 'pokemon',
    language: 'en',
    series_id: '',
    release_date: '',
    total_cards: '',
    printed_total: '',
  })
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  function set<K extends keyof typeof form>(key: K, value: string) {
    setForm(prev => ({ ...prev, [key]: value }))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError('')
    try {
      const created = await createAdminSet({
        code: form.code.trim(),
        name: form.name.trim(),
        name_pt: form.name_pt.trim() || undefined,
        name_en: form.name_en.trim() || undefined,
        tcg: form.tcg,
        language: form.language,
        series_id: form.series_id.trim() || undefined,
        release_date: form.release_date || undefined,
        total_cards: form.total_cards ? Number(form.total_cards) : undefined,
        printed_total: form.printed_total ? Number(form.printed_total) : undefined,
      })
      router.push(`/admin/catalogo/sets/${created.id}`)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Erro ao criar set')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="mx-auto max-w-2xl px-4 py-6">
      <div className="mb-6">
        <div className="flex items-center gap-2 text-sm text-zinc-500 mb-1">
          <Link href="/admin/catalogo" className="hover:text-zinc-900 dark:hover:text-zinc-100">
            Catalogo
          </Link>
          <span>/</span>
          <Link href="/admin/catalogo/sets?tcg=pokemon" className="hover:text-zinc-900 dark:hover:text-zinc-100">
            Sets
          </Link>
          <span>/</span>
          <span className="text-zinc-900 dark:text-zinc-50">Novo</span>
        </div>
        <h1 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50">Novo Set</h1>
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
        {/* Identidade */}
        <div className={sectionCls}>
          <h2 className={sectionTitleCls}>Identidade</h2>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className={labelCls}>
                TCG <span className="text-red-500">*</span>
              </label>
              <select
                required
                value={form.tcg}
                onChange={e => set('tcg', e.target.value)}
                className={inputCls}
              >
                {TCG_OPTIONS.map(opt => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </select>
            </div>
            <div>
              <label className={labelCls}>
                Idioma <span className="text-red-500">*</span>
              </label>
              <select
                required
                value={form.language}
                onChange={e => set('language', e.target.value)}
                className={inputCls}
              >
                {LANG_OPTIONS.map(opt => (
                  <option key={opt.value} value={opt.value}>{opt.label}</option>
                ))}
              </select>
            </div>
          </div>

          <div>
            <label className={labelCls}>
              Codigo do set <span className="text-red-500">*</span>
            </label>
            <input
              type="text"
              required
              value={form.code}
              onChange={e => set('code', e.target.value)}
              placeholder="ex: sv01"
              className={`${inputCls} font-mono`}
            />
            <p className="mt-1 text-xs text-zinc-400">
              Slug unico do set — nunca pode conter hifen. Ex: sv01, base1, swsh1
            </p>
          </div>

          <div>
            <label className={labelCls}>
              Nome (EN) <span className="text-red-500">*</span>
            </label>
            <input
              type="text"
              required
              value={form.name}
              onChange={e => set('name', e.target.value)}
              placeholder="ex: Scarlet & Violet"
              className={inputCls}
            />
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className={labelCls}>Nome EN alternativo</label>
              <input
                type="text"
                value={form.name_en}
                onChange={e => set('name_en', e.target.value)}
                placeholder="igual ao nome se vazio"
                className={inputCls}
              />
            </div>
            <div>
              <label className={labelCls}>Nome PT</label>
              <input
                type="text"
                value={form.name_pt}
                onChange={e => set('name_pt', e.target.value)}
                placeholder="Nome em portugues"
                className={inputCls}
              />
            </div>
          </div>
        </div>

        {/* Metadata */}
        <div className={sectionCls}>
          <h2 className={sectionTitleCls}>Metadata</h2>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className={labelCls}>ID da Serie</label>
              <input
                type="text"
                value={form.series_id}
                onChange={e => set('series_id', e.target.value)}
                placeholder="UUID da serie (opcional)"
                className={`${inputCls} font-mono text-xs`}
              />
            </div>
            <div>
              <label className={labelCls}>Data de lancamento</label>
              <input
                type="date"
                value={form.release_date}
                onChange={e => set('release_date', e.target.value)}
                className={inputCls}
              />
            </div>
            <div>
              <label className={labelCls}>Total de cartas</label>
              <input
                type="number"
                min="0"
                value={form.total_cards}
                onChange={e => set('total_cards', e.target.value)}
                className={inputCls}
                placeholder="0"
              />
            </div>
            <div>
              <label className={labelCls}>Total impresso</label>
              <input
                type="number"
                min="0"
                value={form.printed_total}
                onChange={e => set('printed_total', e.target.value)}
                className={inputCls}
                placeholder="0"
              />
              <p className="mt-1 text-xs text-zinc-400">
                Usado para autocomplete no formato 110/217
              </p>
            </div>
          </div>
        </div>

        {error && (
          <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400">
            {error}
          </div>
        )}

        <div className="flex items-center gap-3 pt-1 border-t border-zinc-200 dark:border-zinc-800">
          <button
            type="submit"
            disabled={saving}
            className="rounded-lg bg-violet-600 px-5 py-2.5 text-sm font-semibold text-white hover:bg-violet-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {saving ? 'Criando...' : 'Criar Set'}
          </button>
          <Link
            href="/admin/catalogo/sets?tcg=pokemon"
            className="px-4 py-2.5 text-sm font-medium text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 transition-colors"
          >
            Cancelar
          </Link>
        </div>
      </form>
    </div>
  )
}
