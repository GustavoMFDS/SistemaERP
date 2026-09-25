import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { apiJson, errorMessage } from '../lib/api'

type Product = {
  id: string
  sku: string
  name: string
  unit: string
  category_id?: string | null
  barcode?: string | null
  description?: string | null
  cost_price: number
  price_cash: number
  promo_price?: number | null
  min_stock: number
  active: boolean
  qty_on_hand: number
}

type ListResponse = { items: Product[]; total: number }

type ProductCreateRequest = {
  category_id?: string | null
  sku: string
  barcode?: string | null
  name: string
  description?: string | null
  unit: string
  cost_price: number
  price_cash: number
  promo_price?: number | null
  min_stock: number
  active: boolean
}

export default function ProductsPage() {
  const [query, setQuery] = useState('')
  const [items, setItems] = useState<Product[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const [sku, setSku] = useState('')
  const [name, setName] = useState('')
  const [unit, setUnit] = useState('un')
  const [priceCash, setPriceCash] = useState<number>(0)
  const [minStock, setMinStock] = useState<number>(0)

  const canCreate = useMemo(() => sku.trim() && name.trim() && priceCash > 0, [
    sku,
    name,
    priceCash,
  ])

  async function load() {
    setError('')
    setLoading(true)
    try {
      const qs = new URLSearchParams()
      if (query.trim()) qs.set('query', query.trim())
      const data = await apiJson<ListResponse>(`/api/v1/products?${qs.toString()}`)
      setItems(data.items)
      setTotal(data.total)
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function onCreate(e: FormEvent) {
    e.preventDefault()
    if (!canCreate) return
    setError('')
    try {
      const payload: ProductCreateRequest = {
        sku: sku.trim(),
        name: name.trim(),
        unit: unit.trim() || 'un',
        cost_price: 0,
        price_cash: Number(priceCash),
        min_stock: Number(minStock) || 0,
        active: true,
      }
      await apiJson<{ id: string }>('/api/v1/products', {
        method: 'POST',
        body: payload,
      })
      setSku('')
      setName('')
      setPriceCash(0)
      setMinStock(0)
      await load()
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  return (
    <div>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">Produtos</h2>
          <p className="text-sm text-gray-600">Total: {total}</p>
        </div>
        <button
          onClick={() => void load()}
          className="rounded-md border px-3 py-2 text-sm hover:bg-gray-50"
          disabled={loading}
        >
          {loading ? 'Atualizando…' : 'Atualizar'}
        </button>
      </div>

      <div className="mt-4 flex gap-2">
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Buscar por nome, SKU ou código"
          className="w-full rounded-md border px-3 py-2 text-sm"
        />
        <button
          onClick={() => void load()}
          className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white"
        >
          Buscar
        </button>
      </div>

      {error ? (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
          {error}
        </div>
      ) : null}

      <div className="mt-4 overflow-auto rounded-md border">
        <table className="min-w-full text-left text-sm">
          <thead className="bg-gray-50 text-xs text-gray-600">
            <tr>
              <th className="px-3 py-2">SKU</th>
              <th className="px-3 py-2">Nome</th>
              <th className="px-3 py-2">Un</th>
              <th className="px-3 py-2">Preço</th>
              <th className="px-3 py-2">Qtd</th>
              <th className="px-3 py-2">Min</th>
              <th className="px-3 py-2">Ativo</th>
            </tr>
          </thead>
          <tbody className="divide-y">
            {items.map((p) => (
              <tr key={p.id}>
                <td className="px-3 py-2 font-mono text-xs">{p.sku}</td>
                <td className="px-3 py-2">{p.name}</td>
                <td className="px-3 py-2">{p.unit}</td>
                <td className="px-3 py-2">{p.price_cash.toFixed(2)}</td>
                <td className="px-3 py-2">{p.qty_on_hand.toFixed(3)}</td>
                <td className="px-3 py-2">{p.min_stock.toFixed(3)}</td>
                <td className="px-3 py-2">{p.active ? 'Sim' : 'Não'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="mt-6">
        <h3 className="text-sm font-semibold">Cadastrar produto</h3>
        <form onSubmit={onCreate} className="mt-2 grid grid-cols-1 gap-3 md:grid-cols-5">
          <label className="block md:col-span-1">
            <span className="text-xs text-gray-600">SKU</span>
            <input
              value={sku}
              onChange={(e) => setSku(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              required
            />
          </label>
          <label className="block md:col-span-2">
            <span className="text-xs text-gray-600">Nome</span>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              required
            />
          </label>
          <label className="block">
            <span className="text-xs text-gray-600">Unidade</span>
            <input
              value={unit}
              onChange={(e) => setUnit(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              required
            />
          </label>
          <label className="block">
            <span className="text-xs text-gray-600">Preço (à vista)</span>
            <input
              value={String(priceCash)}
              onChange={(e) => setPriceCash(Number(e.target.value))}
              type="number"
              step="0.01"
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              required
            />
          </label>
          <label className="block">
            <span className="text-xs text-gray-600">Estoque mín.</span>
            <input
              value={String(minStock)}
              onChange={(e) => setMinStock(Number(e.target.value))}
              type="number"
              step="0.001"
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            />
          </label>

          <div className="md:col-span-5">
            <button
              disabled={!canCreate}
              className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
            >
              Cadastrar
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
