'use client'

import { useState, useEffect, useRef } from 'react'
import { useRouter } from 'next/navigation'
import Link from 'next/link'
import { createStore, lookupCNPJ, searchUsers, uploadStoreLogo } from '@/lib/stores-admin'
import type { UserSummary } from '@/lib/stores-admin'

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
  if (d.length <= 5) return `${d.slice(0, 2)}.${d.slice(2)}`
  if (d.length <= 8) return `${d.slice(0, 2)}.${d.slice(2, 5)}.${d.slice(5)}`
  if (d.length <= 12) return `${d.slice(0, 2)}.${d.slice(2, 5)}.${d.slice(5, 8)}/${d.slice(8)}`
  return `${d.slice(0, 2)}.${d.slice(2, 5)}.${d.slice(5, 8)}/${d.slice(8, 12)}-${d.slice(12)}`
}

function maskCPF(v: string) {
  const d = v.replace(/\D/g, '').slice(0, 11)
  if (d.length <= 3) return d
  if (d.length <= 6) return `${d.slice(0, 3)}.${d.slice(3)}`
  if (d.length <= 9) return `${d.slice(0, 3)}.${d.slice(3, 6)}.${d.slice(6)}`
  return `${d.slice(0, 3)}.${d.slice(3, 6)}.${d.slice(6, 9)}-${d.slice(9)}`
}

function maskCEP(v: string) {
  const d = v.replace(/\D/g, '').slice(0, 8)
  if (d.length <= 5) return d
  return `${d.slice(0, 5)}-${d.slice(5)}`
}

function maskPhone(v: string) {
  const d = v.replace(/\D/g, '').slice(0, 11)
  if (!d) return ''
  if (d.length <= 2) return `(${d}`
  if (d.length <= 6) return `(${d.slice(0, 2)}) ${d.slice(2)}`
  if (d.length <= 10) return `(${d.slice(0, 2)}) ${d.slice(2, 6)}-${d.slice(6)}`
  return `(${d.slice(0, 2)}) ${d.slice(2, 7)}-${d.slice(7)}`
}

const BR_STATES = [
  'AC', 'AL', 'AP', 'AM', 'BA', 'CE', 'DF', 'ES', 'GO', 'MA',
  'MT', 'MS', 'MG', 'PA', 'PB', 'PR', 'PE', 'PI', 'RJ', 'RN',
  'RS', 'RO', 'RR', 'SC', 'SP', 'SE', 'TO',
]

const inputCls = 'w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500'
const labelCls = 'block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1.5'
const sectionTitleCls = 'text-xs font-semibold uppercase tracking-widest text-zinc-400 mb-4'

export default function NovaLojaPage() {
  const router = useRouter()

  // Logo
  const [logoFile, setLogoFile] = useState<File | null>(null)
  const [logoPreview, setLogoPreview] = useState<string | null>(null)

  // Owner search
  const [ownerQuery, setOwnerQuery] = useState('')
  const [ownerResults, setOwnerResults] = useState<UserSummary[]>([])
  const [ownerDropOpen, setOwnerDropOpen] = useState(false)
  const [ownerSearchLoading, setOwnerSearchLoading] = useState(false)
  const ownerRef = useRef<HTMLDivElement>(null)

  // Form state
  const [form, setForm] = useState({
    ownerID: '',
    ownerLabel: '',
    name: '',
    slug: '',
    description: '',
    documentType: 'cnpj' as 'cpf' | 'cnpj',
    documentNumber: '',
    legalName: '',
    tradeName: '',
    phone: '',
    addressZip: '',
    addressStreet: '',
    addressNumber: '',
    addressComplement: '',
    addressNeighborhood: '',
    addressCity: '',
    addressState: '',
  })
  const [slugManual, setSlugManual] = useState(false)
  const [lookupLoading, setLookupLoading] = useState(false)
  const [lookupError, setLookupError] = useState('')
  const [cepLoading, setCepLoading] = useState(false)
  const [submitLoading, setSubmitLoading] = useState(false)
  const [submitError, setSubmitError] = useState('')

  function set<K extends keyof typeof form>(key: K, value: typeof form[K]) {
    setForm(prev => ({ ...prev, [key]: value }))
  }

  function handleLogoChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0] ?? null
    setLogoFile(file)
    setLogoPreview(file ? URL.createObjectURL(file) : null)
  }

  function removeLogo() {
    setLogoFile(null)
    setLogoPreview(null)
  }

  function handleNameChange(v: string) {
    set('name', v)
    if (!slugManual) set('slug', slugify(v))
  }

  function handleDocNumberChange(raw: string) {
    const masked = form.documentType === 'cnpj' ? maskCNPJ(raw) : maskCPF(raw)
    set('documentNumber', masked)
    if (form.documentType === 'cnpj') {
      set('legalName', '')
      setLookupError('')
    }
  }

  // Close owner dropdown on outside click
  useEffect(() => {
    function handle(e: MouseEvent) {
      if (ownerRef.current && !ownerRef.current.contains(e.target as Node)) {
        setOwnerDropOpen(false)
      }
    }
    document.addEventListener('mousedown', handle)
    return () => document.removeEventListener('mousedown', handle)
  }, [])

  // Owner search debounce
  useEffect(() => {
    if (ownerQuery.length < 2) {
      setOwnerResults([])
      setOwnerDropOpen(false)
      return
    }
    const timer = setTimeout(async () => {
      setOwnerSearchLoading(true)
      try {
        const results = await searchUsers(ownerQuery)
        setOwnerResults(results)
        setOwnerDropOpen(results.length > 0)
      } catch {
        setOwnerResults([])
        setOwnerDropOpen(false)
      } finally {
        setOwnerSearchLoading(false)
      }
    }, 300)
    return () => clearTimeout(timer)
  }, [ownerQuery])

  function selectOwner(u: UserSummary) {
    setForm(prev => ({ ...prev, ownerID: u.id, ownerLabel: `${u.display_name} · ${u.email}` }))
    setOwnerQuery(u.email)
    setOwnerDropOpen(false)
  }

  function clearOwner() {
    setForm(prev => ({ ...prev, ownerID: '', ownerLabel: '' }))
    setOwnerQuery('')
  }

  async function handleCNPJLookup() {
    setLookupLoading(true)
    setLookupError('')
    try {
      const info = await lookupCNPJ(form.documentNumber)
      setForm(prev => ({
        ...prev,
        legalName: info.legal_name,
        tradeName: info.trade_name || prev.tradeName,
        phone: info.phone ? maskPhone(info.phone) : prev.phone,
        addressZip: info.address_zip ? maskCEP(info.address_zip) : prev.addressZip,
        addressStreet: info.address_street || prev.addressStreet,
        addressNumber: info.address_number || prev.addressNumber,
        addressComplement: info.address_complement || prev.addressComplement,
        addressNeighborhood: info.address_neighborhood || prev.addressNeighborhood,
        addressCity: info.address_city || prev.addressCity,
        addressState: info.address_state || prev.addressState,
      }))
      if (info.situation !== 'ATIVA') {
        setLookupError(`Situação do CNPJ: ${info.situation} (não está ativa)`)
      }
    } catch (e) {
      setLookupError(e instanceof Error ? e.message : 'Erro na consulta')
    } finally {
      setLookupLoading(false)
    }
  }

  async function fetchCEP(digits: string) {
    setCepLoading(true)
    try {
      const res = await fetch(`https://viacep.com.br/ws/${digits}/json/`)
      const data = await res.json()
      if (!data.erro) {
        setForm(prev => ({
          ...prev,
          addressStreet: data.logradouro || prev.addressStreet,
          addressNeighborhood: data.bairro || prev.addressNeighborhood,
          addressCity: data.localidade || prev.addressCity,
          addressState: data.uf || prev.addressState,
        }))
      }
    } catch {
      // silent — user fills manually
    } finally {
      setCepLoading(false)
    }
  }

  // Auto-lookup CEP when 8 digits entered and street is empty
  useEffect(() => {
    const digits = form.addressZip.replace(/\D/g, '')
    if (digits.length === 8 && !form.addressStreet) {
      fetchCEP(digits)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [form.addressZip])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!form.ownerID) {
      setSubmitError('Selecione um proprietário pelo e-mail')
      return
    }
    const phoneDigits = form.phone.replace(/\D/g, '')
    if (phoneDigits.length < 10) {
      setSubmitError('Telefone inválido — informe DDD + número')
      return
    }
    setSubmitLoading(true)
    setSubmitError('')
    try {
      const created = await createStore({
        owner_id: form.ownerID,
        name: form.name,
        slug: form.slug,
        description: form.description || undefined,
        document_type: form.documentType,
        document_number: form.documentNumber,
        trade_name: form.tradeName || undefined,
        phone: phoneDigits,
        address_zip: form.addressZip.replace(/\D/g, '') || undefined,
        address_street: form.addressStreet || undefined,
        address_number: form.addressNumber || undefined,
        address_complement: form.addressComplement || undefined,
        address_neighborhood: form.addressNeighborhood || undefined,
        address_city: form.addressCity || undefined,
        address_state: form.addressState || undefined,
      })
      if (logoFile) {
        try {
          await uploadStoreLogo(created.id, logoFile)
        } catch {
          // Logo opcional — loja já foi criada, ignorar falha no upload
        }
      }
      router.push('/admin/lojas')
    } catch (e) {
      setSubmitError(e instanceof Error ? e.message : 'Erro ao criar loja')
    } finally {
      setSubmitLoading(false)
    }
  }

  const docPlaceholder = form.documentType === 'cnpj' ? '00.000.000/0000-00' : '000.000.000-00'
  const docDigits = form.documentNumber.replace(/\D/g, '')
  const docReady = form.documentType === 'cnpj' ? docDigits.length === 14 : docDigits.length === 11
  const cepDigits = form.addressZip.replace(/\D/g, '')

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

      <form onSubmit={handleSubmit} className="flex flex-col gap-8">

        {/* ── PROPRIETÁRIO ── */}
        <section>
          <h2 className={sectionTitleCls}>Proprietário</h2>
          <div ref={ownerRef} className="relative">
            <label className={labelCls}>
              Buscar por e-mail <span className="text-red-500">*</span>
            </label>
            <div className="relative">
              <input
                type="text"
                value={ownerQuery}
                onChange={e => {
                  setOwnerQuery(e.target.value)
                  if (form.ownerID) clearOwner()
                }}
                placeholder="Digite o e-mail do proprietário..."
                className={inputCls}
                autoComplete="off"
              />
              {ownerSearchLoading && (
                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-zinc-400">
                  Buscando...
                </span>
              )}
            </div>

            {ownerDropOpen && ownerResults.length > 0 && (
              <div className="absolute z-10 mt-1 w-full rounded-lg border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 shadow-lg overflow-hidden">
                {ownerResults.map(u => (
                  <button
                    key={u.id}
                    type="button"
                    onClick={() => selectOwner(u)}
                    className="w-full px-4 py-2.5 text-left hover:bg-zinc-50 dark:hover:bg-zinc-800 transition-colors border-b border-zinc-100 dark:border-zinc-800 last:border-0"
                  >
                    <p className="text-sm font-medium text-zinc-900 dark:text-zinc-100">{u.display_name}</p>
                    <p className="text-xs text-zinc-500">{u.email}</p>
                  </button>
                ))}
              </div>
            )}

            {form.ownerLabel && (
              <div className="mt-2 inline-flex items-center gap-2 rounded-full bg-violet-50 dark:bg-violet-900/20 border border-violet-200 dark:border-violet-800 px-3 py-1">
                <span className="text-xs font-medium text-violet-700 dark:text-violet-300">{form.ownerLabel}</span>
                <button
                  type="button"
                  onClick={clearOwner}
                  className="text-violet-400 hover:text-violet-700 dark:hover:text-violet-200 leading-none text-sm"
                >
                  ×
                </button>
              </div>
            )}
          </div>
        </section>

        {/* ── IDENTIDADE DA LOJA ── */}
        <section>
          <h2 className={sectionTitleCls}>Identidade da Loja</h2>
          <div className="flex flex-col gap-4">
            {/* Logo */}
            <div>
              <label className={labelCls}>Logo</label>
              <div className="flex items-start gap-4">
                {logoPreview ? (
                  <div className="relative w-24 h-24 rounded-xl overflow-hidden border border-zinc-200 dark:border-zinc-700 flex-shrink-0 bg-zinc-50 dark:bg-zinc-800">
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img src={logoPreview} alt="preview do logo" className="w-full h-full object-cover" />
                    <button
                      type="button"
                      onClick={removeLogo}
                      className="absolute top-1 right-1 w-5 h-5 rounded-full bg-black/60 text-white text-xs flex items-center justify-center hover:bg-black/80 transition-colors leading-none"
                    >
                      ×
                    </button>
                  </div>
                ) : (
                  <label className="w-24 h-24 rounded-xl border-2 border-dashed border-zinc-300 dark:border-zinc-700 flex flex-col items-center justify-center cursor-pointer hover:border-violet-400 dark:hover:border-violet-500 transition-colors flex-shrink-0 bg-zinc-50 dark:bg-zinc-900">
                    <span className="text-2xl text-zinc-300 dark:text-zinc-600 select-none">+</span>
                    <span className="text-[11px] text-zinc-400 mt-1 select-none">Logo</span>
                    <input
                      type="file"
                      accept="image/jpeg,image/png,image/webp,image/gif"
                      onChange={handleLogoChange}
                      className="sr-only"
                    />
                  </label>
                )}
                <div className="pt-1.5">
                  <p className="text-xs text-zinc-500 dark:text-zinc-400">
                    Formatos aceitos: JPG, PNG, WebP, GIF
                  </p>
                  <p className="text-xs text-zinc-400 dark:text-zinc-500 mt-0.5">Tamanho máximo: 5 MB</p>
                  {logoFile && (
                    <p className="text-xs text-violet-600 dark:text-violet-400 mt-1 font-medium">
                      {logoFile.name}
                    </p>
                  )}
                </div>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className={labelCls}>Nome da loja <span className="text-red-500">*</span></label>
                <input
                  type="text"
                  required
                  value={form.name}
                  onChange={e => handleNameChange(e.target.value)}
                  className={inputCls}
                  placeholder="Ex: Cards do João"
                />
              </div>
              <div>
                <label className={labelCls}>Slug (URL) <span className="text-red-500">*</span></label>
                <input
                  type="text"
                  required
                  value={form.slug}
                  onChange={e => { setSlugManual(true); set('slug', e.target.value) }}
                  className={`${inputCls} font-mono`}
                  placeholder="cards-do-joao"
                />
                <p className="mt-1 text-xs text-zinc-400">Gerado do nome. Editável.</p>
              </div>
            </div>
            <div>
              <label className={labelCls}>Descrição</label>
              <textarea
                value={form.description}
                onChange={e => set('description', e.target.value)}
                rows={2}
                className={`${inputCls} resize-none`}
                placeholder="Descrição pública da loja (opcional)"
              />
            </div>
          </div>
        </section>

        {/* ── DOCUMENTO FISCAL ── */}
        <section>
          <h2 className={sectionTitleCls}>
            Documento Fiscal <span className="text-red-500 normal-case tracking-normal font-normal text-xs">*</span>
          </h2>
          <fieldset className="rounded-xl border border-zinc-200 dark:border-zinc-800 p-4">
            <div className="flex gap-5 mb-4">
              {(['cnpj', 'cpf'] as const).map(t => (
                <label key={t} className="flex items-center gap-2 cursor-pointer select-none">
                  <input
                    type="radio"
                    name="docType"
                    value={t}
                    checked={form.documentType === t}
                    onChange={() => {
                      setForm(prev => ({ ...prev, documentType: t, documentNumber: '', legalName: '' }))
                      setLookupError('')
                    }}
                    className="accent-violet-600"
                  />
                  <span className="text-sm font-semibold text-zinc-700 dark:text-zinc-300 uppercase">{t}</span>
                </label>
              ))}
            </div>

            <div className="flex flex-col gap-3">
              <div className="flex gap-2">
                <input
                  type="text"
                  required
                  value={form.documentNumber}
                  onChange={e => handleDocNumberChange(e.target.value)}
                  placeholder={docPlaceholder}
                  className={`${inputCls} flex-1 font-mono`}
                />
                {form.documentType === 'cnpj' && (
                  <button
                    type="button"
                    onClick={handleCNPJLookup}
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
                  <p className="text-xs text-green-600 dark:text-green-400 font-medium">Razão social</p>
                  <p className="text-sm text-green-800 dark:text-green-200 font-semibold mt-0.5">{form.legalName}</p>
                </div>
              )}

              {form.documentType === 'cpf' && docReady && (
                <p className="text-xs text-zinc-400">
                  CPF requer revisão manual — status <strong>Pendente</strong> até a verificação pelo admin.
                </p>
              )}
            </div>
          </fieldset>
        </section>

        {/* ── DADOS COMERCIAIS ── */}
        <section>
          <h2 className={sectionTitleCls}>Dados Comerciais</h2>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className={labelCls}>Nome Fantasia</label>
              <input
                type="text"
                value={form.tradeName}
                onChange={e => set('tradeName', e.target.value)}
                className={inputCls}
                placeholder="Nome público da empresa"
              />
              <p className="mt-1 text-xs text-zinc-400">Preenchido automaticamente via CNPJ.</p>
            </div>
            <div>
              <label className={labelCls}>Telefone <span className="text-red-500">*</span></label>
              <input
                type="text"
                required
                value={form.phone}
                onChange={e => set('phone', maskPhone(e.target.value))}
                className={inputCls}
                placeholder="(00) 00000-0000"
              />
            </div>
          </div>
        </section>

        {/* ── ENDEREÇO ── */}
        <section>
          <h2 className={sectionTitleCls}>Endereço</h2>
          <div className="flex flex-col gap-4">
            <div className="flex items-end gap-2">
              <div className="w-44">
                <label className={labelCls}>CEP <span className="text-red-500">*</span></label>
                <input
                  type="text"
                  required
                  value={form.addressZip}
                  onChange={e => set('addressZip', maskCEP(e.target.value))}
                  className={`${inputCls} font-mono`}
                  placeholder="00000-000"
                />
              </div>
              <button
                type="button"
                onClick={() => fetchCEP(cepDigits)}
                disabled={cepDigits.length !== 8 || cepLoading}
                className="mb-0 rounded-lg bg-zinc-100 dark:bg-zinc-800 border border-zinc-300 dark:border-zinc-700 px-3 py-2 text-sm font-medium text-zinc-700 dark:text-zinc-200 hover:bg-zinc-200 dark:hover:bg-zinc-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors whitespace-nowrap"
              >
                {cepLoading ? 'Buscando...' : 'Buscar CEP'}
              </button>
            </div>

            <div className="grid grid-cols-3 gap-4">
              <div className="col-span-2">
                <label className={labelCls}>Logradouro <span className="text-red-500">*</span></label>
                <input
                  type="text"
                  required
                  value={form.addressStreet}
                  onChange={e => set('addressStreet', e.target.value)}
                  className={inputCls}
                  placeholder="Rua, Av., etc."
                />
              </div>
              <div>
                <label className={labelCls}>Número <span className="text-red-500">*</span></label>
                <input
                  type="text"
                  required
                  value={form.addressNumber}
                  onChange={e => set('addressNumber', e.target.value)}
                  className={inputCls}
                  placeholder="123"
                />
              </div>
            </div>

            <div>
              <label className={labelCls}>Complemento</label>
              <input
                type="text"
                value={form.addressComplement}
                onChange={e => set('addressComplement', e.target.value)}
                className={inputCls}
                placeholder="Sala, Andar, Galpão... (opcional)"
              />
            </div>

            <div className="grid grid-cols-3 gap-4">
              <div>
                <label className={labelCls}>Bairro <span className="text-red-500">*</span></label>
                <input
                  type="text"
                  required
                  value={form.addressNeighborhood}
                  onChange={e => set('addressNeighborhood', e.target.value)}
                  className={inputCls}
                  placeholder="Bairro"
                />
              </div>
              <div>
                <label className={labelCls}>Cidade <span className="text-red-500">*</span></label>
                <input
                  type="text"
                  required
                  value={form.addressCity}
                  onChange={e => set('addressCity', e.target.value)}
                  className={inputCls}
                  placeholder="Cidade"
                />
              </div>
              <div>
                <label className={labelCls}>UF <span className="text-red-500">*</span></label>
                <select
                  required
                  value={form.addressState}
                  onChange={e => set('addressState', e.target.value)}
                  className={inputCls}
                >
                  <option value="">—</option>
                  {BR_STATES.map(uf => (
                    <option key={uf} value={uf}>{uf}</option>
                  ))}
                </select>
              </div>
            </div>
          </div>
        </section>

        {submitError && (
          <div className="rounded-lg border border-red-200 bg-red-50 dark:border-red-900/50 dark:bg-red-950/30 px-4 py-3 text-sm text-red-700 dark:text-red-400">
            {submitError}
          </div>
        )}

        <div className="flex items-center gap-3 pt-1 border-t border-zinc-200 dark:border-zinc-800">
          <button
            type="submit"
            disabled={submitLoading || !form.ownerID}
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
