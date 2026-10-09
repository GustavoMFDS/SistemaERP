import { useEffect, useRef, useState } from 'react'
import { apiJson, errorMessage } from '../lib/api'
import { getSessionScope } from '../lib/auth'

type Receipt = {
  batch_id: string
  item_count: number
  actor_name: string
  created_at: string
}

type Page = {
  items: Receipt[]
  limit: number
  offset: number
  has_more: boolean
}

type Props = {
  kind: 'products' | 'opening-stock'
  refreshVersion?: number
}

const pageSize = 10

export default function ImportBatchHistory({ kind, refreshVersion = 0 }: Props) {
  const [offset, setOffset] = useState(0)
  const [page, setPage] = useState<Page | null>(null)
  const [refreshCounter, setRefreshCounter] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const requestSequence = useRef(0)
  const scope = getSessionScope()

  const endpoint = kind === 'products'
    ? '/api/v1/products/import-batches/history'
    : '/api/v1/inventory/opening-stock/batches/history'

  useEffect(() => {
    const requestID = ++requestSequence.current
    const requestScope = getSessionScope()
    if (!requestScope) {
      setPage(null)
      setError('Entre na loja para consultar o histórico.')
      setLoading(false)
      return
    }
    setLoading(true)
    setPage(null)
    setError('')
    void apiJson<Page>(`${endpoint}?limit=${pageSize}&offset=${offset}`).then((data) => {
      if (requestID !== requestSequence.current || requestScope !== getSessionScope()) return
      setPage(data)
      setLoading(false)
    }).catch((cause: unknown) => {
      if (requestID !== requestSequence.current || requestScope !== getSessionScope()) return
      setPage(null)
      setError(errorMessage(cause))
      setLoading(false)
    })
    return () => { ++requestSequence.current }
  }, [endpoint, offset, refreshCounter, refreshVersion, scope])

  return (
    <section className="mt-5 rounded-md border p-3" aria-label="Histórico de importações confirmadas">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h4 className="text-sm font-semibold">Histórico de importações confirmadas</h4>
          <p className="mt-1 text-xs text-gray-600">
            Somente lotes concluídos nesta loja. Uma tentativa sem confirmação pode não aparecer.
            Não mostra os dados da planilha.
          </p>
        </div>
        <button type="button" disabled={loading}
          onClick={() => setRefreshCounter((value) => value + 1)}
          className="rounded-md border px-3 py-2 text-xs disabled:opacity-50">
          Atualizar histórico
        </button>
      </div>
      {loading ? <p role="status" className="mt-3 text-xs text-gray-600">Consultando histórico…</p> : null}
      {error ? (
        <p role="alert" className="mt-3 text-xs text-red-700">
          Não foi possível consultar o histórico: {error}
        </p>
      ) : null}
      {!loading && page && page.items.length === 0 ? (
        <p className="mt-3 text-xs text-gray-600">Nenhum lote confirmado nesta página.</p>
      ) : null}
      {!loading && page && page.items.length > 0 ? (
        <div className="mt-3 overflow-auto">
          <table className="min-w-full text-left text-xs">
            <thead className="bg-gray-50 text-gray-600">
              <tr>
                <th className="px-2 py-2">Data</th>
                <th className="px-2 py-2">Responsável</th>
                <th className="px-2 py-2">Produtos</th>
                <th className="px-2 py-2">Identificador do lote</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {page.items.map((item) => (
                <tr key={item.batch_id}>
                  <td className="px-2 py-2">{new Date(item.created_at).toLocaleString('pt-BR')}</td>
                  <td className="px-2 py-2">{item.actor_name}</td>
                  <td className="px-2 py-2">{item.item_count}</td>
                  <td className="px-2 py-2"><code className="break-all">{item.batch_id}</code></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
      <div className="mt-3 flex flex-wrap items-center gap-3 text-xs">
        <button type="button" disabled={loading || offset === 0}
          onClick={() => setOffset((value) => Math.max(0, value - pageSize))}
          className="rounded-md border px-3 py-2 disabled:opacity-50">
          Página anterior
        </button>
        <span>Página {Math.floor(offset / pageSize) + 1}</span>
        <button type="button" disabled={loading || !page?.has_more}
          onClick={() => setOffset((value) => value + pageSize)}
          className="rounded-md border px-3 py-2 disabled:opacity-50">
          Próxima página
        </button>
      </div>
    </section>
  )
}
