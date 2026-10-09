import { useEffect, useMemo, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { APIError, apiJson, errorMessage } from '../lib/api'
import { useProductThumbnails } from '../lib/productThumbnails'
import {
  claimLegacyQueue,
  discardLegacyQueue,
  discardQueueItem,
  enqueueRequest,
  flushQueue,
  getLegacyQueueCount,
  getQueueItems,
  getQueueSummary,
  getQueueSummaryForCashSession,
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
  promo_price?: number | null
  qty_on_hand?: number
  active: boolean
}

type MeResponse = {
  permissions?: string[]
}

type ProductsListResponse = { items: Product[]; total: number }
type ProductCache = { savedAt: number; items: Product[] }

type CashMovementAttempt = {
  sessionId: string
  movementType: 'supply' | 'withdrawal'
  amount: number
  fingerprint: string
  key: string
}

type CashOpenResponse = { id: string }
type CurrentCashSessionResponse = {
  id: string
  cash_register_id: string
  opened_by_user_id: string
  status: string
  opening_amount: number
}
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

type ReceiptSnapshot = {
  saleId: string
  total: number
  saleDiscount: number
  paymentMethod: string
  createdAt: string
  items: Array<{
    label: string
    qty: number
    unitPrice: number
    discountValue: number
    lineTotal: number
  }>
}

const CASH_MOVEMENT_ATTEMPT_NAMESPACE = 'sistemaemgo:cashMovementAttempt:v1'
const PRODUCTS_CACHE_NAMESPACE = 'sistemaemgo:productsCache:v2'
const PRODUCTS_CACHE_MAX_AGE_MS = 24 * 60 * 60 * 1000
const CLOSE_METHODS = [
  ['pix', 'PIX'],
  ['debit', 'Débito'],
  ['credit', 'Crédito'],
  ['transfer', 'Transferência'],
  ['voucher', 'Voucher'],
] as const

const PAYMENT_LABELS: Record<string, string> = {
  cash: 'Dinheiro',
  pix: 'PIX',
  debit: 'Débito',
  credit: 'Crédito',
  transfer: 'Transferência',
  voucher: 'Voucher',
}

function loadCashMovementAttempt(): CashMovementAttempt | null {
  const key = scopedStorageKey(CASH_MOVEMENT_ATTEMPT_NAMESPACE)
  if (!key) return null
  const raw = localStorage.getItem(key)
  if (!raw) return null
  try {
    const value = JSON.parse(raw) as CashMovementAttempt
    if (
      !value ||
      typeof value.sessionId !== 'string' ||
      (value.movementType !== 'supply' && value.movementType !== 'withdrawal') ||
      !Number.isFinite(value.amount) ||
      value.amount <= 0 ||
      typeof value.fingerprint !== 'string' ||
      typeof value.key !== 'string' ||
      value.key.trim() === ''
    ) {
      return null
    }
    return value
  } catch {
    return null
  }
}

function persistCashMovementAttempt(attempt: CashMovementAttempt): void {
  const key = scopedStorageKey(CASH_MOVEMENT_ATTEMPT_NAMESPACE)
  if (!key) throw new Error('authenticated tenant/user scope is required')
  localStorage.setItem(key, JSON.stringify(attempt))
}

function clearPersistedCashMovementAttempt(): void {
  const key = scopedStorageKey(CASH_MOVEMENT_ATTEMPT_NAMESPACE)
  if (key) localStorage.removeItem(key)
}

function productSalePrice(product: Product): number {
  const promo = Number(product.promo_price)
  if (Number.isFinite(promo) && promo > 0) return promo
  return Number(product.price_cash) || 0
}

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
  const [receipt, setReceipt] = useState<ReceiptSnapshot | null>(null)
  const [finalizing, setFinalizing] = useState(false)
  const finalizeInFlight = useRef(false)
  const cashMovementAttempt = useRef<CashMovementAttempt | null>(loadCashMovementAttempt())

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

  const productThumbnails = useProductThumbnails(products.map((product) => product.id))

  const filteredProducts = useMemo(() => {
    const q = productQuery.trim().toLowerCase()
    if (!q) return products
    return products.filter((p) =>
      [p.sku, p.name, p.barcode ?? ''].some((value) => value.toLowerCase().includes(q)),
    )
  }, [products, productQuery])

  useEffect(() => {
    const search = productQuery.trim()
    if (search.length < 2 || !online) return
    let cancelled = false
    const timer = window.setTimeout(async () => {
      try {
        const result = await apiJson<ProductsListResponse>(
          `/api/v1/products?query=${encodeURIComponent(search)}&limit=200&offset=0`,
        )
        if (cancelled) return
        // Merge search results instead of dropping cart product metadata.
        setProducts((previous) => {
          const combined = new Map(previous.map((item) => [item.id, item]))
          for (const product of result.items ?? []) {
            if (product.active) combined.set(product.id, product)
          }
          return Array.from(combined.values())
        })
      } catch {
        // Keep cached products available on intermittent connectivity errors.
      }
    }, 300)
    return () => { cancelled = true; window.clearTimeout(timer) }
  }, [productQuery, online])

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
      if (cacheKey) {
        const cache: ProductCache = { savedAt: Date.now(), items: active }
        localStorage.setItem(cacheKey, JSON.stringify(cache))
      }
    } catch (e: unknown) {
      const canUseOfflineCache =
        !(e instanceof APIError) ||
        e.status >= 500 ||
        [408, 425, 429].includes(e.status)

      if (!canUseOfflineCache) {
        setProducts([])
        setError(errorMessage(e))
        return
      }

      const cacheKey = scopedStorageKey(PRODUCTS_CACHE_NAMESPACE)
      const cachedRaw = cacheKey ? localStorage.getItem(cacheKey) : null
      if (cachedRaw) {
        try {
          const parsed = JSON.parse(cachedRaw) as ProductCache | Product[]
          if (
            !Array.isArray(parsed) &&
            Number.isFinite(parsed.savedAt) &&
            Array.isArray(parsed.items) &&
            parsed.items.length > 0 &&
            Date.now() - parsed.savedAt <= PRODUCTS_CACHE_MAX_AGE_MS
          ) {
            setProducts(parsed.items.filter((product) => product.active))
            setError('Catálogo carregado do cache local porque o servidor está indisponível.')
            return
          }
          setError('O catálogo offline está ausente ou expirado. Conecte-se antes de registrar novas vendas.')
          return
        } catch {
          setError('O catálogo offline local está inválido. Conecte-se para atualizá-lo.')
          return
        }
      }
      setError(errorMessage(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadProducts()
    void reconcileCurrentCashSession().catch(() => {
      // Keep the local session hint on transient connectivity failures.
    })
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

  async function reconcileCurrentCashSession(): Promise<boolean> {
    try {
      const current = await apiJson<CurrentCashSessionResponse>('/api/v1/cash/sessions/current')
      setCashSessionId(current.id)
      setCashSessionIdState(current.id)
      return true
    } catch (e: unknown) {
      if (e instanceof APIError && e.status === 404) {
        clearCashSessionId()
        setCashSessionIdState('')
        return false
      }
      throw e
    }
  }

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
      if (e instanceof APIError && e.status === 409) {
        try {
          if (await reconcileCurrentCashSession()) {
            setError('Sessão de caixa já estava aberta e foi recuperada.')
            return
          }
        } catch {
          // Fall through to the original API error.
        }
      }
      setError(errorMessage(e))
    }
  }

  async function recordCashMovement(movementType: 'supply' | 'withdrawal') {
    if (!cashSessionId || movementAmount <= 0) return
    setError('')

    const amount = Number(movementAmount) || 0
    const fingerprint = `${cashSessionId}|${movementType}|${amount.toFixed(2)}`
    let attempt = cashMovementAttempt.current ?? loadCashMovementAttempt()

    if (attempt && attempt.fingerprint !== fingerprint) {
      setError(
        `Existe uma movimentação anterior com resposta pendente: ${attempt.movementType === 'supply' ? 'suprimento' : 'sangria'} de R$ ${attempt.amount.toFixed(2)}. Repita essa operação primeiro para reconciliar o resultado.`,
      )
      return
    }

    if (!attempt) {
      attempt = {
        sessionId: cashSessionId,
        movementType,
        amount,
        fingerprint,
        key: crypto.randomUUID(),
      }
      try {
        persistCashMovementAttempt(attempt)
      } catch (e: unknown) {
        setError(`Não foi possível preservar a tentativa de caixa antes do envio: ${errorMessage(e)}`)
        return
      }
      cashMovementAttempt.current = attempt
    }

    try {
      await apiJson(`/api/v1/cash/sessions/${cashSessionId}/movements`, {
        method: 'POST',
        headers: { 'Idempotency-Key': attempt.key },
        body: {
          movement_type: movementType,
          amount,
          notes: null,
        },
      })
      cashMovementAttempt.current = null
      clearPersistedCashMovementAttempt()
      setMovementAmount(0)
    } catch (e: unknown) {
      cashMovementAttempt.current = attempt
      setError(errorMessage(e))
    }
  }

  async function closeCash() {
    if (!cashSessionId) return
    setError('')

    const legacyPending = getLegacyQueueCount()
    if (legacyPending > 0) {
      setLegacyQueueCount(legacyPending)
      setError(
        `Não é possível fechar o caixa: existem ${legacyPending} item(ns) na fila offline legada ainda não revisada. Importe a fila para atenção ou descarte-a explicitamente antes do fechamento.`,
      )
      return
    }

    const unresolvedMovement = cashMovementAttempt.current ?? loadCashMovementAttempt()
    if (unresolvedMovement?.sessionId === cashSessionId) {
      cashMovementAttempt.current = unresolvedMovement
      setError(
        'Não é possível fechar o caixa enquanto uma sangria/suprimento tiver resposta pendente. Repita a mesma movimentação para reconciliar o resultado.',
      )
      return
    }

    const queuedForCash = getQueueSummaryForCashSession(cashSessionId)
    if (queuedForCash.total > 0) {
      refreshPending()
      setError(
        `Não é possível fechar o caixa: existem ${queuedForCash.total} venda(s) offline deste caixa (${queuedForCash.pending} pendente(s), ${queuedForCash.attention} em atenção). Sincronize ou reconcilie essas vendas primeiro.`,
      )
      return
    }

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
      cashMovementAttempt.current = null
      clearPersistedCashMovementAttempt()
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
      if (e instanceof APIError && e.status === 409) {
        try {
          const stillOpen = await reconcileCurrentCashSession()
          if (!stillOpen) {
            setError('A sessão de caixa já estava fechada e o estado local foi reconciliado.')
            return
          }
        } catch {
          // Fall through to the original API error.
        }
      }
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
    const rawQty = Number(qty)
    if (!Number.isFinite(rawQty) || rawQty <= 0) {
      setError('Informe uma quantidade maior que zero.')
      return
    }
    const normalizedQty = Math.round(rawQty * 1000) / 1000
    if (normalizedQty <= 0) {
      setError('Informe uma quantidade válida com até 3 casas decimais.')
      return
    }
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
          unit_price: productSalePrice(p),
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
      setProducts((prev) => (prev.some((item) => item.id === product!.id) ? prev : [...prev, product!]))
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

  async function resumeSuspended(cart: SuspendedCart) {
    if (items.length > 0 && !window.confirm('Substituir o carrinho atual pela venda suspensa?')) return

    setError('')
    let catalog = products
    if (navigator.onLine) {
      try {
        const data = await apiJson<ProductsListResponse>('/api/v1/products?limit=200&offset=0')
        catalog = data.items.filter((product) => product.active)
        setProducts(catalog)
        const cacheKey = scopedStorageKey(PRODUCTS_CACHE_NAMESPACE)
        if (cacheKey) {
          const cache: ProductCache = { savedAt: Date.now(), items: catalog }
          localStorage.setItem(cacheKey, JSON.stringify(cache))
        }
      } catch (e: unknown) {
        setError(`Não foi possível atualizar preços antes de retomar a venda: ${errorMessage(e)}`)
        return
      }
    }

    const currentById = new Map(catalog.map((product) => [product.id, product]))
    let priceChanged = false
    const refreshedItems: SaleItem[] = []
    for (const item of cart.items) {
      const product = currentById.get(item.product_id)
      if (!product?.active) {
        setError('A venda suspensa contém produto indisponível ou inativo. Revise o catálogo antes de retomar.')
        return
      }
      const currentPrice = productSalePrice(product)
      if (Math.abs(currentPrice - item.unit_price) > 0.0001) priceChanged = true
      refreshedItems.push({ ...item, unit_price: currentPrice })
    }

    try {
      removeSuspendedCart(cart.id)
    } catch (e: unknown) {
      setError(`Não foi possível retirar a venda da lista de suspensas: ${errorMessage(e)}`)
      return
    }

    setItems(refreshedItems)
    setPayMethod(cart.payMethod)
    setSaleDiscount(canDiscount ? cart.saleDiscount : 0)
    refreshSuspended()
    if (priceChanged) {
      setError('A venda suspensa foi retomada com os preços atuais do catálogo.')
    }
    barcodeInputRef.current?.focus()
  }

  function discardSuspended(id: string) {
    if (!window.confirm('Descartar esta venda suspensa?')) return
    try {
      removeSuspendedCart(id)
      refreshSuspended()
    } catch (e: unknown) {
      setError(`Não foi possível descartar a venda suspensa: ${errorMessage(e)}`)
    }
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
    setReceipt(null)

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
      const receiptItems = items.map((item) => {
        const product = productById.get(item.product_id)
        const unitPrice = Number(item.unit_price) || 0
        const qty = Number(item.qty) || 0
        const discountValue = Number(item.discount_value) || 0
        return {
          label: product ? `${product.sku} — ${product.name}` : item.product_id,
          qty,
          unitPrice,
          discountValue,
          lineTotal: Math.round((unitPrice * qty - discountValue) * 100) / 100,
        }
      })
      setReceipt({
        saleId: res.id,
        total: res.total,
        saleDiscount: canDiscount ? saleDiscount : 0,
        paymentMethod: payMethod,
        createdAt: new Date().toISOString(),
        items: receiptItems,
      })
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

  function printNonFiscalReceipt() {
    if (!receipt) return

    const popup = window.open('', '_blank', 'width=420,height=640')
    if (!popup) {
      setError('O navegador bloqueou a janela do comprovante. Libere pop-ups e tente novamente.')
      return
    }
    popup.opener = null
    const doc = popup.document
    doc.title = 'Comprovante não fiscal'

    const style = doc.createElement('style')
    style.textContent =
      'body{font-family:ui-monospace,monospace;max-width:380px;margin:20px auto;padding:0 12px;color:#111}' +
      'h1{font-size:18px;text-align:center;margin:0 0 4px}' +
      '.warn{text-align:center;font-weight:700;border:2px solid #111;padding:8px;margin:8px 0}' +
      '.muted{font-size:11px;color:#444}.row{display:flex;justify-content:space-between;gap:12px}' +
      '.item{border-top:1px dashed #777;padding:6px 0}.total{font-size:18px;font-weight:700;border-top:2px solid #111;padding-top:8px;margin-top:8px}' +
      '@media print{body{margin:0 auto}.no-print{display:none}}'
    doc.head.appendChild(style)

    const addText = (tag: 'h1' | 'div' | 'p', text: string, className?: string) => {
      const node = doc.createElement(tag)
      node.textContent = text
      if (className) node.className = className
      doc.body.appendChild(node)
      return node
    }

    addText('h1', 'SistemaEmGo')
    addText('div', 'COMPROVANTE NÃO FISCAL', 'warn')
    addText('p', 'Não é documento fiscal e não substitui NFC-e/NF-e.', 'muted')
    addText('p', `Venda: ${receipt.saleId}`, 'muted')
    addText('p', `Data/hora do terminal: ${new Date(receipt.createdAt).toLocaleString('pt-BR')}`, 'muted')

    for (const item of receipt.items) {
      const box = doc.createElement('div')
      box.className = 'item'
      const label = doc.createElement('div')
      label.textContent = item.label
      box.appendChild(label)
      const detail = doc.createElement('div')
      detail.className = 'row muted'
      const left = doc.createElement('span')
      left.textContent = `${item.qty.toFixed(3)} x R$ ${item.unitPrice.toFixed(2)}`
      const right = doc.createElement('span')
      right.textContent = `R$ ${item.lineTotal.toFixed(2)}`
      detail.append(left, right)
      box.appendChild(detail)
      if (item.discountValue > 0) {
        const discount = doc.createElement('div')
        discount.className = 'muted'
        discount.textContent = `Desconto do item: R$ ${item.discountValue.toFixed(2)}`
        box.appendChild(discount)
      }
      doc.body.appendChild(box)
    }

    if (receipt.saleDiscount > 0) {
      addText('p', `Desconto da venda: R$ ${receipt.saleDiscount.toFixed(2)}`, 'muted')
    }
    addText('p', `Pagamento: ${PAYMENT_LABELS[receipt.paymentMethod] ?? receipt.paymentMethod}`, 'muted')
    addText('div', `TOTAL R$ ${receipt.total.toFixed(2)}`, 'total')
    addText('p', 'Guarde apenas como comprovante comercial interno/ao cliente.', 'muted')

    popup.focus()
    popup.print()
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
    <div className="space-y-6">
      <header className="flex flex-col justify-between gap-4 sm:flex-row sm:items-start">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">Frente de caixa</p>
          <h2 className="mt-1 text-2xl font-bold tracking-tight text-slate-900">PDV</h2>
          <p className="mt-1 max-w-xl text-sm text-slate-600">
            Prepare a venda, confira o valor e só então finalize o pagamento.
          </p>
          <p className="mt-2 text-sm font-medium text-slate-700">
            {cashSessionId ? 'Caixa pronto para atender.' : 'Abra o caixa para começar.'}
          </p>
        </div>
      </header>

      {(!online || pendingSync > 0 || attentionSync > 0) ? (
        <div role="status" className="rounded-xl border border-amber-300 bg-amber-50 px-4 py-3 text-sm text-amber-900">
          {!online ? 'Sem internet. Vendas pendentes precisam ser sincronizadas. ' : ''}
          {pendingSync > 0 ? `${pendingSync} venda(s) aguardando sincronização. ` : ''}
          {attentionSync > 0 ? `${attentionSync} operação(ões) precisam de revisão.` : ''}
        </div>
      ) : null}

      {error ? (
        <div role="alert" className="rounded-xl border border-red-200 bg-red-50 p-4 text-sm text-red-800">
          {error}
        </div>
      ) : null}

      <section aria-labelledby="pdv-cash-title" className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm sm:p-5">
        <div className="mb-2">
          <h3 id="pdv-cash-title" className="text-base font-bold text-slate-900">{cashSessionId ? "Caixa aberto" : "Abrir caixa"}</h3>
          <p className="mt-1 text-sm text-slate-600">
            {cashSessionId ? 'O caixa está aberto. Você já pode registrar vendas.' : 'Informe o valor inicial em dinheiro para começar o turno.'}
          </p>
        </div>
        {cashSessionId ? (
          <div className="space-y-4">
            <details className="rounded-xl bg-slate-50 p-4">
              <summary className="cursor-pointer text-sm font-semibold text-slate-700">Movimentar dinheiro (entrada ou retirada)</summary>
              <div className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-end">
              <label className="block max-w-xs flex-1 text-sm font-medium text-slate-700">
                <span>Movimento (R$)</span>
                <input
                  value={String(movementAmount)}
                  onChange={(e) => setMovementAmount(Number(e.target.value))}
                  type="number"
                  min="0"
                  step="0.01"
                  className="mt-2 w-full rounded-lg border border-slate-300 px-3 py-2.5 text-sm"
                />
              </label>
              <div className="flex flex-wrap gap-2">
                <button type="button" onClick={() => void recordCashMovement('supply')}
                  className="rounded-lg border border-slate-300 px-4 py-2.5 text-sm font-medium hover:bg-slate-50">
                  Suprimento
                </button>
                <button type="button" onClick={() => void recordCashMovement('withdrawal')}
                  className="rounded-lg border border-slate-300 px-4 py-2.5 text-sm font-medium hover:bg-slate-50">
                  Sangria
                </button>
              </div>
            </div>
              <p className="mt-3 text-xs text-slate-500">Suprimento: entrada de dinheiro no caixa. Sangria: retirada de dinheiro.</p>
            </details>
          </div>
        ) : (
          <form onSubmit={openCash} className="flex flex-col gap-3 sm:flex-row sm:items-end">
            <label className="block w-full max-w-xs text-sm font-medium text-slate-700">
              <span>Abertura (R$)</span>
              <input
                value={String(openingAmount)}
                onChange={(e) => setOpeningAmount(Number(e.target.value))}
                type="number"
                min="0"
                step="0.01"
                className="mt-2 w-full rounded-lg border border-slate-300 px-3 py-2.5 text-sm"
              />
            </label>
            <button type="submit" disabled={Boolean(cashSessionId)}
              className="rounded-lg bg-slate-900 px-6 py-3 text-sm font-semibold text-white hover:bg-slate-700 disabled:opacity-50">
              Abrir
            </button>
          </form>
        )}
      </section>

      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1.1fr)_minmax(380px,1fr)]">
        <div className="min-w-0 space-y-5">
          <section aria-labelledby="pdv-items-title" className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
            <div className="flex items-center gap-3">
              <div>
                <h3 id="pdv-items-title" className="text-lg font-bold text-slate-900">Produtos da venda</h3>
                <p className="text-sm text-slate-500">Leia o código ou busque o produto para montar a venda.</p>
              </div>
            </div>


        <form onSubmit={scanBarcode} className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-end">
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
            className="rounded-lg bg-slate-900 px-5 py-3 text-sm font-semibold text-white hover:bg-slate-700 sm:self-end"
          >
            Ler código
          </button>
        </form>
        <p className="mt-1 text-xs text-gray-500">
          Leitores USB/Bluetooth que funcionam como teclado podem enviar o código seguido de Enter.
        </p>

        <div className="mt-5 grid grid-cols-1 gap-3 sm:grid-cols-6">
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
              className="w-full rounded-lg bg-slate-100 px-4 py-3 text-sm font-semibold text-slate-900 hover:bg-slate-200 sm:mt-5"
            >
              Adicionar
            </button>
          </div>
        </div>

        <div aria-label="Produtos para adicionar" className="mt-5">
          <div className="mb-2 flex items-center justify-between gap-2">
            <h4 className="text-sm font-semibold text-slate-800">{loading ? 'Carregando produtos…' : 'Escolha um produto'}</h4>
            <p className="text-xs text-slate-500">{productQuery ? 'Resultados da busca' : 'Catálogo carregado'} • até 8 opções rápidas</p>
          </div>
          {filteredProducts.length > 0 ? (
            <div className="grid gap-2 sm:grid-cols-2">
              {filteredProducts.slice(0,8).map((product) => (
                <button key={product.id} type="button" onClick={() => addProductToCart(product)}
                  aria-label={`Adicionar ${product.name} à venda`}
                  className="flex min-w-0 items-center justify-between gap-3 rounded-xl border border-slate-200 bg-slate-50 p-3 text-left hover:border-slate-500 hover:bg-white focus-visible:outline-2">
                  {productThumbnails[product.id] ? (
                    <img src={productThumbnails[product.id]} alt="" className="h-12 w-12 shrink-0 rounded-md object-cover" />
                  ) : null}
                  <span className="min-w-0"><strong className="block truncate text-sm text-slate-900">{product.name}</strong>
                    <span className="text-xs text-slate-500">{product.sku} • saldo {product.qty_on_hand ?? '—'}</span></span>
                  <span className="shrink-0 text-sm font-bold text-slate-900">R$ {productSalePrice(product).toFixed(2)} <span aria-hidden="true">+</span></span>
                </button>
              ))}
            </div>
          ) : (
            <p className="rounded-xl bg-slate-50 p-4 text-sm text-slate-600">Nenhum produto encontrado. Revise a busca ou atualize o catálogo.</p>
          )}
        </div>

        <div className="mt-6 overflow-auto rounded-xl border border-slate-200">
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
                  <td className="px-3 py-10 text-center text-sm text-gray-500" colSpan={5}>
                    Nenhum item.
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
          </section>
      {suspendedCarts.length > 0 ? (
        <div className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm">
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
                    onClick={() => void resumeSuspended(cart)}
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
        </div>
        <aside className="min-w-0 space-y-5 xl:sticky xl:top-5">
          <section aria-labelledby="pdv-payment-title" className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
            <div className="mb-6">
              <div>
                <h3 id="pdv-payment-title" className="text-lg font-bold text-slate-900">Pagamento</h3>
                <p className="text-sm text-slate-500">Revise o total e escolha a forma de pagamento.</p>
              </div>
            </div>
        <div className="mt-5 flex flex-col gap-5">
          <div className="flex flex-col gap-5 rounded-xl bg-slate-50 p-4">
            <div className="text-sm">
              <div className="text-xs text-gray-600">Total</div>
              <div className="text-4xl font-bold tracking-tight text-slate-900">R$ {computedTotal.toFixed(2)}</div>
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
                  className="mt-1 w-full rounded-md border px-3 py-3 text-sm"
                />
              </label>
            ) : (
              <span className="text-xs text-gray-500">Desconto requer permissão.</span>
            )}
          </div>

          <div className="flex flex-col gap-4">
            <button
              type="button"
              onClick={suspendCurrentCart}
              disabled={items.length === 0}
              className="rounded-lg border border-slate-300 px-4 py-3 text-sm hover:bg-slate-50 disabled:opacity-50"
            >
              Suspender
            </button>
            <label className="block w-full">
              <span className="text-sm font-semibold text-slate-700">Forma de pagamento</span>
              <select
                value={payMethod}
                onChange={(e) => setPayMethod(e.target.value)}
                className="mt-2 w-full rounded-lg border px-4 py-3 text-base"
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
              className="min-h-12 w-full rounded-lg bg-slate-900 px-5 py-3 text-base font-semibold text-white hover:bg-slate-700 disabled:cursor-not-allowed disabled:opacity-40"
            >
              {finalizing ? 'Finalizando…' : 'Finalizar'}
            </button>
          </div>
        </div>
            <p className="mt-4 rounded-lg bg-slate-50 p-3 text-xs leading-relaxed text-slate-600">
              As formas de pagamento são registradas aqui. Pix e cartões ainda não têm confirmação bancária automática integrada.
            </p>
          </section>
        {saleId ? (
          <div role="status" className={saleId.startsWith('offline:')
            ? 'rounded-xl border border-amber-300 bg-amber-50 p-4 text-sm text-amber-900'
            : 'rounded-xl border border-emerald-200 bg-emerald-50 p-4 text-sm text-emerald-900'}>
            {saleId.startsWith('offline:') ? (
              <>
                Venda registrada offline (pendente sync): <span className="font-mono text-xs">{saleId.replace('offline:', '')}</span> • Total R$ {saleTotal.toFixed(2)}
                <span className="mt-2 block font-semibold">Aguardando confirmação no servidor. Confira as pendências antes de encerrar o caixa.</span>
              </>
            ) : (
              <>
                Venda finalizada: <span className="font-mono text-xs">{saleId}</span> • Total R$ {saleTotal.toFixed(2)}
                {receipt ? (
                  <button
                    type="button"
                    onClick={printNonFiscalReceipt}
                    className="ml-3 rounded border border-green-300 px-2 py-1 text-xs"
                  >
                    Imprimir comprovante não fiscal
                  </button>
                ) : null}
              </>
            )}
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
        </aside>
      </div>

      {cashSessionId ? (
        <section aria-labelledby="pdv-close-title" className="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm sm:p-6">
          <div className="mb-5 border-b border-slate-100 pb-4">
            <div className="flex items-center gap-3">
              <h3 id="pdv-close-title" className="text-base font-bold text-slate-900">Encerrar turno</h3>
            </div>
            <p className="mt-2 text-sm text-slate-600">
              Ao terminar o expediente, confira os valores recebidos antes de fechar o caixa.
              Vendas offline pendentes precisam ser resolvidas primeiro.
            </p>
          </div>
          <div className="rounded-xl border border-slate-200 bg-slate-50 p-4 sm:p-5">
            <h4 className="text-sm font-bold text-slate-900">Conferência dos recebimentos</h4>
            <p className="mt-1 mb-4 text-xs leading-relaxed text-slate-600">
              Conte o dinheiro físico. Para Pix, cartão e outras formas, informe o total registrado nos respectivos comprovantes.
              A conferência aqui não verifica o banco automaticamente.
            </p>
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
              <label className="block text-sm font-medium text-slate-700">
                <span>Dinheiro declarado</span>
              <input type="number" min="0" step="0.01" value={String(closingAmount)}
                onChange={(e) => setClosingAmount(Number(e.target.value))}
                className="mt-2 w-full rounded-lg border border-slate-300 px-3 py-2.5 text-sm" />
            </label>
            {CLOSE_METHODS.map(([method, label]) => (
              <label key={method} className="block text-sm font-medium text-slate-700">
                <span>{label} líquido declarado</span>
                <input
                  value={String(closingByMethod[method] ?? 0)}
                  onChange={(e) => setClosingByMethod((prev) => ({
                    ...prev,
                    [method]: Number(e.target.value) || 0,
                  }))}
                  type="number" min="0" step="0.01"
                  className="mt-2 w-full rounded-lg border border-slate-300 px-3 py-2.5 text-sm"
                />
              </label>
            ))}
            </div>
          </div>
          <div className="mt-5 flex justify-end border-t border-slate-100 pt-5">
            <button type="button" onClick={() => void closeCash()}
              className="rounded-lg border border-slate-400 bg-white px-5 py-3 text-sm font-semibold text-slate-900 hover:bg-slate-100">
              Fechar caixa
            </button>
          </div>
        </section>
      ) : null}

      {cashCloseSummary ? (
        <div className="rounded-2xl border border-slate-200 bg-slate-50 p-5 text-sm">
          <div className="font-semibold">Conciliação do último fechamento</div>
          <div className="mt-1 text-xs text-gray-700">
            Dinheiro — esperado R$ {cashCloseSummary.expected_cash.toFixed(2)} • declarado R$
            {cashCloseSummary.closing_amount.toFixed(2)} • diferença R$
            {cashCloseSummary.closing_difference.toFixed(2)}
          </div>
          <div className="mt-2 grid grid-cols-1 gap-1 text-xs text-gray-700 md:grid-cols-2">
            {CLOSE_METHODS.map(([method, label]) => (
              <div key={method}>
                {label}: esperado R$ {(cashCloseSummary.expected_by_method[method] ?? 0).toFixed(2)}
                {' • '}declarado R$
                {(cashCloseSummary.declared_by_method[method] ?? 0).toFixed(2)}
                {' • '}diferença R$
                {(cashCloseSummary.difference_by_method[method] ?? 0).toFixed(2)}
              </div>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  )
}
