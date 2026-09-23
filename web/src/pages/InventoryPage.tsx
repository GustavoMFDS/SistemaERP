import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { apiJson, errorMessage } from '../lib/api'

type Product = {
  id: string
  sku: string
  name: string
  unit: string
  min_stock: number
  qty_on_hand: number
  active: boolean
  price_cash: number
}

type LowStockResponse = { items: Product[] }

type ProductsListResponse = { items: Product[]; total: number }

type AdjustRequest = {
  product_id: string
  delta: number
  reason: string
  type: 'adjustment' | 'loss' | 'damage'
}

export default function InventoryPage() {
  const [low, setLow] = useState<Product[]>([])
  const [products, setProducts] = useState<Product[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [canAdjustPermission, setCanAdjustPermission] = useState(false)

  const [productId, setProductId] = useState('')
  const [delta, setDelta] = useState<number>(0)
  const [reason, setReason] = useState('')
  const [type, setType] = useState<AdjustRequest['type']>('adjustment')
  const canAdjust = useMemo(
    () => productId && delta !== 0 && reason.trim().length >= 3,
    [productId, delta, reason],
  )

  async function load() {
    setError('')
    setLoading(true)
    try {
      const [lowRes, prodRes] = await Promise.all([
        apiJson<LowStockResponse>('/api/v1/inventory/low-stock?limit=50'),
        apiJson<ProductsListResponse>('/api/v1/products?limit=200&offset=0'),
      ])
      setLow(lowRes.items)
      setProducts(prodRes.items)
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    void apiJson<{ permissions: string[] }>('/api/v1/auth/me')
      .then((me) => setCanAdjustPermission(me.permissions.includes('inventory:adjust')))
      .catch((e: unknown) => setError(errorMessage(e)))
  }, [])

  async function onAdjust(e: FormEvent) {
    e.preventDefault()
    if (!canAdjust) return
    setError('')
    try {
      const payload: AdjustRequest = {
        product_id: productId,
        delta: Number(delta),
        reason: reason.trim(),
        type,
      }
      await apiJson('/api/v1/inventory/adjust', { method: 'POST', body: payload })
      setDelta(0)
      setReason('')
      await load()
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  return (
    <div>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Estoque</h2>
          <p className="text-sm text-gray-600">Ajuste manual e alerta de baixo estoque.</p>
        </div>
        <button
          onClick={() => void load()}
          className="rounded-md border px-3 py-2 text-sm hover:bg-gray-50"
          disabled={loading}
        >
          {loading ? 'Atualizando…' : 'Atualizar'}
        </button>
      </div>

      {error ? (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
          {error}
        </div>
      ) : null}

      <div className="mt-4">
        <h3 className="text-sm font-semibold">Baixo estoque</h3>
        <div className="mt-2 overflow-auto rounded-md border">
          <table className="min-w-full text-left text-sm">
            <thead className="bg-gray-50 text-xs text-gray-600">
              <tr>
                <th className="px-3 py-2">SKU</th>
                <th className="px-3 py-2">Produto</th>
                <th className="px-3 py-2">Qtd</th>
                <th className="px-3 py-2">Min</th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {low.map((p) => (
                <tr key={p.id}>
                  <td className="px-3 py-2 font-mono text-xs">{p.sku}</td>
                  <td className="px-3 py-2">{p.name}</td>
                  <td className="px-3 py-2">{p.qty_on_hand.toFixed(2)}</td>
                  <td className="px-3 py-2">{p.min_stock.toFixed(2)}</td>
                </tr>
              ))}
              {low.length === 0 ? (
                <tr>
                  <td className="px-3 py-6 text-center text-sm text-gray-500" colSpan={4}>
                    Nenhum item com baixo estoque.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </div>

      {canAdjustPermission ? (
        <div className="mt-6">
          <h3 className="text-sm font-semibold">Ajuste de estoque</h3>
        <form onSubmit={onAdjust} className="mt-2 grid grid-cols-1 gap-3 md:grid-cols-4">
          <label className="block md:col-span-2">
            <span className="text-xs text-gray-600">Produto</span>
            <select
              value={productId}
              onChange={(e) => setProductId(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              required
            >
              <option value="">Selecione…</option>
              {products.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.sku} — {p.name}
                </option>
              ))}
            </select>
          </label>

          <label className="block">
            <span className="text-xs text-gray-600">Tipo</span>
            <select
              value={type}
              onChange={(e) => setType(e.target.value as AdjustRequest['type'])}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            >
              <option value="adjustment">Ajuste</option>
              <option value="loss">Perda</option>
              <option value="damage">Avaria</option>
            </select>
          </label>

          <label className="block">
            <span className="text-xs text-gray-600">Delta</span>
            <input
              value={String(delta)}
              onChange={(e) => setDelta(Number(e.target.value))}
              type="number"
              step="0.01"
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              required
            />
          </label>

          <label className="block md:col-span-4">
            <span className="text-xs text-gray-600">Motivo</span>
            <input
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              placeholder="Ex.: Entrada por compra, ajuste de inventário…"
              required
            />
          </label>

          <div className="md:col-span-4">
            <button
              disabled={!canAdjust}
              className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
            >
              Aplicar ajuste
            </button>
          </div>
        </form>
        </div>
      ) : null}
    </div>
  )
}
