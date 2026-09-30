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
  ncm?: string | null
  cest?: string | null
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
  ncm?: string | null
  cest?: string | null
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
  const [barcodeDrafts, setBarcodeDrafts] = useState<Record<string, string>>({})
  const [ncmDrafts, setNcmDrafts] = useState<Record<string, string>>({})
  const [cestDrafts, setCestDrafts] = useState<Record<string, string>>({})
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [canWrite, setCanWrite] = useState(false)

  const [sku, setSku] = useState('')
  const [barcode, setBarcode] = useState('')
  const [ncm, setNcm] = useState('')
  const [cest, setCest] = useState('')
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
    void apiJson<{ permissions: string[] }>('/api/v1/auth/me')
      .then((me) => setCanWrite(me.permissions.includes('product:write')))
      .catch((e: unknown) => setError(errorMessage(e)))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function saveCatalogFiscal(product: Product) {
    setError('')
    const nextBarcode = (barcodeDrafts[product.id] ?? product.barcode ?? '').trim()
    const nextNCM = (ncmDrafts[product.id] ?? product.ncm ?? '').trim()
    const nextCEST = (cestDrafts[product.id] ?? product.cest ?? '').trim()
    try {
      const payload: ProductCreateRequest = {
        category_id: product.category_id ?? null,
        sku: product.sku,
        barcode: nextBarcode || null,
        ncm: nextNCM || null,
        cest: nextCEST || null,
        name: product.name,
        description: product.description ?? null,
        unit: product.unit,
        cost_price: product.cost_price,
        price_cash: product.price_cash,
        promo_price: product.promo_price ?? null,
        min_stock: product.min_stock,
        active: product.active,
      }
      await apiJson<{ id: string }>(`/api/v1/products/${product.id}`, {
        method: 'PUT',
        body: payload,
      })
      setBarcodeDrafts((prev) => {
        const next = { ...prev }
        delete next[product.id]
        return next
      })
      setNcmDrafts((prev) => {
        const next = { ...prev }
        delete next[product.id]
        return next
      })
      setCestDrafts((prev) => {
        const next = { ...prev }
        delete next[product.id]
        return next
      })
      await load()
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  async function onCreate(e: FormEvent) {
    e.preventDefault()
    if (!canCreate) return
    setError('')
    try {
      const payload: ProductCreateRequest = {
        sku: sku.trim(),
        barcode: barcode.trim() || null,
        ncm: ncm.trim() || null,
        cest: cest.trim() || null,
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
      setBarcode('')
      setNcm('')
      setCest('')
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
              <th className="px-3 py-2">Código de barras</th>
              <th className="px-3 py-2">NCM</th>
              <th className="px-3 py-2">CEST</th>
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
                <td className="px-3 py-2">
                  {canWrite ? (
                    <input
                      value={barcodeDrafts[p.id] ?? p.barcode ?? ''}
                      onChange={(e) =>
                        setBarcodeDrafts((prev) => ({ ...prev, [p.id]: e.target.value }))
                      }
                      placeholder="Sem código"
                      autoComplete="off"
                      className="min-w-52 rounded-md border px-2 py-1 font-mono text-xs"
                    />
                  ) : (
                    <span className="font-mono text-xs">{p.barcode ?? '—'}</span>
                  )}
                </td>
                <td className="px-3 py-2">
                  {canWrite ? (
                    <input
                      value={ncmDrafts[p.id] ?? p.ncm ?? ''}
                      onChange={(e) =>
                        setNcmDrafts((prev) => ({ ...prev, [p.id]: e.target.value }))
                      }
                      placeholder="8 dígitos"
                      inputMode="numeric"
                      maxLength={8}
                      className="w-28 rounded-md border px-2 py-1 font-mono text-xs"
                    />
                  ) : (
                    <span className="font-mono text-xs">{p.ncm ?? '—'}</span>
                  )}
                </td>
                <td className="px-3 py-2">
                  {canWrite ? (
                    <div className="flex items-center gap-2">
                      <input
                        value={cestDrafts[p.id] ?? p.cest ?? ''}
                        onChange={(e) =>
                          setCestDrafts((prev) => ({ ...prev, [p.id]: e.target.value }))
                        }
                        placeholder="7 dígitos"
                        inputMode="numeric"
                        maxLength={7}
                        className="w-28 rounded-md border px-2 py-1 font-mono text-xs"
                      />
                      <button
                        type="button"
                        onClick={() => void saveCatalogFiscal(p)}
                        className="rounded-md border px-2 py-1 text-xs hover:bg-gray-50"
                      >
                        Salvar
                      </button>
                    </div>
                  ) : (
                    <span className="font-mono text-xs">{p.cest ?? '—'}</span>
                  )}
                </td>
                <td className="px-3 py-2">{p.name}</td>
                <td className="px-3 py-2">{p.unit}</td>
                <td className="px-3 py-2">{p.price_cash.toFixed(2)}</td>
                <td className="px-3 py-2">{p.qty_on_hand.toFixed(2)}</td>
                <td className="px-3 py-2">{p.min_stock.toFixed(2)}</td>
                <td className="px-3 py-2">{p.active ? 'Sim' : 'Não'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {canWrite ? (
        <div className="mt-6">
          <h3 className="text-sm font-semibold">Cadastrar produto</h3>
        <form onSubmit={onCreate} className="mt-2 grid grid-cols-1 gap-3 md:grid-cols-8">
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
            <span className="text-xs text-gray-600">Código de barras</span>
            <input
              value={barcode}
              onChange={(e) => setBarcode(e.target.value)}
              placeholder="EAN / GTIN"
              autoComplete="off"
              className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm"
            />
          </label>
          <label className="block">
            <span className="text-xs text-gray-600">NCM</span>
            <input
              value={ncm}
              onChange={(e) => setNcm(e.target.value)}
              placeholder="8 dígitos"
              inputMode="numeric"
              maxLength={8}
              className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm"
            />
          </label>
          <label className="block">
            <span className="text-xs text-gray-600">CEST</span>
            <input
              value={cest}
              onChange={(e) => setCest(e.target.value)}
              placeholder="7 dígitos"
              inputMode="numeric"
              maxLength={7}
              className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm"
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
              step="0.01"
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            />
          </label>

          <div className="md:col-span-8">
            <button
              disabled={!canCreate}
              className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
            >
              Cadastrar
            </button>
          </div>
        </form>
        </div>
      ) : null}
    </div>
  )
}
