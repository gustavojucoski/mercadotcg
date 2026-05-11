'use client'

import { useEffect, useRef, useState } from 'react'
import { useParams } from 'next/navigation'
import {
  listStoreMembers,
  addStoreMember,
  removeStoreMember,
  updateStoreMemberRole,
  getMyRole,
  StoreMemberRow,
} from '@/lib/stores-admin'
import { useAuth } from '@/components/AuthProvider'

const ROLE_LABELS: Record<string, string> = {
  admin: 'Administrador',
  stock_manager: 'Gestor de estoque',
  viewer: 'Visualizador',
}

const ROLES = ['admin', 'stock_manager', 'viewer'] as const

export default function StoreMembrosPage() {
  const { id } = useParams<{ id: string }>()
  const { user } = useAuth()

  const [members, setMembers] = useState<StoreMemberRow[]>([])
  const [myRole, setMyRole] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // add member form
  const [addEmail, setAddEmail] = useState('')
  const [addRole, setAddRole] = useState<string>('viewer')
  const [adding, setAdding] = useState(false)
  const [addError, setAddError] = useState<string | null>(null)

  const [removing, setRemoving] = useState<string | null>(null)
  const [updatingRole, setUpdatingRole] = useState<string | null>(null)

  const isAdmin = myRole === 'admin'

  useEffect(() => {
    Promise.all([listStoreMembers(id), getMyRole(id)])
      .then(([m, r]) => { setMembers(m ?? []); setMyRole(r) })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }, [id])

  async function handleAdd(e: React.FormEvent) {
    e.preventDefault()
    if (!addEmail) return
    setAdding(true)
    setAddError(null)
    try {
      await addStoreMember(id, addEmail, addRole)
      const updated = await listStoreMembers(id)
      setMembers(updated ?? [])
      setAddEmail('')
    } catch (e) {
      setAddError(e instanceof Error ? e.message : 'Erro ao adicionar')
    } finally {
      setAdding(false)
    }
  }

  async function handleRemove(userId: string) {
    setRemoving(userId)
    try {
      await removeStoreMember(id, userId)
      setMembers(prev => prev.filter(m => m.user_id !== userId))
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Erro ao remover')
    } finally {
      setRemoving(null)
    }
  }

  async function handleRoleChange(userId: string, newRole: string) {
    setUpdatingRole(userId)
    try {
      await updateStoreMemberRole(id, userId, newRole)
      setMembers(prev => prev.map(m =>
        m.user_id === userId ? { ...m, role: newRole as StoreMemberRow['role'] } : m
      ))
    } catch (e) {
      alert(e instanceof Error ? e.message : 'Erro ao atualizar role')
    } finally {
      setUpdatingRole(null)
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-16 text-zinc-400 text-sm">
        Carregando...
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-3xl px-4 py-6 space-y-6">
      <div>
        <h1 className="text-base font-semibold text-zinc-900 dark:text-zinc-50">Membros</h1>
        <p className="text-sm text-zinc-500 mt-0.5">
          Gerencie quem tem acesso à sua loja.
        </p>
      </div>

      {error && (
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-400">
          {error}
        </div>
      )}

      {/* Add member form */}
      {isAdmin && (
        <form onSubmit={handleAdd} className="rounded-xl border border-zinc-200 dark:border-zinc-800 p-5 space-y-4">
          <h2 className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">Adicionar membro</h2>
          <div className="flex gap-3 items-end">
            <div className="flex-1">
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">E-mail</label>
              <input
                type="email" value={addEmail} onChange={e => setAddEmail(e.target.value)}
                placeholder="usuario@exemplo.com" required
                className="w-full rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500"
              />
            </div>
            <div>
              <label className="block text-xs font-medium text-zinc-600 dark:text-zinc-400 mb-1">Papel</label>
              <select value={addRole} onChange={e => setAddRole(e.target.value)}
                className="rounded-lg border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-violet-500"
              >
                {ROLES.map(r => (
                  <option key={r} value={r}>{ROLE_LABELS[r]}</option>
                ))}
              </select>
            </div>
            <button
              type="submit" disabled={adding}
              className="rounded-lg bg-violet-600 px-4 py-2 text-sm font-semibold text-white hover:bg-violet-700 disabled:opacity-50 transition-colors shrink-0"
            >
              {adding ? 'Adicionando...' : 'Adicionar'}
            </button>
          </div>
          {addError && <p className="text-xs text-red-600">{addError}</p>}
        </form>
      )}

      {/* Members list */}
      <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 overflow-hidden">
        <table className="w-full text-sm">
          <thead>
            <tr className="bg-zinc-50 dark:bg-zinc-900 border-b border-zinc-200 dark:border-zinc-800">
              <th className="px-4 py-3 text-left font-medium text-zinc-500">Usuário</th>
              <th className="px-4 py-3 text-left font-medium text-zinc-500">Papel</th>
              <th className="px-4 py-3 text-left font-medium text-zinc-500">Desde</th>
              {isAdmin && <th className="px-4 py-3" />}
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-100 dark:divide-zinc-800">
            {members.map(m => {
              const isMe = m.user_id === user?.id
              return (
                <tr key={m.id} className="bg-white dark:bg-zinc-900">
                  <td className="px-4 py-3">
                    <p className="font-medium text-zinc-900 dark:text-zinc-100">{m.user_display_name}</p>
                    <p className="text-xs text-zinc-400">{m.user_email}</p>
                  </td>
                  <td className="px-4 py-3">
                    {isAdmin && !isMe ? (
                      <select
                        value={m.role}
                        disabled={updatingRole === m.user_id}
                        onChange={e => handleRoleChange(m.user_id, e.target.value)}
                        className="rounded-md border border-zinc-200 dark:border-zinc-700 bg-white dark:bg-zinc-900 px-2 py-1 text-xs focus:outline-none focus:ring-1 focus:ring-violet-500"
                      >
                        {ROLES.map(r => (
                          <option key={r} value={r}>{ROLE_LABELS[r]}</option>
                        ))}
                      </select>
                    ) : (
                      <span className="text-zinc-600 dark:text-zinc-400">
                        {ROLE_LABELS[m.role] ?? m.role}
                        {isMe && <span className="ml-1 text-xs text-zinc-400">(você)</span>}
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-zinc-500 text-xs">
                    {new Date(m.joined_at).toLocaleDateString('pt-BR')}
                  </td>
                  {isAdmin && (
                    <td className="px-4 py-3 text-right">
                      {!isMe && (
                        <button
                          onClick={() => handleRemove(m.user_id)}
                          disabled={removing === m.user_id}
                          className="text-xs text-red-500 hover:text-red-700 disabled:opacity-50"
                        >
                          {removing === m.user_id ? 'Removendo...' : 'Remover'}
                        </button>
                      )}
                    </td>
                  )}
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
