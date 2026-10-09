import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { apiJson, errorMessage } from '../lib/api'
import { getSessionScope } from '../lib/auth'

type Movement = {
  id: string
  product_id: string
  product_sku: string
  product_name: string
  movement_type: string
  delta: number
  qty_before: number
  qty_after: number
  reason?: string | null
  created_at: string
}
type PageResponse = { items: Movement[] | null; total: number }

const pageSize = 20
const maxOffset = 5000
const number = (n: number) => new Intl.NumberFormat('pt-BR', {
  minimumFractionDigits: 0, maximumFractionDigits: 3,
}).format(Number(n))

const typeLabels: Record<string, string> = {
  purchase: 'Recebimento de compra',
  sale: 'Venda',
  return: 'Devolução',
  adjustment: 'Ajuste de estoque',
  loss: 'Perda',
  damage: 'Avaria',
}

export default function StockMovementsPage() {
  const [items, setItems] = useState<Movement[]>([])
  const [total, setTotal] = useState(0)
  const [offset, setOffset] = useState(0)
  const [product, setProduct] = useState<{ id: string; name: string } | null>(null)
  const [reload, setReload] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const seq = useRef(0)
  const scope = getSessionScope()

  useEffect(() => {
    const id = ++seq.current
    const originalScope = getSessionScope()
    setItems([])
    setTotal(0)
    setLoading(true)
    setError('')
    if (!originalScope) {
      setLoading(false)
      setError('Entre na loja para consultar movimentações.')
      return
    }
    const params = new URLSearchParams({ limit: String(pageSize), offset: String(offset) })
    if (product) params.set('product_id', product.id)
    void apiJson<PageResponse>(`/api/v1/inventory/movements?${params.toString()}`).then((data) => {
      if (id !== seq.current || originalScope !== getSessionScope()) return
      setItems(data.items ?? [])
      setTotal(data.total)
      setLoading(false)
    }).catch((cause: unknown) => {
      if (id !== seq.current || originalScope !== getSessionScope()) return
      setError(errorMessage(cause))
      setLoading(false)
    })
    return () => { ++seq.current }
  }, [offset, product, reload, scope])

  return (
    <div>
      <h2 className="text-lg font-semibold">Movimentações de estoque</h2>
      <p className="mt-2 text-sm text-gray-600">
        Entradas, saídas e ajustes registrados nesta loja, com o saldo anterior e o novo saldo.
        Esta página é apenas para consulta; não altera o estoque.
      </p>
      <div className="mt-4 flex flex-wrap items-center gap-2">
        <Link className="rounded-md border px-3 py-2 text-xs text-blue-700" to="/inventory">
          Voltar ao estoque
        </Link>
        <button type="button" className="rounded-md border px-3 py-2 text-xs disabled:opacity-50"
          disabled={loading} onClick={() => setReload((n) => n + 1)}>Atualizar</button>
        {product ? (
          <>
            <span className="text-xs text-gray-700">Produto: {product.name}</span>
            <button type="button" className="rounded-md border px-3 py-2 text-xs"
              disabled={loading} onClick={() => { setProduct(null); setOffset(0) }}>
              Todos os produtos
            </button>
          </>
        ) : null}
      </div>
      {loading ? <p className="mt-3 text-sm" role="status">Consultando movimentações…</p> : null}
      {error ? <p className="mt-3 text-sm text-red-700" role="alert">
        Não foi possível consultar: {error}
      </p> : null}
      {!loading && !error && items.length === 0 ? (
        <p className="mt-4 text-sm text-gray-600">Nenhuma movimentação encontrada.</p>
      ) : null}
      {!loading && items.length > 0 ? (
        <div className="mt-4 overflow-x-auto rounded-md border">
          <table className="min-w-full text-left text-xs">
            <thead className="bg-gray-50 text-gray-700">
              <tr>
                <th className="px-3 py-2">Data</th>
                <th className="px-3 py-2">Produto</th>
                <th className="px-3 py-2">Movimento</th>
                <th className="px-3 py-2">Diferença</th>
                <th className="px-3 py-2">Antes</th>
                <th className="px-3 py-2">Depois</th>
                <th className="px-3 py-2">Motivo</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {items.map((entry) => (
                <tr key={entry.id}>
                  <td className="px-3 py-2">
                    {new Date(entry.created_at).toLocaleString('pt-BR', { timeZone: 'America/Sao_Paulo' })}
                  </td>
                  <td className="px-3 py-2">
                    <button type="button" className="text-left text-blue-700 underline"
                      onClick={() => {
                        setProduct({ id: entry.product_id, name: entry.product_name })
                        setOffset(0)
                      }}>
                      {entry.product_name}
                    </button>
                    <span className="block text-gray-500">{entry.product_sku}</span>
                  </td>
                  <td className="px-3 py-2">{typeLabels[entry.movement_type] ?? entry.movement_type}</td>
                  <td className="px-3 py-2 font-medium">
                    {entry.delta > 0 ? '+' : ''}{number(entry.delta)}
                  </td>
                  <td className="px-3 py-2">{number(entry.qty_before)}</td>
                  <td className="px-3 py-2">{number(entry.qty_after)}</td>
                  <td className="px-3 py-2">{entry.reason || '—'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <div className="mt-4 flex flex-wrap items-center gap-3 text-xs">
        <button type="button" disabled={loading || offset === 0}
          onClick={() => setOffset((n) => Math.max(0, n - pageSize))}
          className="rounded-md border px-3 py-2 disabled:opacity-50">
          Página anterior
        </button>
        <span>Página {Math.floor(offset / pageSize) + 1} — {total} movimentações</span>
        <button type="button" disabled={loading || items.length < pageSize || offset + pageSize >= total || offset + pageSize > maxOffset}
          onClick={() => setOffset((n) => n + pageSize)}
          className="rounded-md border px-3 py-2 disabled:opacity-50">
          Próxima página
        </button>
        {offset >= maxOffset ? <span className="text-amber-800">Limite de consulta atingido.</span> : null}
      </div>
      <p className="mt-4 text-xs text-gray-600">
        Esta lista reflete os lançamentos registrados pelo sistema. Não substitui contagem física
        ou conciliação independente dos saldos em prateleira.
      </p>
    </div>
  )
}
