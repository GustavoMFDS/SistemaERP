import { useEffect, useMemo, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { APIError, apiJson, errorMessage } from '../lib/api'
import {
  claimLegacyQueue,
  discardLegacyQueue,
  discardQueueItem,
  enqueueRequest,
  flushQueue,
  getLegacyQueueCount,
  getQueueItems,
  getQueueSummary,
  markQueueItemAttention,
  rebindQueueItemToCashSession,
  retryQueueItem,
  type QueuedRequest,
} from '../lib/offlineQueue'
import {
  clearCashSessionId,
  getCashSessionId,
  setCashSessionId,
  scopedStorageKey,
} from '../lib/auth'
import {
  getSuspendedCarts,
  removeSuspendedCart,
  suspendCart,
  type SuspendedCart,
} from '../lib/suspendedCart'

type Product = {
  id: string
  sku: string
  name: string
  unit: string
  barcode?: string | null
  price_cash: number
  qty_on_hand?: number
  active: boolean
}

type MeResponse = {
  permissions?: string[]
}

type ProductsListResponse = { items: Product[]; total: number }

type CashOpenResponse = { id: string }
type CashCloseResponse = {
  status: string
  expected_cash: number
  closing_amount: number
  closing_difference: number
  expected_by_method: Record<string, number>
  declared_by_method: Record<string, number>
  difference_by_method: Record<string, number>
}

type SaleCreateResponse = { id: string; status: string; total: number }

type SaleItem = {
  product_id: string
  qty: number
  unit_price: number
  discount_value: number
}

type SalePayment = { method: string; amount: number }

const PRODUCTS_CACHE_NAMESPACE = 'sistemaemgo:productsCache:v2'
const CLOSE_METHODS = [
  ['pix', 'PIX'],
  ['debit', 'Débito'],
  ['credit', 'Crédito'],
  ['transfer', 'Transferência'],
  ['voucher', 'Voucher'],
] as const

export default function PDVPage() {
  const [products, setProducts] = useState<Product[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const [online, setOnline] = useState<boolean>(navigator.onLine)
  const initialQueue = getQueueSummary()
  const [pendingSync, setPendingSync] = useState<number>(initialQueue.pending)
  const [attentionSync, setAttentionSync] = useState<number>(initialQueue.attention)
  const [queueItems, setQueueItems] = useState<QueuedRequest[]>(getQueueItems())
  const [legacyQueueCount, setLegacyQueueCount] = useState<number>(getLegacyQueueCount())

  const [cashSessionId, setCashSessionIdState] = useState(getCashSessionId())
  const [openingAmount, setOpeningAmount] = useState<number>(0)
  const [closingAmount, setClosingAmount] = useState<number>(0)
  const [closingByMethod, setClosingByMethod] = useState<Record<string, number>>({
    pix: 0,
    debit: 0,
    credit: 0,
    transfer: 0,
    voucher: 0,
  })
  const [movementAmount, setMovementAmount] = useState<number>(0)
  const [cashCloseSummary, setCashCloseSummary] = useState<CashCloseResponse | null>(null)

  const [barcodeScan, setBarcodeScan] = useState('')
  const barcodeInputRef = useRef<HTMLInputElement>(null)
  const productSearchRef = useRef<HTMLInputElement>(null)
  const [productQuery, setProductQuery] = useState('')
  const [itemProductId, setItemProductId] = useState('')
  const [itemQty, setItemQty] = useState<number>(1)
  const [items, setItems] = useState<SaleItem[]>([])
  const [saleDiscount, setSaleDiscount] = useState(0)
  const [canDiscount, setCanDiscount] = useState(false)
  const [suspendedCarts, setSuspendedCarts] = useState<SuspendedCart[]>(getSuspendedCarts())

  const [payMethod, setPayMethod] = useState('pix')
  const [saleId, setSaleId] = useState('')
  const [saleTotal, setSaleTotal] = useState<number>(0)
  const [finalizing, setFinalizing] = useState(false)
  const finalizeInFlight = useRef(false)

  const productById = useMemo(() => {
    const map = new Map<string, Product>()
    for (const p of products) map.set(p.id, p)
    return map
  }, [products])

  const productByBarcode = useMemo(() => {
    const map = new Map<string, Product>()
    for (const p of products) {
      const code = p.barcode?.trim()
      if (code) map.set(code, p)
    }
    return map
  }, [products])

  const filteredProducts = useMemo(() => {
    const q = productQuery.trim().toLowerCase()
    if (!q) return products
    return products.filter((p) =>
      [p.sku, p.name, p.barcode ?? ''].some((value) => value.toLowerCase().includes(q)),
    )
  }, [products, productQuery])

  const computedTotal = useMemo(() => {
    let t = 0
    for (const it of items) t += it.unit_price * it.qty - it.discount_value
    t -= canDiscount ? saleDiscount : 0
    return Math.max(0, Math.round(t * 100) / 100)
  }, [items, saleDiscount, canDiscount])

  function refreshPending() {
    const summary = getQueueSummary()
    setPendingSync(summary.pending)
    setAttentionSync(summary.attention)
    setQueueItems(getQueueItems())
    setLegacyQueueCount(getLegacyQueueCount())
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
    apiJson<MeResponse>('/api/v1/auth/me')
      .then((me) => {
        const allowed = Boolean(me.permissions?.includes('sale:discount'))
        setCanDiscount(allowed)
        if (!allowed) setSaleDiscount(0)
      })
      .catch(() => {
        setCanDiscount(false)
        setSaleDiscount(0)
      })
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
      setCashCloseSummary(null)
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  async function recordCashMovement(movementType: 'supply' | 'withdrawal') {
    if (!cashSessionId || movementAmount <= 0) return
    setError('')
    try {
      await apiJson(`/api/v1/cash/sessions/${cashSessionId}/movements`, {
        method: 'POST',
        body: {
          movement_type: movementType,
          amount: Number(movementAmount) || 0,
          notes: null,
        },
      })
      setMovementAmount(0)
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  async function closeCash() {
    if (!cashSessionId) return
    setError('')
    try {
      const result = await apiJson<CashCloseResponse>(
        `/api/v1/cash/sessions/${cashSessionId}/close`,
        {
          method: 'POST',
          body: {
            closing_amount: Number(closingAmount) || 0,
            closing_by_method: closingByMethod,
            notes: null,
          },
        },
      )
      setCashCloseSummary(result)
      clearCashSessionId()
      setCashSessionIdState('')
      setClosingAmount(0)
      setClosingByMethod({
        pix: 0,
        debit: 0,
        credit: 0,
        transfer: 0,
        voucher: 0,
      })
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  async function retryAttention(id: string) {
    if (!retryQueueItem(id)) return
    await syncPending()
  }

  function discardAttention(id: string) {
    if (!window.confirm('Descartar esta venda offline preservada? Esta ação não pode ser desfeita.')) {
      return
    }
    discardQueueItem(id)
    refreshPending()
  }

  async function rebindAttentionToCurrentCash(id: string) {
    if (!cashSessionId) return
    if (
      !window.confirm(
        'Recriar esta venda para o caixa atual preservando a Idempotency-Key original? Se a venda antiga já tiver sido processada, o backend bloqueará a duplicação.',
      )
    ) {
      return
    }
    if (!rebindQueueItemToCashSession(id, cashSessionId)) return
    await syncPending()
  }

  function importLegacyQueue() {
    const imported = claimLegacyQueue()
    refreshPending()
    if (imported > 0) {
      setError(
        `${imported} item(ns) legado(s) importado(s) como atenção. Revise cada item antes de reenviar.`,
      )
    }
  }

  function removeLegacyQueue() {
    if (!window.confirm('Descartar a fila offline legada deste navegador?')) return
    discardLegacyQueue()
    refreshPending()
  }

  function addProductToCart(p: Product, qty = 1) {
    const normalizedQty = Number(qty) || 1
    setItems((prev) => {
      const idx = prev.findIndex((item) => item.product_id === p.id && item.discount_value === 0)
      if (idx >= 0) {
        return prev.map((item, i) =>
          i === idx ? { ...item, qty: item.qty + normalizedQty } : item,
        )
      }
      return [
        ...prev,
        {
          product_id: p.id,
          qty: normalizedQty,
          unit_price: Number(p.price_cash) || 0,
          discount_value: 0,
        },
      ]
    })
  }

  function addItem() {
    const p = productById.get(itemProductId)
    if (!p) return
    addProductToCart(p, itemQty)
  }

  async function scanBarcode(e: FormEvent) {
    e.preventDefault()
    const code = barcodeScan.trim()
    if (!code) return

    setError('')
    try {
      let product = productByBarcode.get(code)
      if (!product && navigator.onLine) {
        const fetched = await apiJson<Product>(`/api/v1/products/barcode/${encodeURIComponent(code)}`)
        product = fetched
        setProducts((prev) => (prev.some((item) => item.id === fetched.id) ? prev : [...prev, fetched]))
      }
      if (!product) {
        setError(
          navigator.onLine
            ? `Nenhum produto encontrado para o código ${code}.`
            : `Código ${code} não está no cache local. Conecte-se para consultar o catálogo completo.`,
        )
        return
      }
      if (!product.active) {
        setError(`O produto ${product.name} está inativo.`)
        return
      }

      addProductToCart(product, 1)
      setBarcodeScan('')
    } catch (e: unknown) {
      setError(errorMessage(e))
    } finally {
      barcodeInputRef.current?.focus()
    }
  }

  function updateItemQty(idx: number, qty: number) {
    const normalized = Math.round(Math.max(0, Number(qty) || 0) * 1000) / 1000
    if (normalized <= 0) {
      removeItem(idx)
      return
    }
    setItems((prev) => prev.map((item, i) => (i === idx ? { ...item, qty: normalized } : item)))
  }

  function removeItem(idx: number) {
    setItems((prev) => prev.filter((_, i) => i !== idx))
  }

  function refreshSuspended() {
    setSuspendedCarts(getSuspendedCarts())
  }

  function suspendCurrentCart() {
    if (items.length === 0) return
    try {
      suspendCart({
        items,
        payMethod,
        saleDiscount: canDiscount ? saleDiscount : 0,
      })
      setItems([])
      setSaleDiscount(0)
      refreshSuspended()
      barcodeInputRef.current?.focus()
    } catch (e: unknown) {
      setError(errorMessage(e))
    }
  }

  function resumeSuspended(cart: SuspendedCart) {
    if (items.length > 0 && !window.confirm('Substituir o carrinho atual pela venda suspensa?')) return
    setItems(cart.items)
    setPayMethod(cart.payMethod)
    setSaleDiscount(canDiscount ? cart.saleDiscount : 0)
    removeSuspendedCart(cart.id)
    refreshSuspended()
    barcodeInputRef.current?.focus()
  }

  function discardSuspended(id: string) {
    if (!window.confirm('Descartar esta venda suspensa?')) return
    removeSuspendedCart(id)
    refreshSuspended()
  }

  const canFinalize = useMemo(
    () => cashSessionId && items.length > 0 && computedTotal > 0,
    [cashSessionId, items.length, computedTotal],
  )

  async function finalizeSale() {
    if (!canFinalize || finalizeInFlight.current) return

    finalizeInFlight.current = true
    setFinalizing(true)
    setError('')
    setSaleId('')
    setSaleTotal(0)

    const payments: SalePayment[] = [{ method: payMethod, amount: computedTotal }]
    const body = {
      cash_session_id: cashSessionId,
      customer_id: null,
      discount_value: canDiscount ? saleDiscount : 0,
      items: items.map(({ product_id, qty, discount_value }) => ({
        product_id,
        qty,
        discount_value,
      })),
      payments,
    }
    const idempotencyKey = crypto.randomUUID()

    let queuedId = ''
    try {
      // Write-ahead: persist the exact intent before the first network send.
      // If storage is unavailable, abort without creating an ambiguous sale.
      queuedId = enqueueRequest({
        method: 'POST',
        path: '/api/v1/sales',
        body,
        headers: { 'Idempotency-Key': idempotencyKey },
      })
      refreshPending()
    } catch (e: unknown) {
      setError(
        `Não foi possível preservar a intenção de venda no navegador. Nenhuma venda foi enviada. ${errorMessage(e)}`,
      )
      finalizeInFlight.current = false
      setFinalizing(false)
      return
    }

    try {
      if (!navigator.onLine) {
        setSaleId(`offline:${queuedId}`)
        setSaleTotal(computedTotal)
        setItems([])
        setSaleDiscount(0)
        return
      }

      const res = await apiJson<SaleCreateResponse>('/api/v1/sales', {
        method: 'POST',
        headers: { 'Idempotency-Key': idempotencyKey },
        body,
      })

      discardQueueItem(queuedId)
      refreshPending()
      setSaleId(res.id)
      setSaleTotal(res.total)
      setItems([])
      setSaleDiscount(0)
      void loadProducts()
    } catch (e: unknown) {
      const msg = errorMessage(e)
      const permanent =
        e instanceof APIError &&
        e.status >= 400 &&
        e.status < 500 &&
        ![401, 408, 425, 429].includes(e.status)

      if (permanent) {
        markQueueItemAttention(queuedId, 'request_rejected', msg)
        refreshPending()
        setError(msg)
        return
      }

      // Network, auth retry exhaustion, rate limiting and server failures keep
      // the already-persisted intent pending with the same key for safe replay.
      setSaleId(`offline:${queuedId}`)
      setSaleTotal(computedTotal)
      setItems([])
      setSaleDiscount(0)
      refreshPending()
      setError(`Venda preservada para reenvio seguro: ${msg}`)
    } finally {
      finalizeInFlight.current = false
      setFinalizing(false)
      barcodeInputRef.current?.focus()
    }
  }

  useEffect(() => {
    function onShortcut(event: KeyboardEvent) {
      if (event.key === 'F2') {
        event.preventDefault()
        barcodeInputRef.current?.focus()
      } else if (event.key === 'F4') {
        event.preventDefault()
        productSearchRef.current?.focus()
      } else if (event.key === 'F8') {
        event.preventDefault()
        document.getElementById('pdv-finalize')?.click()
      }
    }
    window.addEventListener('keydown', onShortcut)
    return () => window.removeEventListener('keydown', onShortcut)
  }, [])

  return (
    <div>
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">PDV</h2>
          <p className="text-sm text-gray-600">Abrir caixa e registrar venda finalizada.</p>
          <p className="mt-1 text-xs text-gray-600">
            Status: {online ? 'online' : 'offline'} • Pendências: {pendingSync} • Atenção: {attentionSync}
          </p>
          <p className="mt-1 text-xs text-gray-500">
            Atalhos: F2 scanner • F4 busca rápida • F8 finalizar venda
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
            <button
              disabled={Boolean(cashSessionId)}
              className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
            >
              Abrir
            </button>
          </form>

          <div className="flex flex-1 items-end justify-between gap-2">
            <div className="text-sm">
              <div className="text-xs text-gray-600">cash_session_id</div>
              <div className="font-mono text-xs">{cashSessionId || '—'}</div>
            </div>
            {cashSessionId ? (
              <div className="flex flex-col gap-2">
                <div className="flex items-end gap-2">
                  <label className="block">
                    <span className="text-xs text-gray-600">Movimento (R$)</span>
                    <input
                      value={String(movementAmount)}
                      onChange={(e) => setMovementAmount(Number(e.target.value))}
                      type="number"
                      min="0"
                      step="0.01"
                      className="mt-1 w-32 rounded-md border px-3 py-2 text-sm"
                    />
                  </label>
                  <button
                    type="button"
                    onClick={() => void recordCashMovement('supply')}
                    className="rounded-md border px-3 py-2 text-xs hover:bg-gray-50"
                  >
                    Suprimento
                  </button>
                  <button
                    type="button"
                    onClick={() => void recordCashMovement('withdrawal')}
                    className="rounded-md border px-3 py-2 text-xs hover:bg-gray-50"
                  >
                    Sangria
                  </button>
                </div>
                <div className="grid grid-cols-2 gap-2 md:grid-cols-3">
                  <label className="block">
                    <span className="text-xs text-gray-600">Dinheiro declarado</span>
                    <input
                      value={String(closingAmount)}
                      onChange={(e) => setClosingAmount(Number(e.target.value))}
                      type="number"
                      min="0"
                      step="0.01"
                      className="mt-1 w-full rounded-md border px-2 py-2 text-sm"
                    />
                  </label>
                  {CLOSE_METHODS.map(([method, label]) => (
                    <label key={method} className="block">
                      <span className="text-xs text-gray-600">{label} declarado</span>
                      <input
                        value={String(closingByMethod[method] ?? 0)}
                        onChange={(e) =>
                          setClosingByMethod((prev) => ({
                            ...prev,
                            [method]: Number(e.target.value) || 0,
                          }))
                        }
                        type="number"
                        min="0"
                        step="0.01"
                        className="mt-1 w-full rounded-md border px-2 py-2 text-sm"
                      />
                    </label>
                  ))}
                </div>
                <button
                  type="button"
                  onClick={() => void closeCash()}
                  className="rounded-md border px-3 py-2 text-sm hover:bg-gray-50"
                >
                  Fechar caixa
                </button>
              </div>
            ) : null}
          </div>
        </div>
      </div>

      {cashCloseSummary ? (
        <div className="mt-3 rounded-md border bg-gray-50 p-3 text-sm">
          <div className="font-semibold">Conciliação do último fechamento</div>
          <div className="mt-1 text-xs text-gray-700">
            Dinheiro — esperado R$ {cashCloseSummary.expected_cash.toFixed(2)} • declarado R 
            {cashCloseSummary.closing_amount.toFixed(2)} • diferença R 
            {cashCloseSummary.closing_difference.toFixed(2)}
          </div>
          <div className="mt-2 grid grid-cols-1 gap-1 text-xs text-gray-700 md:grid-cols-2">
            {CLOSE_METHODS.map(([method, label]) => (
              <div key={method}>
                {label}: esperado R$ {(cashCloseSummary.expected_by_method[method] ?? 0).toFixed(2)}
                {' • '}declarado R 
                {(cashCloseSummary.declared_by_method[method] ?? 0).toFixed(2)}
                {' • '}diferença R 
                {(cashCloseSummary.difference_by_method[method] ?? 0).toFixed(2)}
              </div>
            ))}
          </div>
        </div>
      ) : null}

      {legacyQueueCount > 0 ? (
        <div className="mt-4 rounded-md border border-amber-300 bg-amber-50 p-3">
          <h3 className="text-sm font-semibold text-amber-900">Fila offline legada detectada</h3>
          <p className="mt-1 text-xs text-amber-800">
            {legacyQueueCount} item(ns) da versão anterior ainda existem neste navegador. Eles não
            serão enviados automaticamente porque não possuem escopo tenant/usuário confiável.
          </p>
          <div className="mt-2 flex gap-2">
            <button
              type="button"
              onClick={importLegacyQueue}
              className="rounded-md border border-amber-400 px-3 py-2 text-xs"
            >
              Importar para revisão
            </button>
            <button
              type="button"
              onClick={removeLegacyQueue}
              className="rounded-md border border-red-300 px-3 py-2 text-xs text-red-700"
            >
              Descartar legado
            </button>
          </div>
        </div>
      ) : null}

      {queueItems.some((item) => item.state === 'attention') ? (
        <div className="mt-4 rounded-md border p-3">
          <h3 className="text-sm font-semibold">Reconciliação offline</h3>
          <div className="mt-2 space-y-2">
            {queueItems
              .filter((item) => item.state === 'attention')
              .map((item) => (
                <div key={item.id} className="rounded-md border p-2 text-xs">
                  <div className="font-mono">{item.id}</div>
                  <div className="mt-1 text-gray-700">
                    Motivo: {item.attentionReason ?? 'revisao'} •{' '}
                    {item.lastError ?? 'sem detalhe'}
                  </div>
                  <div className="mt-2 flex gap-2">
                    <button
                      type="button"
                      onClick={() => void retryAttention(item.id)}
                      className="rounded-md border px-2 py-1"
                    >
                      Tentar novamente
                    </button>
                    {cashSessionId && item.path.includes('/api/v1/sales') ? (
                      <button
                        type="button"
                        onClick={() => void rebindAttentionToCurrentCash(item.id)}
                        className="rounded-md border px-2 py-1"
                      >
                        Usar caixa atual
                      </button>
                    ) : null}
                    <button
                      type="button"
                      onClick={() => discardAttention(item.id)}
                      className="rounded-md border border-red-300 px-2 py-1 text-red-700"
                    >
                      Descartar
                    </button>
                  </div>
                </div>
              ))}
          </div>
        </div>
      ) : null}

      {suspendedCarts.length > 0 ? (
        <div className="mt-4 rounded-md border p-3">
          <h3 className="text-sm font-semibold">Vendas suspensas</h3>
          <p className="mt-1 text-xs text-gray-600">
            Carrinhos locais deste operador/tenant. Nenhuma venda foi enviada ao servidor.
          </p>
          <div className="mt-2 space-y-2">
            {suspendedCarts.map((cart) => (
              <div key={cart.id} className="flex flex-col gap-2 rounded-md border p-2 text-xs md:flex-row md:items-center md:justify-between">
                <div>
                  <div>
                    {new Date(cart.createdAt).toLocaleString()} • {cart.items.length} item(ns) •{' '}
                    {cart.payMethod.toUpperCase()}
                  </div>
                  {cart.saleDiscount > 0 ? (
                    <div className="text-gray-600">Desconto salvo: R$ {cart.saleDiscount.toFixed(2)}</div>
                  ) : null}
                </div>
                <div className="flex gap-2">
                  <button
                    type="button"
                    onClick={() => resumeSuspended(cart)}
                    className="rounded-md border px-2 py-1"
                  >
                    Retomar
                  </button>
                  <button
                    type="button"
                    onClick={() => discardSuspended(cart.id)}
                    className="rounded-md border border-red-300 px-2 py-1 text-red-700"
                  >
                    Descartar
                  </button>
                </div>
              </div>
            ))}
          </div>
        </div>
      ) : null}

      <div className="mt-4 rounded-md border p-3">
        <h3 className="text-sm font-semibold">Itens</h3>

        <form onSubmit={scanBarcode} className="mt-2 flex gap-2">
          <label className="block flex-1">
            <span className="text-xs text-gray-600">Código de barras</span>
            <input
              ref={barcodeInputRef}
              value={barcodeScan}
              onChange={(e) => setBarcodeScan(e.target.value)}
              placeholder="Bipe o produto e pressione Enter"
              autoComplete="off"
              autoFocus
              className="mt-1 w-full rounded-md border px-3 py-2 font-mono text-sm"
            />
          </label>
          <button
            type="submit"
            className="mt-5 rounded-md bg-gray-900 px-4 py-2 text-sm font-medium text-white"
          >
            Ler código
          </button>
        </form>
        <p className="mt-1 text-xs text-gray-500">
          Leitores USB/Bluetooth que funcionam como teclado podem enviar o código seguido de Enter.
        </p>

        <div className="mt-3 grid grid-cols-1 gap-2 md:grid-cols-6">
          <label className="block md:col-span-6">
            <span className="text-xs text-gray-600">Busca rápida por nome, SKU ou código</span>
            <input
              ref={productSearchRef}
              value={productQuery}
              onChange={(e) => setProductQuery(e.target.value)}
              placeholder="Digite para filtrar produtos"
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            />
          </label>
          <label className="block md:col-span-4">
            <span className="text-xs text-gray-600">Produto</span>
            <select
              value={itemProductId}
              onChange={(e) => setItemProductId(e.target.value)}
              className="mt-1 w-full rounded-md border px-3 py-2 text-sm"
            >
              <option value="">Selecione…</option>
              {filteredProducts.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.sku} — {p.name} • estoque {p.qty_on_hand ?? '—'}
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
                    <td className="px-3 py-2">
                      <div className="flex items-center gap-1">
                        <button
                          type="button"
                          onClick={() => updateItemQty(idx, it.qty - 1)}
                          className="rounded border px-2 py-1 text-xs"
                          aria-label={`Diminuir quantidade de ${name}`}
                        >
                          −
                        </button>
                        <input
                          type="number"
                          min="0.001"
                          step="0.001"
                          value={String(it.qty)}
                          onChange={(e) => updateItemQty(idx, Number(e.target.value))}
                          className="w-20 rounded border px-2 py-1 text-center text-xs"
                          aria-label={`Quantidade de ${name}`}
                        />
                        <button
                          type="button"
                          onClick={() => updateItemQty(idx, it.qty + 1)}
                          className="rounded border px-2 py-1 text-xs"
                          aria-label={`Aumentar quantidade de ${name}`}
                        >
                          +
                        </button>
                      </div>
                    </td>
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
          <div className="flex items-end gap-3">
            <div className="text-sm">
              <div className="text-xs text-gray-600">Total</div>
              <div className="text-lg font-semibold">R$ {computedTotal.toFixed(2)}</div>
            </div>
            {canDiscount ? (
              <label className="block">
                <span className="text-xs text-gray-600">Desconto da venda (R$)</span>
                <input
                  type="number"
                  min="0"
                  step="0.01"
                  value={String(saleDiscount)}
                  onChange={(e) => setSaleDiscount(Math.max(0, Number(e.target.value) || 0))}
                  className="mt-1 w-32 rounded-md border px-2 py-2 text-sm"
                />
              </label>
            ) : (
              <span className="text-xs text-gray-500">Desconto requer permissão.</span>
            )}
          </div>

          <div className="flex items-end gap-2">
            <button
              type="button"
              onClick={suspendCurrentCart}
              disabled={items.length === 0}
              className="rounded-md border px-3 py-2 text-sm disabled:opacity-50"
            >
              Suspender
            </button>
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
              id="pdv-finalize"
              type="button"
              onClick={() => void finalizeSale()}
              disabled={!canFinalize || finalizing}
              className="rounded-md bg-gray-900 px-3 py-2 text-sm font-medium text-white disabled:opacity-60"
            >
              {finalizing ? 'Finalizando…' : 'Finalizar'}
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
                <button
                  type="button"
                  onClick={() => window.print()}
                  className="ml-3 rounded border border-green-300 px-2 py-1 text-xs"
                >
                  Imprimir comprovante não fiscal
                </button>
              </>
            )}
          </div>
        ) : null}
      </div>
    </div>
  )
}
