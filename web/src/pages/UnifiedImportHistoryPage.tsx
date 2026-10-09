import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { apiJson, errorMessage } from '../lib/api'
import { getSessionScope } from '../lib/auth'

type UnifiedReceipt = {
  batch_id: string
  kind: 'products' | 'opening-stock'
  item_count: number
  actor_name: string
  created_at: string
}

type PageResponse = {
  items: UnifiedReceipt[] | null
  limit: number
  offset: number
  has_more: boolean
}

type Permissions = { permissions: string[] }

const pageSize = 10
const maxOffset = 5000

function validRange(from: string, to: string): boolean {
  if (!from || !to) return true
  const start = Date.parse(from + 'T00:00:00Z')
  const end = Date.parse(to + 'T00:00:00Z')
  return Number.isFinite(start) && Number.isFinite(end) &&
    end >= start && end - start <= 365 * 24 * 60 * 60 * 1000
}

export default function UnifiedImportHistoryPage() {
  const [permissions, setPermissions] = useState<string[] | null>(null)
  const [authError, setAuthError] = useState('')
  const [page, setPage] = useState<PageResponse | null>(null)
  const [offset, setOffset] = useState(0)
  const [fromDraft, setFromDraft] = useState('')
  const [toDraft, setToDraft] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [refresh, setRefresh] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const requests = useRef(0)
  const scope = getSessionScope()

  const canProducts = permissions?.includes('product:write') ?? false
  const canStock = permissions?.includes('inventory:adjust') ?? false
  const authorized = canProducts || canStock
  const valid = validRange(fromDraft, toDraft)

  useEffect(() => {
    const originalScope = getSessionScope()
    let cancelled = false
    setPermissions(null)
    setAuthError('')
    void apiJson<Permissions>('/api/v1/auth/me').then((me) => {
      if (!cancelled && originalScope === getSessionScope()) setPermissions(me.permissions)
    }).catch((cause: unknown) => {
      if (!cancelled && originalScope === getSessionScope()) setAuthError(errorMessage(cause))
    })
    return () => { cancelled = true }
  }, [scope])

  useEffect(() => {
    const requestID = ++requests.current
    if (!authorized || !scope) {
      setPage(null)
      setLoading(false)
      return
    }
    const originalScope = scope
    const params = new URLSearchParams({
      limit: String(pageSize),
      offset: String(offset),
    })
    if (from) params.set('from', from)
    if (to) params.set('to', to)
    setPage(null)
    setLoading(true)
    setError('')
    void apiJson<PageResponse>(`/api/v1/imports/history?${params.toString()}`).then((data) => {
      if (requestID !== requests.current || originalScope !== getSessionScope()) return
      setPage(data)
      setLoading(false)
    }).catch((cause: unknown) => {
      if (requestID !== requests.current || originalScope !== getSessionScope()) return
      setPage(null)
      setError(errorMessage(cause))
      setLoading(false)
    })
    return () => { ++requests.current }
  }, [authorized, scope, offset, from, to, refresh])

  function applyDates() {
    if (!valid) return
    setOffset(0)
    setFrom(fromDraft)
    setTo(toDraft)
    setRefresh((n) => n + 1)
  }

  function clearDates() {
    setFromDraft('')
    setToDraft('')
    setFrom('')
    setTo('')
    setOffset(0)
    setRefresh((n) => n + 1)
  }

  return (
    <div>
      <h2 className="text-base font-semibold">Histórico administrativo de importações</h2>
      <p className="mt-2 text-sm text-gray-600">
        Confira, em uma só lista, as importações de produtos e os lançamentos de estoque inicial
        concluídos nesta loja. Cada tipo depende da permissão da sua conta.
      </p>
      {authError ? <p role="alert" className="mt-3 text-sm text-red-700">{authError}</p> : null}
      {permissions && !authorized ? (
        <p role="alert" className="mt-3 text-sm text-amber-800">
          Você não tem permissão para consultar nenhum dos históricos desta loja.
        </p>
      ) : null}
      {authorized ? (
        <>
          <div className="mt-4 flex flex-wrap gap-2 text-xs">
            {canProducts ? <span className="rounded-md border bg-gray-50 px-3 py-2">Produtos autorizados</span> : null}
            {canStock ? <span className="rounded-md border bg-gray-50 px-3 py-2">Estoque inicial autorizado</span> : null}
          </div>
          <section aria-label="Filtros do histórico administrativo" className="mt-4 rounded-md border p-3">
            <div className="flex flex-wrap items-end gap-3">
              <label className="text-xs">Data inicial
                <input type="date" value={fromDraft} onChange={(e) => setFromDraft(e.target.value)}
                  className="mt-1 block rounded-md border px-2 py-2 text-sm" />
              </label>
              <label className="text-xs">Data final
                <input type="date" value={toDraft} onChange={(e) => setToDraft(e.target.value)}
                  className="mt-1 block rounded-md border px-2 py-2 text-sm" />
              </label>
              <button type="button" onClick={applyDates} disabled={loading || !valid}
                className="rounded-md border px-3 py-2 text-sm disabled:opacity-50">
                Filtrar período
              </button>
              <button type="button" onClick={clearDates} disabled={loading}
                className="rounded-md border px-3 py-2 text-sm disabled:opacity-50">
                Limpar filtros
              </button>
              <button type="button" onClick={() => setRefresh((n) => n + 1)} disabled={loading}
                className="rounded-md border px-3 py-2 text-sm disabled:opacity-50">
                Atualizar
              </button>
            </div>
            {!valid ? <p role="alert" className="mt-2 text-xs text-red-700">
              O período deve estar em ordem e ter até 365 dias entre as datas.
            </p> : null}
            <p className="mt-2 text-xs text-gray-600">
              Datas no calendário de Brasília. Somente lotes confirmados aparecem aqui;
              uma falha de conexão não confirma nem desfaz a importação.
            </p>
          </section>
          {loading ? <p role="status" className="mt-4 text-sm">Consultando lotes…</p> : null}
          {error ? <p role="alert" className="mt-4 text-sm text-red-700">
            Não foi possível consultar o histórico: {error}
          </p> : null}
          {!loading && page && (page.items?.length ?? 0) === 0 ? (
            <p className="mt-4 text-sm text-gray-600">Nenhum lote confirmado neste período.</p>
          ) : null}
          {!loading && page && (page.items?.length ?? 0) > 0 ? (
            <div className="mt-4 overflow-x-auto rounded-md border">
              <table className="min-w-full text-left text-xs">
                <thead className="bg-gray-50 text-gray-700">
                  <tr>
                    <th className="px-3 py-2">Data</th>
                    <th className="px-3 py-2">Tipo</th>
                    <th className="px-3 py-2">Responsável</th>
                    <th className="px-3 py-2">Itens</th>
                    <th className="px-3 py-2">Lote</th>
                    <th className="px-3 py-2">Área</th>
                  </tr>
                </thead>
                <tbody className="divide-y">
                  {(page.items ?? []).map((item) => (
                    <tr key={item.kind + ':' + item.batch_id}>
                      <td className="px-3 py-2">
                        {new Date(item.created_at).toLocaleString('pt-BR', { timeZone: 'America/Sao_Paulo' })}
                      </td>
                      <td className="px-3 py-2">{item.kind === 'products' ? 'Cadastro de produtos' : 'Estoque inicial'}</td>
                      <td className="px-3 py-2">{item.actor_name}</td>
                      <td className="px-3 py-2">{item.item_count}</td>
                      <td className="px-3 py-2"><code className="break-all">{item.batch_id}</code></td>
                      <td className="px-3 py-2">
                        <Link to={item.kind === 'products' ? '/products' : '/inventory'}
                          className="text-blue-700 underline">
                          Abrir área
                        </Link>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}
          <div className="mt-4 flex flex-wrap items-center gap-3 text-xs">
            <button type="button" onClick={() => setOffset((n) => Math.max(0, n - pageSize))}
              disabled={loading || offset === 0} className="rounded-md border px-3 py-2 disabled:opacity-50">
              Página anterior
            </button>
            <span>Página {Math.floor(offset / pageSize) + 1}</span>
            <button type="button" onClick={() => setOffset((n) => n + pageSize)}
              disabled={loading || !page?.has_more || offset + pageSize > maxOffset}
              className="rounded-md border px-3 py-2 disabled:opacity-50">
              Próxima página
            </button>
          </div>
          <p className="mt-4 text-xs text-gray-600">
            Para baixar CSV, abra a área de Produtos ou Estoque e use o histórico específico.
            Não há exportação consolidada nem acesso ao conteúdo das planilhas nesta tela.
          </p>
        </>
      ) : null}
    </div>
  )
}
