'use client'

import { useEffect, useState } from 'react'
import Link from 'next/link'
import { listStores, verifyDocument, AdminStore } from '@/lib/stores-admin'

const statusLabel: Record<AdminStore['document_status'], string> = {
  pending: 'Pendente',
  auto_verified: 'Verificado (auto)',
  manually_verified: 'Verificado (manual)',
}

const statusClass: Record<AdminStore['document_status'], string> = {
  pending: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-400',
  auto_verified: 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400',
  manually_verified: 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
}

function formatDoc(type?: string, number?: string) {
  if (!type || !number) return '—'
  if (type === 'cnpj' && number.length === 14) {
    return `${number.slice(0,2)}.${number.slice(2,5)}.${number.slice(5,8)}/${number.slice(8,12)}-${number.slice(12)}`
  }
  if (type === 'cpf' && number.length === 11) {
    return `${number.slice(0,3)}.${number.slice(3,6)}.${number.slice(6,9)}-${number.slice(9)}`
  }
  return number
}

export default function LojasPage() {
  const [stores, setStores] = useState<AdminStore[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [verifying, setVerifying] = useState<string | null>(null)

  useEffect(() => {
    listStores()
      .then(setStores)
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  async function handleVerify(id: string) {
    setVerifying(id)
    try {
      const updated = await verifyDocument(id)
      setStores(prev => prev.map(s => s.id === id ? updated : s))
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Erro ao verificar')
    } finally {
      setVerifying(null)
    }
  }

  return (
    <div className="mx-auto max-w-6xl px-4 py-6">
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-lg font-semibold text-zinc-900 dark:text-zinc-50">Lojas</h1>
          <p className="text-sm text-zinc-500 mt-1">Gerencie as lojas cadastradas na plataforma.</p>
        </div>
        <Link
          href="/admin/lojas/nova"
          className="rounded-lg bg-violet-600 px-4 py-2 text-sm font-semibold text-white hover:bg-violet-700 transition-colors"
        >
          + Nova loja
        </Link>
      </div>

      {loading && (
        <div className="flex items-center justify-center py-16 text-zinc-400 text-sm">
          Carregando...
        </div>
      )}

      {!loading && error && (
        <div className="rounded-xl border border-red-200 bg-red-50 px-5 py-4 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400">
          {error}
        </div>
      )}

      {!loading && !error && stores.length === 0 && (
        <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
          <p className="text-zinc-400 text-sm">Nenhuma loja cadastrada ainda.</p>
          <Link href="/admin/lojas/nova" className="text-sm text-violet-600 hover:underline">
            Cadastrar a primeira loja
          </Link>
        </div>
      )}

      {!loading && !error && stores.length > 0 && (
        <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-zinc-50 dark:bg-zinc-900 border-b border-zinc-200 dark:border-zinc-800">
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Nome</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Slug</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Documento</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Status</th>
                <th className="px-4 py-3 text-left font-medium text-zinc-500">Criada em</th>
                <th className="px-4 py-3 text-right font-medium text-zinc-500">Ações</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-100 dark:divide-zinc-800">
              {stores.map(s => (
                <tr key={s.id} className="bg-white dark:bg-zinc-900 hover:bg-zinc-50 dark:hover:bg-zinc-800/50 transition-colors">
                  <td className="px-4 py-3 font-medium text-zinc-900 dark:text-zinc-100">
                    {s.name}
                    {s.legal_name && (
                      <p className="text-xs text-zinc-400 font-normal mt-0.5 truncate max-w-[200px]">{s.legal_name}</p>
                    )}
                  </td>
                  <td className="px-4 py-3 text-zinc-500 font-mono text-xs">{s.slug}</td>
                  <td className="px-4 py-3 text-zinc-500">
                    {s.document_type && (
                      <span className="uppercase text-xs font-semibold text-zinc-400 mr-1">{s.document_type}</span>
                    )}
                    {formatDoc(s.document_type, s.document_number)}
                  </td>
                  <td className="px-4 py-3">
                    <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${statusClass[s.document_status]}`}>
                      {statusLabel[s.document_status]}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-zinc-500">
                    {new Date(s.created_at).toLocaleDateString('pt-BR')}
                  </td>
                  <td className="px-4 py-3 text-right">
                    {s.document_status === 'pending' && (
                      <button
                        onClick={() => handleVerify(s.id)}
                        disabled={verifying === s.id}
                        className="text-xs text-violet-600 hover:text-violet-700 dark:text-violet-400 disabled:opacity-50"
                      >
                        {verifying === s.id ? 'Verificando...' : 'Verificar manualmente'}
                      </button>
                    )}
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
