'use client'

import { useEffect, useRef, useState } from 'react'
import { useParams } from 'next/navigation'
import {
  getStorePublic,
  updateStoreProfile,
  uploadStoreLogoSelf,
  AdminStore,
} from '@/lib/stores-admin'

export default function StoreProfilePage() {
  const { id } = useParams<{ id: string }>()

  const [store, setStore] = useState<AdminStore | null>(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  // track whether initial data load is complete so ViaCEP doesn't overwrite data
  const populatedRef = useRef(false)

  // form state
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [tradeName, setTradeName] = useState('')
  const [phone, setPhone] = useState('')
  const [addressZip, setAddressZip] = useState('')
  const [addressStreet, setAddressStreet] = useState('')
  const [addressNumber, setAddressNumber] = useState('')
  const [addressComplement, setAddressComplement] = useState('')
  const [addressNeighborhood, setAddressNeighborhood] = useState('')
  const [addressCity, setAddressCity] = useState('')
  const [addressState, setAddressState] = useState('')

  // logo
  const [logoFile, setLogoFile] = useState<File | null>(null)
  const [logoPreview, setLogoPreview] = useState<string | null>(null)
  const logoInputRef = useRef<HTMLInputElement>(null)

  function populate(s: AdminStore) {
    setStore(s)
    setName(s.name)
    setDescription(s.description ?? '')
    setTradeName(s.trade_name ?? '')
    setPhone(s.phone ?? '')
    setAddressZip(s.address_zip ?? '')
    setAddressStreet(s.address_street ?? '')
    setAddressNumber(s.address_number ?? '')
    setAddressComplement(s.address_complement ?? '')
    setAddressNeighborhood(s.address_neighborhood ?? '')
    setAddressCity(s.address_city ?? '')
    setAddressState(s.address_state ?? '')
    setLogoPreview(s.logo_url || null)
    populatedRef.current = true
  }

  useEffect(() => {
    populatedRef.current = false
    getStorePublic(id)
      .then(populate)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [id])

  // CEP auto-fill via ViaCEP — only runs when user manually changes ZIP,
  // never on initial load (populatedRef guard) and never when street is already set.
  useEffect(() => {
    if (!populatedRef.current) return
    if (addressZip.length !== 8 || addressStreet) return
    fetch(`https://viacep.com.br/ws/${addressZip}/json/`)
      .then(r => r.json())
      .then(data => {
        if (!data.erro) {
          setAddressStreet(data.logradouro ?? '')
          setAddressNeighborhood(data.bairro ?? '')
          setAddressCity(data.localidade ?? '')
          setAddressState(data.uf ?? '')
        }
      })
      .catch(() => {})
  }, [addressZip, addressStreet])

  function handleLogoChange(e: React.ChangeEvent<HTMLInputElement>) {
    const f = e.target.files?.[0]
    if (!f) return
    setLogoFile(f)
    setLogoPreview(URL.createObjectURL(f))
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError(null)
    setSuccess(null)
    try {
      let updated = await updateStoreProfile(id, {
        name, description, trade_name: tradeName, phone,
        address_zip: addressZip,
        address_street: addressStreet,
        address_number: addressNumber,
        address_complement: addressComplement,
        address_neighborhood: addressNeighborhood,
        address_city: addressCity,
        address_state: addressState,
      })
      if (logoFile) {
        try {
          updated = await uploadStoreLogoSelf(id, logoFile)
          setLogoFile(null)
        } catch {
          // logo failure is non-fatal
        }
      }
      populate(updated)
      setSuccess('Perfil atualizado com sucesso.')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Erro ao salvar')
    } finally {
      setSaving(false)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-24 text-zinc-400 text-sm">
        Carregando...
      </div>
    )
  }

  if (!store) {
    return (
      <div className="mx-auto max-w-2xl px-4 py-8">
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400">
          {error ?? 'Não foi possível carregar os dados da loja.'}
        </div>
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-2xl px-4 py-8">
      <div className="mb-6">
        <h1 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50">
          Perfil da loja
        </h1>
        <p className="text-sm text-zinc-500 mt-1">{store.name}</p>
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">

        {/* Identidade */}
        <section className="rounded-xl border border-zinc-200 dark:border-zinc-800 p-5 space-y-4">
          <h2 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Identidade</h2>

          <div className="flex items-start gap-4">
            <div
              onClick={() => logoInputRef.current?.click()}
              className="relative w-24 h-24 rounded-xl border-2 border-dashed border-zinc-300 dark:border-zinc-700 flex items-center justify-center cursor-pointer overflow-hidden shrink-0 hover:border-violet-400 transition-colors"
            >
              {logoPreview ? (
                <img src={logoPreview} alt="Logo" className="w-full h-full object-cover" />
              ) : (
                <span className="text-xs text-zinc-400 text-center px-2">Clique para logo</span>
              )}
            </div>
            <input ref={logoInputRef} type="file" accept="image/*" className="hidden" onChange={handleLogoChange} />
            <div className="flex-1">
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">Nome da loja *</label>
              <input
                type="text" value={name} onChange={e => setName(e.target.value)} required
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500"
              />
            </div>
          </div>

          <div>
            <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">Descrição</label>
            <textarea
              value={description} onChange={e => setDescription(e.target.value)} rows={3}
              className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500 resize-none"
            />
          </div>
        </section>

        {/* Contato */}
        <section className="rounded-xl border border-zinc-200 dark:border-zinc-800 p-5 space-y-4">
          <h2 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Contato</h2>
          <div className="grid grid-cols-2 gap-4">
            <div className="col-span-2">
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">Nome fantasia</label>
              <input type="text" value={tradeName} onChange={e => setTradeName(e.target.value)}
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500" />
            </div>
            <div className="col-span-2">
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">Telefone</label>
              <input type="text" value={phone} onChange={e => setPhone(e.target.value)}
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500" />
            </div>
          </div>
        </section>

        {/* Endereço */}
        <section className="rounded-xl border border-zinc-200 dark:border-zinc-800 p-5 space-y-4">
          <h2 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Endereço</h2>
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">CEP</label>
              <input type="text" value={addressZip}
                onChange={e => setAddressZip(e.target.value.replace(/\D/g, ''))}
                maxLength={8} placeholder="00000000"
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500" />
            </div>
            <div>
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">Estado (UF)</label>
              <input type="text" value={addressState}
                onChange={e => setAddressState(e.target.value.toUpperCase())} maxLength={2}
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500" />
            </div>
            <div className="col-span-2">
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">Logradouro</label>
              <input type="text" value={addressStreet} onChange={e => setAddressStreet(e.target.value)}
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500" />
            </div>
            <div>
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">Número</label>
              <input type="text" value={addressNumber} onChange={e => setAddressNumber(e.target.value)}
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500" />
            </div>
            <div>
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">Complemento</label>
              <input type="text" value={addressComplement} onChange={e => setAddressComplement(e.target.value)}
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500" />
            </div>
            <div>
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">Bairro</label>
              <input type="text" value={addressNeighborhood} onChange={e => setAddressNeighborhood(e.target.value)}
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500" />
            </div>
            <div>
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">Cidade</label>
              <input type="text" value={addressCity} onChange={e => setAddressCity(e.target.value)}
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500" />
            </div>
          </div>
        </section>

        {error && (
          <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400">
            {error}
          </div>
        )}
        {success && (
          <div className="rounded-lg border border-green-200 bg-green-50 px-4 py-3 text-sm text-green-700 dark:border-green-900/50 dark:bg-green-950/30 dark:text-green-400">
            {success}
          </div>
        )}

        <div className="flex justify-end">
          <button
            type="submit" disabled={saving}
            className="rounded-lg bg-violet-600 px-5 py-2 text-sm font-semibold text-white hover:bg-violet-700 disabled:opacity-50 transition-colors"
          >
            {saving ? 'Salvando...' : 'Salvar alterações'}
          </button>
        </div>
      </form>
    </div>
  )
}
