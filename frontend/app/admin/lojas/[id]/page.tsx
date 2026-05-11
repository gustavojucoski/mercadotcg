'use client'

import { useEffect, useRef, useState } from 'react'
import { useParams } from 'next/navigation'
import Link from 'next/link'
import {
  getStore,
  updateStore,
  verifyDocument,
  uploadStoreLogo,
  getAuditLog,
  listStoreMembers,
  addStoreMember,
  removeStoreMember,
  updateStoreMemberRole,
  searchUsers,
  AdminStore,
  AuditEntry,
  StoreMemberRow,
  UserSummary,
} from '@/lib/stores-admin'

// ── helpers ──────────────────────────────────────────────────────────────────

function formatDoc(type?: string, number?: string) {
  if (!type || !number) return '—'
  if (type === 'cnpj' && number.length === 14)
    return `${number.slice(0, 2)}.${number.slice(2, 5)}.${number.slice(5, 8)}/${number.slice(8, 12)}-${number.slice(12)}`
  if (type === 'cpf' && number.length === 11)
    return `${number.slice(0, 3)}.${number.slice(3, 6)}.${number.slice(6, 9)}-${number.slice(9)}`
  return number
}

function maskCEP(v: string) {
  const d = v.replace(/\D/g, '').slice(0, 8)
  return d.length <= 5 ? d : `${d.slice(0, 5)}-${d.slice(5)}`
}

function maskPhone(v: string) {
  const d = v.replace(/\D/g, '').slice(0, 11)
  if (!d) return ''
  if (d.length <= 2) return `(${d}`
  if (d.length <= 6) return `(${d.slice(0, 2)}) ${d.slice(2)}`
  if (d.length <= 10) return `(${d.slice(0, 2)}) ${d.slice(2, 6)}-${d.slice(6)}`
  return `(${d.slice(0, 2)}) ${d.slice(2, 7)}-${d.slice(7)}`
}

function slugify(s: string) {
  return s
    .toLowerCase()
    .normalize('NFD')
    .replace(/[̀-ͯ]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

function formatTs(iso: string) {
  return new Date(iso).toLocaleString('pt-BR', {
    day: '2-digit', month: '2-digit', year: 'numeric',
    hour: '2-digit', minute: '2-digit',
  })
}

function changeTypeLabel(t: string) {
  const map: Record<string, string> = {
    admin_update: 'Edição (admin)',
    store_update: 'Edição (loja)',
    logo_upload: 'Upload de logo',
    document_verified: 'Verificação de documento',
    member_added: 'Membro adicionado',
    member_removed: 'Membro removido',
    member_role_changed: 'Role de membro alterada',
  }
  return map[t] ?? t
}

// ── style tokens ─────────────────────────────────────────────────────────────

const inputCls =
  'w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 focus:outline-none focus:ring-2 focus:ring-violet-500'
const labelCls = 'block text-sm font-medium text-zinc-700 dark:text-zinc-300 mb-1.5'
const sectionCls = 'rounded-xl border border-zinc-200 dark:border-zinc-800 p-5 space-y-4'
const sectionTitleCls = 'text-xs font-semibold uppercase tracking-widest text-zinc-400'
const btnPrimary =
  'rounded-lg bg-violet-600 px-4 py-2 text-sm font-semibold text-white hover:bg-violet-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors'
const btnSecondary =
  'rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-4 py-2 text-sm font-medium text-zinc-700 dark:text-zinc-200 hover:bg-zinc-50 dark:hover:bg-zinc-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors'
const alertError =
  'rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400'
const alertSuccess =
  'rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-700 dark:border-green-900/50 dark:bg-green-950/30 dark:text-green-400'

const docStatusLabel: Record<AdminStore['document_status'], string> = {
  pending: 'Pendente',
  auto_verified: 'Verificado (auto)',
  manually_verified: 'Verificado (manual)',
}

const docStatusClass: Record<AdminStore['document_status'], string> = {
  pending: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400',
  auto_verified: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
  manually_verified: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
}

const BR_STATES = [
  'AC', 'AL', 'AP', 'AM', 'BA', 'CE', 'DF', 'ES', 'GO', 'MA',
  'MT', 'MS', 'MG', 'PA', 'PB', 'PR', 'PE', 'PI', 'RJ', 'RN',
  'RS', 'RO', 'RR', 'SC', 'SP', 'SE', 'TO',
]

type Tab = 'dados' | 'documento' | 'endereco' | 'membros' | 'auditoria'

const TABS: { id: Tab; label: string }[] = [
  { id: 'dados', label: 'Dados' },
  { id: 'documento', label: 'Documento' },
  { id: 'endereco', label: 'Endereço' },
  { id: 'membros', label: 'Membros' },
  { id: 'auditoria', label: 'Auditoria' },
]

// ── DadosTab ─────────────────────────────────────────────────────────────────

interface DadosTabProps {
  store: AdminStore
  onSaved: (s: AdminStore) => void
}

function DadosTab({ store, onSaved }: DadosTabProps) {
  const [name, setName] = useState(store.name)
  const [slug, setSlug] = useState(store.slug)
  const [slugManual, setSlugManual] = useState(false)
  const [description, setDescription] = useState(store.description ?? '')
  const [isActive, setIsActive] = useState(store.is_active)
  const [tradeName, setTradeName] = useState(store.trade_name ?? '')

  // Owner search
  const [ownerQuery, setOwnerQuery] = useState(store.owner_id)
  const [ownerResults, setOwnerResults] = useState<UserSummary[]>([])
  const [ownerDropOpen, setOwnerDropOpen] = useState(false)
  const [ownerSearchLoading, setOwnerSearchLoading] = useState(false)
  const [selectedOwnerId, setSelectedOwnerId] = useState(store.owner_id)
  const [ownerLabel, setOwnerLabel] = useState('')
  const ownerRef = useRef<HTMLDivElement>(null)

  // Logo
  const [logoFile, setLogoFile] = useState<File | null>(null)
  const [logoPreview, setLogoPreview] = useState<string | null>(store.logo_url ?? null)

  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

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

  // Owner search debounce — 350ms
  useEffect(() => {
    if (ownerQuery.length < 2 || ownerLabel) {
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
    }, 350)
    return () => clearTimeout(timer)
  }, [ownerQuery, ownerLabel])

  function selectOwner(u: UserSummary) {
    setSelectedOwnerId(u.id)
    setOwnerLabel(`${u.display_name} · ${u.email}`)
    setOwnerQuery(u.email)
    setOwnerDropOpen(false)
  }

  function clearOwner() {
    setSelectedOwnerId(store.owner_id)
    setOwnerLabel('')
    setOwnerQuery(store.owner_id)
  }

  function handleNameChange(v: string) {
    setName(v)
    if (!slugManual) setSlug(slugify(v))
  }

  function handleLogoChange(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0] ?? null
    setLogoFile(file)
    setLogoPreview(file ? URL.createObjectURL(file) : store.logo_url ?? null)
  }

  async function handleSave(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError('')
    setSuccess('')
    try {
      let updated = await updateStore(store.id, {
        name,
        slug,
        description: description || undefined,
        is_active: isActive,
        trade_name: tradeName || undefined,
        owner_id: selectedOwnerId !== store.owner_id ? selectedOwnerId : undefined,
      })
      if (logoFile) {
        updated = await uploadStoreLogo(store.id, logoFile)
        setLogoFile(null)
      }
      onSaved(updated)
      setSuccess('Dados salvos com sucesso.')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Erro ao salvar')
    } finally {
      setSaving(false)
    }
  }

  const ownerChanged = selectedOwnerId !== store.owner_id

  return (
    <form onSubmit={handleSave} className="space-y-6">
      {/* Logo */}
      <div className={sectionCls}>
        <h2 className={sectionTitleCls}>Logo</h2>
        <div className="flex items-start gap-4">
          {logoPreview ? (
            <div className="relative w-24 h-24 rounded-xl overflow-hidden border border-zinc-200 dark:border-zinc-700 flex-shrink-0 bg-zinc-50 dark:bg-zinc-800">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src={logoPreview} alt="logo" className="w-full h-full object-cover" />
            </div>
          ) : (
            <div className="w-24 h-24 rounded-xl border-2 border-dashed border-zinc-300 dark:border-zinc-700 flex items-center justify-center text-zinc-400 text-xs flex-shrink-0">
              Sem logo
            </div>
          )}
          <div className="pt-1 space-y-1">
            <label className="cursor-pointer text-sm text-violet-600 hover:text-violet-700 dark:text-violet-400 font-medium">
              {logoPreview ? 'Alterar logo' : 'Enviar logo'}
              <input type="file" accept="image/jpeg,image/png,image/webp,image/gif" onChange={handleLogoChange} className="sr-only" />
            </label>
            {logoFile && <p className="text-xs text-zinc-500">{logoFile.name}</p>}
            <p className="text-xs text-zinc-400">JPG, PNG, WebP, GIF — max 5 MB</p>
          </div>
        </div>
      </div>

      {/* Identidade */}
      <div className={sectionCls}>
        <h2 className={sectionTitleCls}>Identidade</h2>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className={labelCls}>Nome da loja</label>
            <input
              type="text"
              required
              value={name}
              onChange={e => handleNameChange(e.target.value)}
              className={inputCls}
            />
          </div>
          <div>
            <label className={labelCls}>Slug (URL)</label>
            <input
              type="text"
              required
              value={slug}
              onChange={e => { setSlugManual(true); setSlug(e.target.value) }}
              className={`${inputCls} font-mono`}
            />
          </div>
          <div className="col-span-2">
            <label className={labelCls}>Nome Fantasia</label>
            <input
              type="text"
              value={tradeName}
              onChange={e => setTradeName(e.target.value)}
              className={inputCls}
              placeholder="Nome público da empresa"
            />
          </div>
          <div className="col-span-2">
            <label className={labelCls}>Descrição</label>
            <textarea
              value={description}
              onChange={e => setDescription(e.target.value)}
              rows={3}
              className={`${inputCls} resize-none`}
              placeholder="Descrição pública da loja (opcional)"
            />
          </div>
        </div>
        <div className="flex items-center gap-3 pt-1">
          <label className="relative inline-flex items-center cursor-pointer select-none">
            <input
              type="checkbox"
              checked={isActive}
              onChange={e => setIsActive(e.target.checked)}
              className="sr-only peer"
            />
            <div className="w-10 h-5 rounded-full bg-zinc-300 dark:bg-zinc-600 peer-checked:bg-violet-600 transition-colors after:content-[''] after:absolute after:top-0.5 after:left-0.5 after:w-4 after:h-4 after:rounded-full after:bg-white after:transition-transform peer-checked:after:translate-x-5" />
          </label>
          <span className="text-sm text-zinc-700 dark:text-zinc-300">Loja ativa</span>
        </div>
      </div>

      {/* Proprietário */}
      <div className={sectionCls}>
        <h2 className={sectionTitleCls}>Proprietário</h2>
        <p className="text-xs text-zinc-400">
          ID atual: <span className="font-mono">{store.owner_id}</span>
          {ownerChanged && <span className="ml-2 text-amber-500">(será alterado ao salvar)</span>}
        </p>
        <div ref={ownerRef} className="relative">
          <label className={labelCls}>Buscar por e-mail para transferir</label>
          <div className="relative">
            <input
              type="text"
              value={ownerQuery}
              onChange={e => { setOwnerQuery(e.target.value); if (ownerLabel) clearOwner() }}
              placeholder="Digite o e-mail do novo proprietário..."
              className={inputCls}
              autoComplete="off"
            />
            {ownerSearchLoading && (
              <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-zinc-400">Buscando...</span>
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
          {ownerLabel && (
            <div className="mt-2 inline-flex items-center gap-2 rounded-full bg-violet-50 dark:bg-violet-900/20 border border-violet-200 dark:border-violet-800 px-3 py-1">
              <span className="text-xs font-medium text-violet-700 dark:text-violet-300">{ownerLabel}</span>
              <button type="button" onClick={clearOwner} className="text-violet-400 hover:text-violet-700 dark:hover:text-violet-200 text-sm leading-none">×</button>
            </div>
          )}
        </div>
      </div>

      {error && <div className={alertError}>{error}</div>}
      {success && <div className={alertSuccess}>{success}</div>}

      <div className="flex items-center gap-3">
        <button type="submit" disabled={saving} className={btnPrimary}>
          {saving ? 'Salvando...' : 'Salvar alterações'}
        </button>
      </div>
    </form>
  )
}

// ── DocumentoTab ─────────────────────────────────────────────────────────────

interface DocumentoTabProps {
  store: AdminStore
  onSaved: (s: AdminStore) => void
}

function DocumentoTab({ store, onSaved }: DocumentoTabProps) {
  const [legalName, setLegalName] = useState(store.legal_name ?? '')
  const [verifying, setVerifying] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  async function handleVerify() {
    setVerifying(true)
    setError('')
    setSuccess('')
    try {
      const updated = await verifyDocument(store.id)
      onSaved(updated)
      setSuccess('Documento verificado manualmente com sucesso.')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Erro ao verificar')
    } finally {
      setVerifying(false)
    }
  }

  async function handleSaveLegalName(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError('')
    setSuccess('')
    try {
      const updated = await updateStore(store.id, { legal_name: legalName || null })
      onSaved(updated)
      setSuccess('Razão social atualizada.')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Erro ao salvar')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="space-y-6">
      <div className={sectionCls}>
        <h2 className={sectionTitleCls}>Documento Fiscal</h2>
        <div className="grid grid-cols-2 gap-4">
          <div>
            <p className="text-xs text-zinc-500 mb-1">Tipo</p>
            <p className="text-sm font-semibold text-zinc-900 dark:text-zinc-100 uppercase">
              {store.document_type ?? '—'}
            </p>
          </div>
          <div>
            <p className="text-xs text-zinc-500 mb-1">Número</p>
            <p className="text-sm font-mono text-zinc-900 dark:text-zinc-100">
              {formatDoc(store.document_type, store.document_number)}
            </p>
          </div>
          <div>
            <p className="text-xs text-zinc-500 mb-1">Status</p>
            <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${docStatusClass[store.document_status]}`}>
              {docStatusLabel[store.document_status]}
            </span>
          </div>
          {store.document_verified_at && (
            <div>
              <p className="text-xs text-zinc-500 mb-1">Verificado em</p>
              <p className="text-sm text-zinc-700 dark:text-zinc-300">{formatTs(store.document_verified_at)}</p>
            </div>
          )}
        </div>

        {store.document_status === 'pending' && (
          <div className="pt-1 border-t border-zinc-100 dark:border-zinc-800">
            <button type="button" onClick={handleVerify} disabled={verifying} className={btnPrimary}>
              {verifying ? 'Verificando...' : 'Verificar manualmente'}
            </button>
            <p className="mt-2 text-xs text-zinc-400">
              Marca o documento como <strong>manualmente verificado</strong> e registra quem aprovou no log de auditoria.
            </p>
          </div>
        )}
      </div>

      <form onSubmit={handleSaveLegalName} className={`${sectionCls} space-y-4`}>
        <h2 className={sectionTitleCls}>Razão Social</h2>
        <div>
          <label className={labelCls}>Razão social</label>
          <input
            type="text"
            value={legalName}
            onChange={e => setLegalName(e.target.value)}
            className={inputCls}
            placeholder="Preenchida automaticamente via consulta CNPJ"
          />
        </div>
        <button type="submit" disabled={saving} className={btnPrimary}>
          {saving ? 'Salvando...' : 'Salvar razão social'}
        </button>
      </form>

      {error && <div className={alertError}>{error}</div>}
      {success && <div className={alertSuccess}>{success}</div>}
    </div>
  )
}

// ── EnderecoTab ───────────────────────────────────────────────────────────────

interface EnderecoTabProps {
  store: AdminStore
  onSaved: (s: AdminStore) => void
}

function EnderecoTab({ store, onSaved }: EnderecoTabProps) {
  const [phone, setPhone] = useState(store.phone ? maskPhone(store.phone) : '')
  const [zip, setZip] = useState(store.address_zip ? maskCEP(store.address_zip) : '')
  const [street, setStreet] = useState(store.address_street ?? '')
  const [number, setNumber] = useState(store.address_number ?? '')
  const [complement, setComplement] = useState(store.address_complement ?? '')
  const [neighborhood, setNeighborhood] = useState(store.address_neighborhood ?? '')
  const [city, setCity] = useState(store.address_city ?? '')
  const [state, setState] = useState(store.address_state ?? '')
  const [cepLoading, setCepLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  const cepDigits = zip.replace(/\D/g, '')

  async function fetchCEP(digits: string) {
    setCepLoading(true)
    try {
      const res = await fetch(`https://viacep.com.br/ws/${digits}/json/`)
      const data = await res.json()
      if (!data.erro) {
        if (data.logradouro) setStreet(data.logradouro)
        if (data.bairro) setNeighborhood(data.bairro)
        if (data.localidade) setCity(data.localidade)
        if (data.uf) setState(data.uf)
      }
    } catch {
      // silent — user fills manually
    } finally {
      setCepLoading(false)
    }
  }

  useEffect(() => {
    if (cepDigits.length === 8 && !street) fetchCEP(cepDigits)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [zip])

  async function handleSave(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError('')
    setSuccess('')
    try {
      const updated = await updateStore(store.id, {
        phone: phone.replace(/\D/g, '') || undefined,
        address_zip: cepDigits || undefined,
        address_street: street || undefined,
        address_number: number || undefined,
        address_complement: complement || undefined,
        address_neighborhood: neighborhood || undefined,
        address_city: city || undefined,
        address_state: state || undefined,
      })
      onSaved(updated)
      setSuccess('Endereço salvo com sucesso.')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Erro ao salvar')
    } finally {
      setSaving(false)
    }
  }

  return (
    <form onSubmit={handleSave} className="space-y-6">
      <div className={sectionCls}>
        <h2 className={sectionTitleCls}>Contato &amp; Endereço</h2>

        <div className="grid grid-cols-2 gap-4">
          <div className="col-span-2 max-w-xs">
            <label className={labelCls}>Telefone</label>
            <input
              type="text"
              value={phone}
              onChange={e => setPhone(maskPhone(e.target.value))}
              className={inputCls}
              placeholder="(00) 00000-0000"
            />
          </div>
        </div>

        <div className="flex items-end gap-2">
          <div className="w-44">
            <label className={labelCls}>CEP</label>
            <input
              type="text"
              value={zip}
              onChange={e => setZip(maskCEP(e.target.value))}
              className={`${inputCls} font-mono`}
              placeholder="00000-000"
            />
          </div>
          <button
            type="button"
            onClick={() => fetchCEP(cepDigits)}
            disabled={cepDigits.length !== 8 || cepLoading}
            className={btnSecondary}
          >
            {cepLoading ? 'Buscando...' : 'Buscar CEP'}
          </button>
        </div>

        <div className="grid grid-cols-3 gap-4">
          <div className="col-span-2">
            <label className={labelCls}>Logradouro</label>
            <input type="text" value={street} onChange={e => setStreet(e.target.value)} className={inputCls} placeholder="Rua, Av., etc." />
          </div>
          <div>
            <label className={labelCls}>Número</label>
            <input type="text" value={number} onChange={e => setNumber(e.target.value)} className={inputCls} placeholder="123" />
          </div>
        </div>

        <div>
          <label className={labelCls}>Complemento</label>
          <input type="text" value={complement} onChange={e => setComplement(e.target.value)} className={inputCls} placeholder="Sala, Andar... (opcional)" />
        </div>

        <div className="grid grid-cols-3 gap-4">
          <div>
            <label className={labelCls}>Bairro</label>
            <input type="text" value={neighborhood} onChange={e => setNeighborhood(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label className={labelCls}>Cidade</label>
            <input type="text" value={city} onChange={e => setCity(e.target.value)} className={inputCls} />
          </div>
          <div>
            <label className={labelCls}>UF</label>
            <select value={state} onChange={e => setState(e.target.value)} className={inputCls}>
              <option value="">—</option>
              {BR_STATES.map(uf => <option key={uf} value={uf}>{uf}</option>)}
            </select>
          </div>
        </div>
      </div>

      {error && <div className={alertError}>{error}</div>}
      {success && <div className={alertSuccess}>{success}</div>}

      <div>
        <button type="submit" disabled={saving} className={btnPrimary}>
          {saving ? 'Salvando...' : 'Salvar endereço'}
        </button>
      </div>
    </form>
  )
}

// ── MembrosTab ────────────────────────────────────────────────────────────────

interface MembrosTabProps {
  storeId: string
}

function MembrosTab({ storeId }: MembrosTabProps) {
  const [members, setMembers] = useState<StoreMemberRow[]>([])
  const [loadingMembers, setLoadingMembers] = useState(true)
  const [membersError, setMembersError] = useState('')

  // Add member
  const [userQuery, setUserQuery] = useState('')
  const [userResults, setUserResults] = useState<UserSummary[]>([])
  const [userDropOpen, setUserDropOpen] = useState(false)
  const [userSearchLoading, setUserSearchLoading] = useState(false)
  const [selectedUser, setSelectedUser] = useState<UserSummary | null>(null)
  const [newRole, setNewRole] = useState<StoreMemberRow['role']>('viewer')
  const [adding, setAdding] = useState(false)
  const [addError, setAddError] = useState('')
  const [addSuccess, setAddSuccess] = useState('')
  const userRef = useRef<HTMLDivElement>(null)

  // Row actions
  const [pendingRole, setPendingRole] = useState<string | null>(null)
  const [removing, setRemoving] = useState<string | null>(null)
  const [actionError, setActionError] = useState('')

  useEffect(() => {
    listStoreMembers(storeId)
      .then(setMembers)
      .catch(e => setMembersError(e.message))
      .finally(() => setLoadingMembers(false))
  }, [storeId])

  useEffect(() => {
    function handle(e: MouseEvent) {
      if (userRef.current && !userRef.current.contains(e.target as Node)) setUserDropOpen(false)
    }
    document.addEventListener('mousedown', handle)
    return () => document.removeEventListener('mousedown', handle)
  }, [])

  useEffect(() => {
    if (userQuery.length < 2 || selectedUser) {
      setUserResults([])
      setUserDropOpen(false)
      return
    }
    const timer = setTimeout(async () => {
      setUserSearchLoading(true)
      try {
        const results = await searchUsers(userQuery)
        setUserResults(results)
        setUserDropOpen(results.length > 0)
      } catch {
        setUserResults([])
        setUserDropOpen(false)
      } finally {
        setUserSearchLoading(false)
      }
    }, 350)
    return () => clearTimeout(timer)
  }, [userQuery, selectedUser])

  function selectUser(u: UserSummary) {
    setSelectedUser(u)
    setUserQuery(u.email)
    setUserDropOpen(false)
  }

  function clearUser() {
    setSelectedUser(null)
    setUserQuery('')
  }

  async function handleAdd() {
    if (!selectedUser) return
    setAdding(true)
    setAddError('')
    setAddSuccess('')
    try {
      await addStoreMember(storeId, selectedUser.email, newRole)
      const updated = await listStoreMembers(storeId)
      setMembers(updated)
      const addedName = selectedUser.display_name
      clearUser()
      setNewRole('viewer')
      setAddSuccess(`${addedName} adicionado com sucesso.`)
    } catch (e) {
      setAddError(e instanceof Error ? e.message : 'Erro ao adicionar membro')
    } finally {
      setAdding(false)
    }
  }

  async function handleRoleChange(userId: string, role: StoreMemberRow['role']) {
    setPendingRole(userId)
    setActionError('')
    try {
      await updateStoreMemberRole(storeId, userId, role)
      setMembers(prev => prev.map(m => m.user_id === userId ? { ...m, role } : m))
    } catch (e) {
      setActionError(e instanceof Error ? e.message : 'Erro ao alterar role')
    } finally {
      setPendingRole(null)
    }
  }

  async function handleRemove(userId: string) {
    if (!confirm('Remover este membro da loja?')) return
    setRemoving(userId)
    setActionError('')
    try {
      await removeStoreMember(storeId, userId)
      setMembers(prev => prev.filter(m => m.user_id !== userId))
    } catch (e) {
      setActionError(e instanceof Error ? e.message : 'Erro ao remover membro')
    } finally {
      setRemoving(null)
    }
  }

  if (loadingMembers) {
    return <div className="py-10 text-center text-zinc-400 text-sm">Carregando membros...</div>
  }

  return (
    <div className="space-y-6">
      {/* Add member */}
      <div className={sectionCls}>
        <h2 className={sectionTitleCls}>Adicionar membro</h2>
        <div ref={userRef} className="relative">
          <label className={labelCls}>Buscar por e-mail</label>
          <div className="relative">
            <input
              type="text"
              value={userQuery}
              onChange={e => { setUserQuery(e.target.value); if (selectedUser) clearUser() }}
              placeholder="Digite o e-mail..."
              className={inputCls}
              autoComplete="off"
            />
            {userSearchLoading && (
              <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-zinc-400">Buscando...</span>
            )}
          </div>
          {userDropOpen && userResults.length > 0 && (
            <div className="absolute z-10 mt-1 w-full rounded-lg border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 shadow-lg overflow-hidden">
              {userResults.map(u => (
                <button
                  key={u.id}
                  type="button"
                  onClick={() => selectUser(u)}
                  className="w-full px-4 py-2.5 text-left hover:bg-zinc-50 dark:hover:bg-zinc-800 transition-colors border-b border-zinc-100 dark:border-zinc-800 last:border-0"
                >
                  <p className="text-sm font-medium text-zinc-900 dark:text-zinc-100">{u.display_name}</p>
                  <p className="text-xs text-zinc-500">{u.email}</p>
                </button>
              ))}
            </div>
          )}
          {selectedUser && (
            <div className="mt-2 inline-flex items-center gap-2 rounded-full bg-violet-50 dark:bg-violet-900/20 border border-violet-200 dark:border-violet-800 px-3 py-1">
              <span className="text-xs font-medium text-violet-700 dark:text-violet-300">
                {selectedUser.display_name} · {selectedUser.email}
              </span>
              <button type="button" onClick={clearUser} className="text-violet-400 hover:text-violet-700 dark:hover:text-violet-200 text-sm leading-none">×</button>
            </div>
          )}
        </div>

        <div className="flex items-end gap-3">
          <div className="max-w-[200px]">
            <label className={labelCls}>Role</label>
            <select value={newRole} onChange={e => setNewRole(e.target.value as StoreMemberRow['role'])} className={inputCls}>
              <option value="viewer">Visualizador</option>
              <option value="stock_manager">Gerente de Estoque</option>
              <option value="admin">Admin</option>
            </select>
          </div>
          <button type="button" onClick={handleAdd} disabled={!selectedUser || adding} className={btnPrimary}>
            {adding ? 'Adicionando...' : 'Adicionar'}
          </button>
        </div>

        {membersError && <div className={alertError}>{membersError}</div>}
        {addError && <div className={alertError}>{addError}</div>}
        {addSuccess && <div className={alertSuccess}>{addSuccess}</div>}
      </div>

      {/* Members list */}
      {actionError && <div className={alertError}>{actionError}</div>}

      {members.length === 0 ? (
        <p className="text-sm text-zinc-400 text-center py-8">Nenhum membro cadastrado.</p>
      ) : (
        <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-zinc-50 dark:bg-zinc-900 border-b border-zinc-200 dark:border-zinc-800">
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Usuário</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Role</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Entrou em</th>
                <th className="px-4 py-3 text-right font-medium text-zinc-500">Ações</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-100 dark:divide-zinc-800">
              {members.map(m => (
                <tr key={m.id} className="bg-white dark:bg-zinc-900 hover:bg-zinc-50 dark:hover:bg-zinc-800/50 transition-colors">
                  <td className="px-4 py-3">
                    <p className="font-medium text-zinc-900 dark:text-zinc-100">{m.user_display_name}</p>
                    <p className="text-xs text-zinc-500">{m.user_email}</p>
                  </td>
                  <td className="px-4 py-3">
                    <select
                      value={m.role}
                      onChange={e => handleRoleChange(m.user_id, e.target.value as StoreMemberRow['role'])}
                      disabled={pendingRole === m.user_id}
                      className="rounded border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-800 px-2 py-1 text-xs text-zinc-700 dark:text-zinc-300 focus:outline-none focus:ring-1 focus:ring-violet-500 disabled:opacity-50"
                    >
                      <option value="viewer">Visualizador</option>
                      <option value="stock_manager">Gerente de Estoque</option>
                      <option value="admin">Admin</option>
                    </select>
                  </td>
                  <td className="px-4 py-3 text-zinc-500 text-xs">
                    {new Date(m.joined_at).toLocaleDateString('pt-BR')}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button
                      type="button"
                      onClick={() => handleRemove(m.user_id)}
                      disabled={removing === m.user_id}
                      className="text-xs text-red-500 hover:text-red-700 disabled:opacity-50"
                    >
                      {removing === m.user_id ? 'Removendo...' : 'Remover'}
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

// ── AuditoriaTab ──────────────────────────────────────────────────────────────

interface AuditoriaTabProps {
  storeId: string
}

function AuditoriaTab({ storeId }: AuditoriaTabProps) {
  const [entries, setEntries] = useState<AuditEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    getAuditLog(storeId, 100, 0)
      .then(data => setEntries(data ?? []))
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [storeId])

  if (loading) {
    return <div className="py-10 text-center text-zinc-400 text-sm">Carregando auditoria...</div>
  }

  if (error) {
    return <div className={alertError}>{error}</div>
  }

  if (entries.length === 0) {
    return <p className="text-sm text-zinc-400 text-center py-10">Nenhuma entrada no log de auditoria.</p>
  }

  return (
    <div className="space-y-3">
      {entries.map(entry => (
        <div key={entry.id} className="rounded-xl border border-zinc-200 dark:border-zinc-800 p-4">
          <div className="flex items-start justify-between gap-4 mb-3">
            <span className="inline-flex items-center rounded-full bg-zinc-100 dark:bg-zinc-800 px-2.5 py-0.5 text-xs font-mono font-medium text-zinc-700 dark:text-zinc-300">
              {changeTypeLabel(entry.change_type)}
            </span>
            <div className="text-right flex-shrink-0">
              <p className="text-xs font-medium text-zinc-700 dark:text-zinc-300">{entry.changed_by_name}</p>
              <p className="text-xs text-zinc-500">{entry.changed_by_email}</p>
              <p className="text-xs text-zinc-400 mt-0.5">{formatTs(entry.created_at)}</p>
            </div>
          </div>

          {Object.keys(entry.changes).length > 0 && (
            <div className="space-y-1.5 border-t border-zinc-100 dark:border-zinc-800 pt-3 mt-1">
              {Object.entries(entry.changes).map(([field, diff]) => (
                <div key={field} className="flex items-baseline gap-2 text-xs">
                  <span className="font-mono text-zinc-400 flex-shrink-0 w-36 truncate">{field}</span>
                  {field === 'logo_url' ? (
                    <span className="italic text-zinc-400">imagem atualizada</span>
                  ) : (
                    <>
                      <span className="text-red-500 dark:text-red-400 line-through truncate max-w-[140px]">
                        {diff.old !== null && diff.old !== undefined ? String(diff.old) : '—'}
                      </span>
                      <span className="text-zinc-400 flex-shrink-0">→</span>
                      <span className="text-green-600 dark:text-green-400 truncate max-w-[140px]">
                        {diff.new !== null && diff.new !== undefined ? String(diff.new) : '—'}
                      </span>
                    </>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  )
}

// ── page ──────────────────────────────────────────────────────────────────────

export default function EditStorePage() {
  const { id } = useParams<{ id: string }>()
  const [store, setStore] = useState<AdminStore | null>(null)
  const [loadError, setLoadError] = useState('')
  const [activeTab, setActiveTab] = useState<Tab>('dados')

  useEffect(() => {
    getStore(id)
      .then(setStore)
      .catch(e => setLoadError(e.message))
  }, [id])

  if (loadError) {
    return (
      <div className="mx-auto max-w-4xl px-4 py-6">
        <div className={alertError}>{loadError}</div>
        <Link href="/admin/lojas" className="mt-3 inline-block text-sm text-violet-600 hover:underline">
          Voltar para lojas
        </Link>
      </div>
    )
  }

  if (!store) {
    return (
      <div className="min-h-[60vh] flex items-center justify-center">
        <div className="animate-pulse text-zinc-400 text-sm">Carregando loja...</div>
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-4xl px-4 py-6">
      {/* Breadcrumb + header */}
      <div className="mb-6">
        <div className="flex items-center gap-2 text-sm text-zinc-500 mb-1">
          <Link href="/admin/lojas" className="hover:text-zinc-900 dark:hover:text-zinc-100">Lojas</Link>
          <span>/</span>
          <span className="text-zinc-900 dark:text-zinc-50 truncate max-w-[240px]">{store.name}</span>
        </div>
        <div className="flex items-center justify-between gap-4">
          <div>
            <h1 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50">{store.name}</h1>
            <p className="text-sm text-zinc-500 font-mono mt-0.5">{store.slug}</p>
          </div>
          <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${docStatusClass[store.document_status]}`}>
            {docStatusLabel[store.document_status]}
          </span>
        </div>
      </div>

      {/* Tabs nav */}
      <div className="flex items-center gap-1 border-b border-zinc-200 dark:border-zinc-800 mb-6">
        {TABS.map(tab => (
          <button
            key={tab.id}
            type="button"
            onClick={() => setActiveTab(tab.id)}
            className={[
              'px-4 py-2.5 text-sm font-medium transition-colors -mb-px',
              activeTab === tab.id
                ? 'border-b-2 border-violet-600 text-violet-600 dark:text-violet-400'
                : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100',
            ].join(' ')}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Tab content */}
      {activeTab === 'dados' && <DadosTab store={store} onSaved={setStore} />}
      {activeTab === 'documento' && <DocumentoTab store={store} onSaved={setStore} />}
      {activeTab === 'endereco' && <EnderecoTab store={store} onSaved={setStore} />}
      {activeTab === 'membros' && <MembrosTab storeId={store.id} />}
      {activeTab === 'auditoria' && <AuditoriaTab storeId={store.id} />}
    </div>
  )
}
