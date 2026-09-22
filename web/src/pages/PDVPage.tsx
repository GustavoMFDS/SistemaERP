import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import { apiJson, errorMessage } from '../lib/api'
import { enqueueRequest, flushQueue, getQueueSummary } from '../lib/offlineQueue'
import {
  clearCashSessionId,
  getCashSessionId,
  setCashSessionId,
  scopedStorageKey,
} from '../lib/auth'

type Product = {
  id: string
  sku: string
  name: string
  unit: string
  price_cash: number
  active: boolean
}

type ProductsListResponse = { items: Product[]; total: number }

type CashOpenResponse = { id: string }

type SaleCreateResponse = { id: string; status: string; total: number }

type SaleItem = {
  product_id: string
  qty: number
  unit_price: number
  discount_value: number
}

type SalePayment = { method: string; amount: number }

const PRODUCTS_CACHE_NAMESPACE = 'sistemaemgo:productsCache:v2'

export default function PDVPage() {
  const [products, setProducts] = useState<Product[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const [online, setOnline] = useState<boolean>(navigator.onLine)
  const initialQueue = getQueueSummary()
  const [pendingSync, setPendingSync] = useState<number>(initialQueue.pending)
  const [attentionSync, setAttentionSync] = useState<number>(initialQueue.attention)

  const [cashSessionId, setCashSessionIdState] = useState(getCashSessionId())
  const [openingAmount, setOpeningAmount] = useState<number>(0)

  const [itemProductId, setItemProductId] = useState('')
  const [itemQty, setItemQty] = useState<number>(1)
  const [items, setItems] = useState<SaleItem[]>([])

  const [payMethod, setPayMethod] = useState('pix')
  const [saleId, setSaleId] = useState('')
  const [saleTotal, setSaleTotal] = useState<number>(0)

  const productById = useMemo(() => {
    const map = new Map<string, Product>()
    for (const p of products) map.set(p.id, p)
    return map
  }, [products])

  const computedTotal = useMemo(() => {
    let t = 0
    for (const it of items) t += it.unit_price * it.qty - it.discount_value
    return Math.max(0, Math.round(t * 100) / 100)
  }, [items])

  function refreshPending() {
    const summary = getQueueSummary()
    setPendingSync(summary.pending)
    setAttentionSync(summary.attention)
  }

  async function syncPending() {
    if (!navigator.onLine) {
      refreshPending()
      return
    }
    const res = await flushQueue()
    refreshPending()
    if (res.ok === false) {
      setError(`Falha ao sincronizar pendências: ${res.error}`)
    } else if (res.attention > 0) {
      setError(`${res.attention} venda(s) offline requer(em) atenção manual e foram preservadas localmente.`)
    }
  }

  async function loadProducts() {
    setError('')
    setLoading(true)
    try {
      const data = await apiJson<ProductsListResponse>('/api/v1/products?limit=200&offset=0')
      const active = data.items.filter((p) => p.active)
      setProducts(active)
      const cacheKey = scopedStorageKey(PRODUCTS_CACHE_NAMESPACE)
      if (cacheKey) localStorage.setItem(cacheKey, JSON.stringify(active))
    } catch (e: unknown) {
      const cacheKey = scopedStorageKey(PRODUCTS_CACHE_NAMESPACE)
      const cachedRaw = cacheKey ? localStorage.getItem(cacheKey) : null
      if (cachedRaw) {
        try {
          const cached = JSON.parse(cachedRaw) as Product[]
          if (Array.isArray(cached) && cached.length > 0) {
            setProducts(cached)
            return
          }
        } catch {
          // ignore
        }
      }
      setError(errorMessage(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadProducts()
  }, [])

  useEffect(() => {
    function onOnline() {
      setOnline(true)
      void syncPending()
    }
    function onOffline() {
      setOnline(false)
      refreshPending()
    }
    window.addEventListener('online', onOnline)
    window.addEventListener('offline', onOffline)

    // Attempt a sync when opening the PDV page.
    if (navigator.onLine) {
      void syncPending()
    }

    return () => {
      window.removeEventListener('online', onOnline)
      window.removeEventListener('offline', onOffline)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function openCash(e: FormEvent) {
    e.preventDefault()
    setError('')
    try {
      const res = await apiJson<CashOpenResponse>('/api/v1/cash/sessions/open', {
        method: 'POST',
        body: { opening_amount: Number(openingAmount) || 0, notes: null },
      })
      setCashSessionId(res.id)
      setCashSessionIdState(res.id)
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  function clearCash() {
    clearCashSessionId()
    setCashSessionIdState('')
  }

  function addItem() {
    const p = productById.get(itemProductId)
    if (!p) return
    const next: SaleItem = {
      product_id: p.id,
      qty: Number(itemQty) || 1,
      unit_price: Number(p.price_cash) || 0,
      discount_value: 0,
    }
    setItems((prev) => [...prev, next])
  }

  function removeItem(idx: number) {
    setItems((prev) => prev.filter((_, i) => i !== idx))
  }

  const canFinalize = useMemo(
    () => cashSessionId && items.length > 0 && computedTotal > 0,
    [cashSessionId, items.length, computedTotal],
  )

  async function finalizeSale() {
    if (!canFinalize) return
    setError('')
    setSaleId('')
    setSaleTotal(0)

    const payments: SalePayment[] = [
      { method: payMethod, amount: computedTotal },
    ]
    const body = {
      cash_session_id: cashSessionId,
      customer_id: null,
      discount_value: 0,
      items: items.map(({ product_id, qty, discount_value }) => ({
        product_id,
        qty,
        discount_value,
      })),
      payments,
    }
    // Generate this exactly once. If the response is lost after the backend
    // commits, the queued retry must use the same key to replay safely.
    const idempotencyKey = crypto.randomUUID()

    try {
      if (!navigator.onLine) {
        const queuedId = enqueueRequest({
          method: 'POST',
          path: '/api/v1/sales',
          body,
          headers: { 'Idempotency-Key': idempotencyKey },
        })
        setSaleId(`offline:${queuedId}`)
        setSaleTotal(computedTotal)
        setItems([])
        refreshPending()
        return
      }

      const res = await apiJson<SaleCreateResponse>('/api/v1/sales', {
        method: 'POST',
        headers: { 'Idempotency-Key': idempotencyKey },
        body,
      })

      setSaleId(res.id)
      setSaleTotal(res.total)
      setItems([])
    } catch (e: unknown) {
      const msg = errorMessage(e)
      const networkLike =
        !navigator.onLine ||
        msg.includes('NetworkError') ||
        msg.includes('Failed to fetch')

      if (networkLike) {
        try {
          const queuedId = enqueueRequest({
            method: 'POST',
            path: '/api/v1/sales',
            body,
            headers: { 'Idempotency-Key': idempotencyKey },
          })
          setSaleId(`offline:${queuedId}`)
          setSaleTotal(computedTotal)
          setItems([])
          refreshPending()
          return
        } catch {
          // fall-through
        }
      }

      setError(msg)
    }
  }

  return (
    <div>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">PDV</h2>
          <p className="text-sm text-gray-600">Abrir caixa e registrar venda finalizada.</p>
          <p className="mt-1 text-xs text-gray-600">
            Status: {online ? 'online' : 'offline'} • Pendências: {pendingSync} • Atenção: {attentionSync}
          </p>
        </div>
        <button
          onClick={() => void loadProducts()}
          className="rounded-md border px-3 py-2 text-sm hover:bg-gray-50"
          disabled={loading}
        >
          {loading ? 'Atualizando…' : 'Atualizar produtos'}
        </button>
      </div>

      {error ? (
        <div className="mt-3 rounded-md border border-red-200 bg-red-50 p-2 text-sm text-red-700">
          {error}
        </div>
      ) : null}

      <div className="mt-4 rounded-md border p-3">
        <h3 className="text-sm font-semibold">Sessão de caixa</h3>
        <div className="mt-2 flex flex-col gap-2 md:flex-row md:items-end">
          <form onSubmit={openCash} className="flex flex-1 items-end gap-2">
            <label className="block flex-1">
              <span className="text-xs text-gray-600">Abertura (R$)</span>
              <input
                value={String(openingAmount)}
                onChange={(e) => setOpeningAmount(Number(e.target.value))}
                type="number"
                step="0.01"
                className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
              />
            </label>
            <button className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white">
              Abrir
            </button>
          </form>

          <div className="flex flex-1 items-end justify-between gap-2">
            <div className="text-sm">
              <div className="text-xs text-gray-600">cash_session_id</div>
              <div className="font-mono text-xs">{cashSessionId || '—'}</div>
            </div>
            <button
              onClick={clearCash}
              className="rounded-md border px-3 py-2 text-sm hover:bg-gray-50"
            >
              Limpar
            </button>
          </div>
        </div>
      </div>

      <div className="mt-4 rounded-md border p-3">
        <h3 className="text-sm font-semibold">Itens</h3>
        <div className="mt-2 grid grid-cols-1 gap-2 md:grid-cols-6">
          <label className="block md:col-span-4">
            <span className="text-xs text-gray-600">Produto</span>
            <select
              value={itemProductId}
              onChange={(e) => setItemProductId(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            >
              <option value="">Selecione…</option>
              {products.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.sku} — {p.name}
                </option>
              ))}
            </select>
          </label>
          <label className="block md:col-span-1">
            <span className="text-xs text-gray-600">Qtd</span>
            <input
              value={String(itemQty)}
              onChange={(e) => setItemQty(Number(e.target.value))}
              type="number"
              step="0.01"
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            />
          </label>
          <div className="md:col-span-1">
            <button
              type="button"
              onClick={addItem}
              className="mt-5 w-full rounded-md border px-3 py-2 text-sm hover:bg-gray-50"
            >
              Adicionar
            </button>
          </div>
        </div>

        <div className="mt-3 overflow-auto rounded-md border">
          <table className="min-w-full text-left text-sm">
            <thead className="bg-gray-50 text-xs text-gray-600">
              <tr>
                <th className="px-3 py-2">Produto</th>
                <th className="px-3 py-2">Qtd</th>
                <th className="px-3 py-2">Preço</th>
                <th className="px-3 py-2">Total</th>
                <th className="px-3 py-2"></th>
              </tr>
            </thead>
            <tbody className="divide-y">
              {items.map((it, idx) => {
                const p = productById.get(it.product_id)
                const name = p ? `${p.sku} — ${p.name}` : it.product_id
                return (
                  <tr key={idx}>
                    <td className="px-3 py-2">{name}</td>
                    <td className="px-3 py-2">{it.qty.toFixed(2)}</td>
                    <td className="px-3 py-2">{it.unit_price.toFixed(2)}</td>
                    <td className="px-3 py-2">
                      {(it.unit_price * it.qty - it.discount_value).toFixed(2)}
                    </td>
                    <td className="px-3 py-2">
                      <button
                        type="button"
                        onClick={() => removeItem(idx)}
                        className="text-xs text-red-700 hover:underline"
                      >
                        Remover
                      </button>
                    </td>
                  </tr>
                )
              })}
              {items.length === 0 ? (
                <tr>
                  <td className="px-3 py-6 text-center text-sm text-gray-500" colSpan={5}>
                    Nenhum item.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>

        <div className="mt-3 flex flex-col gap-2 md:flex-row md:items-end md:justify-between">
          <div className="text-sm">
            <div className="text-xs text-gray-600">Total</div>
            <div className="text-lg font-semibold">R$ {computedTotal.toFixed(2)}</div>
          </div>

          <div className="flex items-end gap-2">
            <label className="block">
              <span className="text-xs text-gray-600">Pagamento</span>
              <select
                value={payMethod}
                onChange={(e) => setPayMethod(e.target.value)}
                className="mt-1 rounded-md border px-3 py-2 text-sm"
              >
                <option value="cash">Dinheiro</option>
                <option value="pix">Pix</option>
                <option value="debit">Débito</option>
                <option value="credit">Crédito</option>
                <option value="transfer">Transferência</option>
                <option value="voucher">Voucher</option>
              </select>
            </label>
            <button
              type="button"
              onClick={() => void finalizeSale()}
              disabled={!canFinalize}
              className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
            >
              Finalizar
            </button>
          </div>
        </div>

        {saleId ? (
          <div className="mt-3 rounded-md border border-green-200 bg-green-50 p-2 text-sm text-green-700">
            {saleId.startsWith('offline:') ? (
              <>
                Venda registrada offline (pendente sync): <span className="font-mono text-xs">{saleId.replace('offline:', '')}</span> • Total R$ {saleTotal.toFixed(2)}
              </>
            ) : (
              <>
                Venda finalizada: <span className="font-mono text-xs">{saleId}</span> • Total R$ {saleTotal.toFixed(2)}
              </>
            )}
          </div>
        ) : null}
      </div>
    </div>
  )
}
