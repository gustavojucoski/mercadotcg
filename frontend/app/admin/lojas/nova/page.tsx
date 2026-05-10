'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { createStore, lookupCNPJ } from '@/lib/stores-admin'

function slugify(s: string) {
  return s
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

function maskCNPJ(v: string) {
  const d = v.replace(/\D/g, '').slice(0, 14)
  if (d.length <= 2) return d
  if (d.length <= 5) return `${d.slice(0,2)}.${d.slice(2)}`
  if (d.length <= 8) return `${d.slice(0,2)}.${d.slice(2,5)}.${d.slice(5)}`
  if (d.length <= 12) return `${d.slice(0,2)}.${d.slice(2,5)}.${d.slice(5,8)}/${d.slice(8)}`
  return `${d.slice(0,2)}.${d.slice(2,5)}.${d.slice(5,8)}/${d.slice(8,12)}-${d.slice(12)}`
}

function maskCPF(v: string) {
  const d = v.replace(/\D/g, '').slice(0, 11)
  if (d.length <= 3) return d
  if (d.length <= 6) return `${d.slice(0,3)}.${d.slice(3)}`
  if (d.length <= 9) return `${d.slice(0,3)}.${d.slice(3,6)}.${d.slice(6)}`
  return `${d.slice(0,3)}.${d.slice(3,6)}.${d.slice(6,9)}-${d.slice(9)}`
}

export default function NovaLojaPage() {
  const router = useRouter()
  const [form, setForm] = useState({
    ownerID: '',
    name: '',
    slug: '',
    description: '',
    documentType: '' as '' | 'cpf' | 'cnpj',
    documentNumber: '',
    legalName: '',
  })
  const [slugManual, setSlugManual] = useState(false)
  const [lookupLoading, setLookupLoading] = useState(false)
  const [lookupError, setLookupError] = useState('')
  const [submitLoading, setSubmitLoading] = useState(false)
  const [submitError, setSubmitError] = useState('')

  function set<K extends keyof typeof form>(key: K, value: typeof form[K]) {
    setForm(prev => ({ ...prev, [key]: value }))
  }

  function handleNameChange(v: string) {
    set('name', v)
    if (!slugManual) set('slug', slugify(v))
  }

  function handleDocNumberChange(raw: string) {
    const masked = form.documentType === 'cnpj' ? maskCNPJ(raw) : maskCPF(raw)
    set('documentNumber', masked)
    // clear previous lookup when number changes
    if (form.documentType === 'cnpj') {
      set('legalName', '')
      setLookupError('')
    }
  }

  async function handleLookup() {
    setLookupLoading(true)
    setLookupError('')
    try {
      const info = await lookupCNPJ(form.documentNumber)
      set('legalName', info.legal_name)
      if (info.situation !== 'ATIVA') {
        setLookupError(`Situação: ${info.situation} (não está ativa)`)
      }
    } catch (e) {
      setLookupError(e instanceof Error ? e.message : 'Erro na consulta')
    } finally {
      setLookupLoading(false)
    }
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSubmitLoading(true)
    setSubmitError('')
    try {
      await createStore({
        owner_id: form.ownerID,
        name: form.name,
        slug: form.slug,
        description: form.description || undefined,
        document_type: form.documentType || undefined,
        document_number: form.documentNumber || undefined,
      })
      router.push('/admin/lojas')
    } catch (e) {
      setSubmitError(e instanceof Error ? e.message : 'Erro ao criar loja')
    } finally {
      setSubmitLoading(false)
    }
  }

  const docPlaceholder = form.documentType === 'cnpj' ? '00.000.000/0000-00' : '000.000.000-00'
  const docReady = form.documentType === 'cnpj'
    ? form.documentNumber.replace(/\D/g, '').length === 14
    : form.documentNumber.replace(/\D/g, '').length === 11

  return (
    <div className="mx-auto max-w-2xl px-4 py-6">
      <div className="mb-6">
        <div className="flex items-center gap-2 text-sm text-zinc-500 mb-1">
          <Link href="/admin/lojas" className="hover:text-zinc-900 dark:hover:text-zinc-100">Lojas</Link>
          <span>/</span>
          <span className="text-zinc-900 dark:text-zinc-50">Nova loja</span>
        </div>
        <h1 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50">Cadastrar loja</h1>
      </div>

      <form onSubmit={handleSubmit} className="flex flex-col gap-5">
        {/* Nome */}
        <div>
          <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1.5">
            Nome da loja <span className="text-red-500">*</span>
          </label>
          <input
            type="text"
            required
            value={form.name}
            onChange={e => handleNameChange(e.target.value)}
            className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500"
            placeholder="Ex: Cards do João"
          />
        </div>

        {/* Slug */}
        <div>
          <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1.5">
            Slug (URL) <span className="text-red-500">*</span>
          </label>
          <input
            type="text"
            required
            value={form.slug}
            onChange={e => { setSlugManual(true); set('slug', e.target.value) }}
            className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 font-mono placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500"
            placeholder="cards-do-joao"
          />
          <p className="mt-1 text-xs text-zinc-400">Identificador único na URL. Gerado automaticamente do nome.</p>
        </div>

        {/* Descrição */}
        <div>
          <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1.5">
            Descrição
          </label>
          <textarea
            value={form.description}
            onChange={e => set('description', e.target.value)}
            rows={3}
            className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500 resize-none"
            placeholder="Descrição opcional da loja"
          />
        </div>

        {/* Owner ID */}
        <div>
          <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1.5">
            UUID do proprietário <span className="text-red-500">*</span>
          </label>
          <input
            type="text"
            required
            value={form.ownerID}
            onChange={e => set('ownerID', e.target.value)}
            className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 font-mono placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500"
            placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
          />
        </div>

        {/* Documento */}
        <fieldset className="rounded-xl border border-zinc-200 dark:border-zinc-800 p-4">
          <legend className="px-1 text-sm font-medium text-zinc-700 dark:text-zinc-300">
            Documento fiscal
          </legend>

          <div className="flex gap-3 mb-4">
            {(['cnpj', 'cpf'] as const).map(t => (
              <label key={t} className="flex items-center gap-2 cursor-pointer">
                <input
                  type="radio"
                  name="docType"
                  value={t}
                  checked={form.documentType === t}
                  onChange={() => { set('documentType', t); set('documentNumber', ''); set('legalName', ''); setLookupError('') }}
                  className="accent-violet-600"
                />
                <span className="text-sm font-medium text-zinc-700 dark:text-zinc-300 uppercase">{t}</span>
              </label>
            ))}
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="radio"
                name="docType"
                value=""
                checked={form.documentType === ''}
                onChange={() => { set('documentType', ''); set('documentNumber', ''); set('legalName', ''); setLookupError('') }}
                className="accent-violet-600"
              />
              <span className="text-sm text-zinc-500">Sem documento</span>
            </label>
          </div>

          {form.documentType !== '' && (
            <div className="flex flex-col gap-3">
              <div className="flex gap-2">
                <input
                  type="text"
                  value={form.documentNumber}
                  onChange={e => handleDocNumberChange(e.target.value)}
                  placeholder={docPlaceholder}
                  className="flex-1 rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 font-mono placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500"
                />
                {form.documentType === 'cnpj' && (
                  <button
                    type="button"
                    onClick={handleLookup}
                    disabled={!docReady || lookupLoading}
                    className="rounded-lg bg-zinc-100 dark:bg-zinc-800 border border-zinc-300 dark:border-zinc-700 px-3 py-2 text-sm font-medium text-zinc-700 dark:text-zinc-200 hover:bg-zinc-200 dark:hover:bg-zinc-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors whitespace-nowrap"
                  >
                    {lookupLoading ? 'Consultando...' : 'Consultar CNPJ'}
                  </button>
                )}
              </div>

              {lookupError && (
                <p className="text-xs text-amber-600 dark:text-amber-400">{lookupError}</p>
              )}

              {form.legalName && (
                <div className="rounded-lg bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 px-3 py-2.5">
                  <p className="text-xs text-green-600 dark:text-green-400 font-medium">Razão social encontrada</p>
                  <p className="text-sm text-green-800 dark:text-green-200 font-semibold mt-0.5">{form.legalName}</p>
                </div>
              )}

              {form.documentType === 'cpf' && docReady && (
                <p className="text-xs text-zinc-400">
                  CPF requer revisão manual. A loja ficará com status <strong>Pendente</strong> até a verificação.
                </p>
              )}
            </div>
          )}
        </fieldset>

        {submitError && (
          <div className="rounded-lg border border-red-200 bg-red-50 dark:border-red-900/50 dark:bg-red-950/30 px-4 py-3 text-sm text-red-700 dark:text-red-400">
            {submitError}
          </div>
        )}

        <div className="flex items-center gap-3 pt-1">
          <button
            type="submit"
            disabled={submitLoading}
            className="rounded-lg bg-violet-600 px-5 py-2.5 text-sm font-semibold text-white hover:bg-violet-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {submitLoading ? 'Criando...' : 'Criar loja'}
          </button>
          <Link
            href="/admin/lojas"
            className="px-4 py-2.5 text-sm font-medium text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 transition-colors"
          >
            Cancelar
          </Link>
        </div>
      </form>
    </div>
  )
}
