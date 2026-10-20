import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { apiJson } from '../lib/api'
import { getSessionScope } from '../lib/auth'

type Customer = {
  id: string
  name: string
  email: string | null
  phone: string | null
}
type CustomerPage = { items: Customer[]; total: number; limit: number; offset: number }
type CustomerDraft = { name: string; email: string; phone: string }

const emptyDraft: CustomerDraft = { name: '', email: '', phone: '' }
const pageSize = 20

export default function CustomersPage() {
  const [permissions, setPermissions] = useState<string[] | null>(null)
  const [items, setItems] = useState<Customer[]>([])
  const [total, setTotal] = useState(0)
  const [search, setSearch] = useState('')
  const [query, setQuery] = useState('')
  const [offset, setOffset] = useState(0)
  const [refresh, setRefresh] = useState(0)
  const [draft, setDraft] = useState<CustomerDraft>(emptyDraft)
  const [editingID, setEditingID] = useState('')
  const [saving, setSaving] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [listError, setListError] = useState('')
  const [message, setMessage] = useState('')
  const authSequence = useRef(0)
  const listSequence = useRef(0)
  const scope = getSessionScope()
  const canRead = permissions?.includes('customer:read') ?? false
  const canWrite = permissions?.includes('customer:write') ?? false

  useEffect(() => {
    const requestID = ++authSequence.current
    let active = true
    const currentScope = getSessionScope()
    setPermissions(null)
    setItems([])
    void apiJson<{ permissions: string[] }>('/api/v1/auth/me').then((data) => {
      if (!active || requestID !== authSequence.current || currentScope !== getSessionScope()) return
      setPermissions(data.permissions)
    }).catch(() => {
      if (!active || requestID !== authSequence.current || currentScope !== getSessionScope()) return
      setError('Não foi possível verificar seu acesso aos clientes. Atualize a página e tente novamente.')
      setLoading(false)
    })
    return () => { active = false }
  }, [scope])

  useEffect(() => {
    const requestID = ++listSequence.current
    let active = true
    const currentScope = getSessionScope()
    if (!canRead || !currentScope) {
      setItems([])
      setTotal(0)
      setLoading(false)
      return
    }
    setLoading(true)
    setListError('')
    const params = new URLSearchParams({ limit: String(pageSize), offset: String(offset) })
    if (query) params.set('q', query)
    void apiJson<CustomerPage>(`/api/v1/customers?${params.toString()}`).then((data) => {
      if (!active || requestID !== listSequence.current || currentScope !== getSessionScope()) return
      setItems(data.items ?? [])
      setTotal(data.total)
      setListError('')
      setLoading(false)
    }).catch(() => {
      if (!active || requestID !== listSequence.current || currentScope !== getSessionScope()) return
      setListError('Não foi possível consultar os clientes agora. Confira sua conexão e tente novamente.')
      setItems([])
      setTotal(0)
      setLoading(false)
    })
    return () => { active = false }
  }, [canRead, scope, offset, query, refresh])

  function startEdit(item: Customer) {
    setEditingID(item.id)
    setDraft({ name: item.name, email: item.email ?? '', phone: item.phone ?? '' })
    setMessage('')
  }

  function resetForm() {
    setEditingID('')
    setDraft(emptyDraft)
  }

  async function save(event: FormEvent) {
    event.preventDefault()
    if (!canWrite || saving || draft.name.trim().length < 2) return
    setSaving(true)
    setError('')
    setMessage('')
    try {
      const endpoint = editingID ? `/api/v1/customers/${editingID}` : '/api/v1/customers'
      await apiJson<Customer>(endpoint, {
        method: editingID ? 'PUT' : 'POST',
        body: {
          name: draft.name.trim(),
          email: draft.email.trim() || null,
          phone: draft.phone.trim() || null,
        },
      })
      setMessage(editingID ? 'Cadastro atualizado nesta loja.' : 'Cliente cadastrado nesta loja.')
      resetForm()
      setOffset(0)
      setRefresh((n) => n + 1)
    } catch {
      setError('Não foi possível salvar o cadastro. Confira os dados e tente novamente.')
    } finally {
      setSaving(false)
    }
  }

  function filter(event: FormEvent) {
    event.preventDefault()
    setOffset(0)
    setQuery(search.trim())
  }

  return (
    <div>
      <h2 className="text-lg font-semibold">Clientes da loja</h2>
      <p className="mt-2 text-sm text-gray-600">
        Guarde o nome e os contatos dos seus clientes em um só lugar.
        Os dados ficam disponíveis apenas para pessoas autorizadas nesta loja.
      </p>
      {error ? <p role="alert" className="mt-3 text-sm text-red-700">{error}</p> : null}
      {message ? <p role="status" className="mt-3 text-sm">{message}</p> : null}
      {permissions && !canRead ? (
        <p className="mt-3 text-sm text-amber-800">Sem permissão para consultar os clientes desta loja.</p>
      ) : null}
      {canRead ? (
        <>
          <form onSubmit={filter} className="mt-4 flex flex-wrap items-end gap-2">
            <label className="text-sm">Buscar pelo nome
              <input type="search" value={search} maxLength={100}
                onChange={(event) => setSearch(event.target.value)}
                className="mt-1 block rounded-md border px-3 py-2" />
            </label>
            <button type="submit" disabled={loading}
              className="rounded-md border px-3 py-2 text-sm disabled:opacity-50">Buscar</button>
            <button type="button" disabled={loading} onClick={() => {
              setSearch('')
              setQuery('')
              setOffset(0)
              setRefresh((n) => n + 1)
            }} className="rounded-md border px-3 py-2 text-sm disabled:opacity-50">
              Mostrar todos
            </button>
          </form>
          {canWrite ? (
            <form onSubmit={(event) => void save(event)}
              className="mt-5 rounded-md border p-3" aria-label="Cadastro de cliente">
              <h3 className="text-sm font-semibold">{editingID ? 'Editar cliente' : 'Novo cliente'}</h3>
              <div className="mt-3 grid gap-3 sm:grid-cols-2">
                <label className="text-xs">Nome
                  <input value={draft.name} required minLength={2} maxLength={120}
                    onChange={(event) => setDraft((d) => ({ ...d, name: event.target.value }))}
                    className="mt-1 block w-full rounded-md border px-3 py-2 text-sm" />
                </label>
                <label className="text-xs">E-mail (opcional)
                  <input type="email" value={draft.email} maxLength={254}
                    onChange={(event) => setDraft((d) => ({ ...d, email: event.target.value }))}
                    className="mt-1 block w-full rounded-md border px-3 py-2 text-sm" />
                </label>
                <label className="text-xs">Telefone (opcional)
                  <input value={draft.phone} maxLength={30}
                    onChange={(event) => setDraft((d) => ({ ...d, phone: event.target.value }))}
                    className="mt-1 block w-full rounded-md border px-3 py-2 text-sm" />
                </label>
              </div>
              <div className="mt-3 flex gap-2">
                <button type="submit" disabled={saving || draft.name.trim().length < 2}
                  className="rounded-md bg-gray-900 px-3 py-2 text-sm text-white disabled:cursor-not-allowed disabled:opacity-50">
                  {saving ? 'Salvando…' : editingID ? 'Salvar alterações' : 'Cadastrar cliente'}
                </button>
                {editingID ? (
                  <button type="button" disabled={saving} onClick={resetForm}
                    className="rounded-md border px-3 py-2 text-sm">Cancelar edição</button>
                ) : null}
              </div>
              <p className="mt-3 text-xs text-gray-600">
                Cadastre apenas informações de contato que o cliente autorizou compartilhar.
              </p>
            </form>
          ) : null}
          {loading ? <p role="status" className="mt-4 text-sm">Consultando clientes…</p> : null}
          {listError ? (
            <div role="alert" className="mt-4 rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-800">
              <p>{listError}</p>
              <button type="button" onClick={() => setRefresh((n) => n + 1)}
                className="mt-2 rounded-md border border-red-300 bg-white px-3 py-2 font-medium hover:bg-red-50">
                Tentar novamente
              </button>
            </div>
          ) : null}
          <section aria-label="Lista de clientes" className="mt-5 overflow-auto" aria-busy={loading}>
            {!loading && !listError ? (
              <p className="mb-2 text-xs text-gray-600">
                {total === 0 ? 'Nenhum cliente encontrado' : `${total} cliente(s) encontrado(s)`}
                {query ? ` para “${query}”` : ''}
              </p>
            ) : null}
            {!listError ? <table className="min-w-full text-left text-xs">
              <thead className="bg-gray-50 text-gray-600">
                <tr><th className="px-2 py-2">Nome</th><th className="px-2 py-2">E-mail</th>
                  <th className="px-2 py-2">Telefone</th><th className="px-2 py-2">Ações</th></tr>
              </thead>
              <tbody className="divide-y">
                {items.map((item) => (
                  <tr key={item.id}>
                    <td className="px-2 py-2">{item.name}</td>
                    <td className="px-2 py-2">{item.email || '—'}</td>
                    <td className="px-2 py-2">{item.phone || '—'}</td>
                    <td className="px-2 py-2">
                      {canWrite ? <button type="button" onClick={() => startEdit(item)}
                        className="rounded-md border px-2 py-1">Editar</button> : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table> : null}
            {!loading && !listError && items.length === 0 ? (
              <p role="status" className="mt-3 rounded-lg border border-dashed border-slate-300 bg-slate-50 p-4 text-sm text-slate-700">
                {query
                  ? 'Não encontramos clientes com esse nome. Experimente outra busca ou escolha “Mostrar todos”.'
                  : canWrite
                    ? 'Ainda não há clientes cadastrados. Preencha o formulário acima para adicionar o primeiro.'
                    : 'Ainda não há clientes cadastrados nesta loja.'}
              </p>
            ) : null}
          </section>
          {!listError ? <div className="mt-3 flex flex-wrap items-center gap-3 text-xs">
            <button type="button" disabled={loading || offset === 0}
              onClick={() => setOffset((n) => Math.max(0, n - pageSize))}
              className="rounded-md border px-3 py-2 disabled:opacity-50">Página anterior</button>
            <span>Página {Math.floor(offset / pageSize) + 1}</span>
            <button type="button" disabled={loading || offset + pageSize >= total || offset + pageSize > 5000}
              onClick={() => setOffset((n) => n + pageSize)}
              className="rounded-md border px-3 py-2 disabled:opacity-50">Próxima página</button>
          </div> : null}
        </>
      ) : null}
    </div>
  )
}
