import { APIError, apiJson, errorMessage } from './api'
import {
  clearAllUserScopedStorage,
  scopedStorageKey,
  userScopedStorageKeys,
} from './auth'

export type QueueState = 'pending' | 'attention'
export type AttentionReason =
  | 'expired'
  | 'request_rejected'
  | 'legacy_migration'
  | 'retention_expired'
  | 'retention_unknown'
  | 'price_snapshot_missing'
  | 'price_changed'

export type QueuedRequest = {
  id: string
  createdAt: number
  intentCreatedAt?: number
  method: string
  path: string
  body?: unknown
  headers?: Record<string, string>
  state?: QueueState
  attentionReason?: AttentionReason
  lastError?: string
  lastAttemptAt?: number
}

export type QueueSummary = {
  pending: number
  attention: number
  total: number
}

const LEGACY_QUEUE_KEY = 'sistemaemgo:offlineQueue:v1'
const QUEUE_NAMESPACE = 'sistemaemgo:offlineQueue:v2'
const QUEUE_TTL_MS = 24 * 60 * 60 * 1000
// Server idempotency records are retained for 30 days. Keep the browser
// replay window shorter so clock skew/maintenance cannot delete the server
// key immediately before a manual retry.
const SAFE_MANUAL_REPLAY_MS = 28 * 24 * 60 * 60 * 1000

function queueKey(): string | null {
  return scopedStorageKey(QUEUE_NAMESPACE)
}

function safeJsonParse<T>(raw: string | null): T | null {
  if (!raw) return null
  try {
    return JSON.parse(raw) as T
  } catch {
    return null
  }
}

function isValidQueueItem(x: QueuedRequest): boolean {
  return Boolean(
    x &&
      typeof x.id === 'string' &&
      typeof x.createdAt === 'number' &&
      typeof x.method === 'string' &&
      typeof x.path === 'string',
  )
}

function saleHasPriceSnapshots(item: QueuedRequest): boolean {
  if (!item.path.includes('/api/v1/sales')) return true
  if (!item.body || typeof item.body !== 'object') return false

  const body = item.body as { items?: unknown }
  if (!Array.isArray(body.items) || body.items.length === 0) return false

  return body.items.every((raw) => {
    if (!raw || typeof raw !== 'object') return false
    const unitPrice = (raw as { unit_price?: unknown }).unit_price
    return typeof unitPrice === 'number' && Number.isFinite(unitPrice) && unitPrice > 0
  })
}

function readLegacyQueue(): QueuedRequest[] {
  const parsed = safeJsonParse<QueuedRequest[]>(localStorage.getItem(LEGACY_QUEUE_KEY))
  if (!parsed || !Array.isArray(parsed)) return []
  return parsed.filter(isValidQueueItem)
}

function loadQueue(): QueuedRequest[] {
  const key = queueKey()
  if (!key) return []

  const parsed = safeJsonParse<QueuedRequest[]>(localStorage.getItem(key))
  if (!parsed || !Array.isArray(parsed)) return []

  const now = Date.now()
  let changed = false
  const valid = parsed.filter((x) => {
    const ok = isValidQueueItem(x)
    if (!ok) changed = true
    return ok
  })

  for (const item of valid) {
    if (!item.state) {
      item.state = 'pending'
      changed = true
    }
    if (typeof item.intentCreatedAt !== 'number') {
      if (typeof item.lastAttemptAt === 'number') {
        // Older v2 clients reset createdAt on retry/rebind, so once an item
        // has an attempt timestamp its original intent age is not trustworthy.
        // Preserve it for reconciliation but never auto-send an ambiguous
        // pre-upgrade intent after server idempotency retention may have ended.
        item.state = 'attention'
        item.attentionReason = 'retention_unknown'
        item.lastError =
          'A idade original desta venda não é confiável após a atualização. Confira no servidor antes de qualquer reenvio ou ajuste.'
      } else {
        item.intentCreatedAt = item.createdAt
      }
      changed = true
    }
    if (item.state === 'pending' && now - item.createdAt > QUEUE_TTL_MS) {
      item.state = 'attention'
      item.attentionReason = 'expired'
      item.lastError = 'Venda offline expirou antes da sincronizacao automatica.'
      changed = true
    }
    if (item.state === 'pending' && !saleHasPriceSnapshots(item)) {
      item.state = 'attention'
      item.attentionReason = 'price_snapshot_missing'
      item.lastError =
        'Venda criada antes da proteção de snapshot de preço. Confira os valores cobrados antes de qualquer reenvio.'
      changed = true
    }
  }

  if (changed) saveQueue(valid)
  return valid
}

function saveQueue(queue: QueuedRequest[]): void {
  const key = queueKey()
  if (!key) return
  localStorage.setItem(key, JSON.stringify(queue))
}

export function getQueueItems(): QueuedRequest[] {
  return loadQueue()
    .slice()
    .sort((a, b) => a.createdAt - b.createdAt)
}

export function getQueueSummary(): QueueSummary {
  const queue = loadQueue()
  const pending = queue.filter((item) => item.state !== 'attention').length
  const attention = queue.filter((item) => item.state === 'attention').length
  return { pending, attention, total: queue.length }
}

export function getQueueCount(): number {
  return getQueueSummary().total
}

export function getAllUserQueueCount(): number {
  let total = 0
  for (const key of userScopedStorageKeys(QUEUE_NAMESPACE)) {
    const parsed = safeJsonParse<QueuedRequest[]>(localStorage.getItem(key))
    if (!parsed || !Array.isArray(parsed)) continue
    total += parsed.filter(isValidQueueItem).length
  }
  return total
}

export function getLegacyQueueCount(): number {
  return readLegacyQueue().length
}

export function claimLegacyQueue(): number {
  const key = queueKey()
  if (!key) throw new Error('authenticated tenant/user scope is required')

  const legacy = readLegacyQueue()
  if (legacy.length === 0) {
    localStorage.removeItem(LEGACY_QUEUE_KEY)
    return 0
  }

  const current = loadQueue()
  for (const old of legacy) {
    const headers = sanitizeQueuedHeaders(old.headers)
    if (old.path.includes('/api/v1/sales') && !headers['Idempotency-Key']) {
      headers['Idempotency-Key'] = old.id
    }
    current.push({
      id: crypto.randomUUID(),
      createdAt: old.createdAt,
      intentCreatedAt: old.intentCreatedAt ?? old.createdAt,
      method: old.method,
      path: old.path,
      body: sanitizeQueuedBody(old.path, old.body),
      headers,
      state: 'attention',
      attentionReason: 'legacy_migration',
      lastError:
        'Item importado da fila anterior ao isolamento por tenant. Revise e tente novamente manualmente.',
    })
  }

  saveQueue(current)
  localStorage.removeItem(LEGACY_QUEUE_KEY)
  return legacy.length
}

export function discardLegacyQueue(): void {
  localStorage.removeItem(LEGACY_QUEUE_KEY)
}

export function enqueueRequest(req: Omit<QueuedRequest, 'id' | 'createdAt' | 'state'>): string {
  const key = queueKey()
  if (!key) throw new Error('authenticated tenant/user scope is required')

  const id = crypto.randomUUID()
  const headers = sanitizeQueuedHeaders(req.headers)
  if (req.path.includes('/api/v1/sales') && !headers['Idempotency-Key']) {
    headers['Idempotency-Key'] = id
  }
  const createdAt = Date.now()
  const next: QueuedRequest = {
    id,
    createdAt,
    intentCreatedAt: createdAt,
    method: req.method,
    path: req.path,
    body: sanitizeQueuedBody(req.path, req.body),
    headers,
    state: 'pending',
  }
  const queue = loadQueue()
  queue.push(next)
  saveQueue(queue)
  return id
}

export function retryQueueItem(id: string): boolean {
  const queue = loadQueue()
  const item = queue.find((candidate) => candidate.id === id)
  if (!item || item.state !== 'attention') return false
  if (item.attentionReason === 'retention_unknown') return false

  const intentCreatedAt = item.intentCreatedAt ?? item.createdAt
  if (Date.now() - intentCreatedAt > SAFE_MANUAL_REPLAY_MS) {
    item.attentionReason = 'retention_expired'
    item.lastError =
      'Venda antiga demais para reenvio idempotente seguro. Confira no servidor antes de descartar ou lançar um ajuste manual.'
    item.lastAttemptAt = Date.now()
    saveQueue(queue)
    return false
  }
  if (!saleHasPriceSnapshots(item)) {
    item.attentionReason = 'price_snapshot_missing'
    item.lastError =
      'Venda sem snapshot confiável de preço. Confira os valores cobrados antes de qualquer reenvio.'
    item.lastAttemptAt = Date.now()
    saveQueue(queue)
    return false
  }

  item.state = 'pending'
  item.createdAt = Date.now()
  item.lastAttemptAt = item.createdAt
  delete item.attentionReason
  delete item.lastError
  saveQueue(queue)
  return true
}

export function discardQueueItem(id: string): boolean {
  const queue = loadQueue()
  const next = queue.filter((item) => item.id !== id)
  if (next.length === queue.length) return false
  saveQueue(next)
  return true
}

export function markQueueItemAttention(
  id: string,
  reason: AttentionReason,
  message: string,
): boolean {
  const queue = loadQueue()
  const item = queue.find((candidate) => candidate.id === id)
  if (!item) return false
  item.state = 'attention'
  item.attentionReason = reason
  item.lastError = message
  item.lastAttemptAt = Date.now()
  saveQueue(queue)
  return true
}

export function rebindQueueItemToCashSession(id: string, cashSessionId: string): boolean {
  const normalizedCashSessionId = cashSessionId.trim()
  if (!normalizedCashSessionId) return false

  const queue = loadQueue()
  const item = queue.find((candidate) => candidate.id === id)
  if (!item || item.state !== 'attention' || !item.path.includes('/api/v1/sales')) return false
  if (!item.body || typeof item.body !== 'object') return false
  if (item.attentionReason === 'retention_unknown') return false

  const intentCreatedAt = item.intentCreatedAt ?? item.createdAt
  if (Date.now() - intentCreatedAt > SAFE_MANUAL_REPLAY_MS) {
    item.attentionReason = 'retention_expired'
    item.lastError =
      'Venda antiga demais para rebind/reenvio idempotente seguro. Confira no servidor antes de qualquer ajuste.'
    item.lastAttemptAt = Date.now()
    saveQueue(queue)
    return false
  }
  if (!saleHasPriceSnapshots(item)) {
    item.attentionReason = 'price_snapshot_missing'
    item.lastError =
      'Venda sem snapshot confiável de preço. Confira os valores cobrados antes de qualquer rebind/reenvio.'
    item.lastAttemptAt = Date.now()
    saveQueue(queue)
    return false
  }

  const body = item.body as Record<string, unknown>
  item.body = { ...body, cash_session_id: normalizedCashSessionId }
  const headers = sanitizeQueuedHeaders(item.headers)
  // Preserve the original key. If the old request was already committed,
  // the backend will return an idempotency conflict for the changed payload
  // instead of accepting a duplicate sale under a new key.
  if (!headers['Idempotency-Key']) headers['Idempotency-Key'] = item.id
  item.headers = headers
  item.state = 'pending'
  item.createdAt = Date.now()
  item.lastAttemptAt = item.createdAt
  delete item.attentionReason
  delete item.lastError
  saveQueue(queue)
  return true
}

export async function refreshQueueItemPriceSnapshots(id: string): Promise<boolean> {
  const initial = loadQueue()
  const item = initial.find((candidate) => candidate.id === id)
  if (!item || item.state !== 'attention' || item.attentionReason !== 'price_changed') {
    return false
  }
  if (!item.body || typeof item.body !== 'object') return false

  const body = item.body as {
    discount_value?: unknown
    items?: unknown
    payments?: unknown
  }
  if (!Array.isArray(body.items) || body.items.length === 0) return false
  if (!Array.isArray(body.payments) || body.payments.length !== 1) {
    item.lastError =
      'A venda possui pagamento composto e exige reconciliação manual antes de atualizar preços.'
    saveQueue(initial)
    return false
  }

  type ProductPrice = {
    price_cash: number
    promo_price?: number | null
    active: boolean
  }

  const updatedItems: Array<Record<string, unknown>> = []
  let totalCents = -moneyValueCents(body.discount_value)

  try {
    for (const raw of body.items) {
      if (!raw || typeof raw !== 'object') return false
      const source = raw as Record<string, unknown>
      const productID = typeof source.product_id === 'string' ? source.product_id.trim() : ''
      const qty = typeof source.qty === 'number' ? source.qty : Number(source.qty)
      const qtyMilli = quantityValueMilli(qty)
      if (!productID || qtyMilli === null) return false

      const product = await apiJson<ProductPrice>(
        `/api/v1/products/${encodeURIComponent(productID)}`,
      )
      if (!product.active) {
        const current = loadQueue()
        const currentItem = current.find((candidate) => candidate.id === id)
        if (currentItem) {
          currentItem.lastError =
            'Um produto desta venda está inativo. A reconciliação deve ser feita manualmente.'
          saveQueue(current)
        }
        return false
      }

      const price =
        typeof product.promo_price === 'number' && product.promo_price > 0
          ? product.promo_price
          : product.price_cash
      const priceCents = moneyValueCents(price)
      if (priceCents <= 0) return false

      const lineGrossCents = roundPositiveInteger(priceCents * qtyMilli, 1000)
      const itemDiscountCents = moneyValueCents(source.discount_value)
      totalCents += lineGrossCents - itemDiscountCents
      updatedItems.push({ ...source, unit_price: priceCents / 100 })
    }
  } catch (error) {
    const current = loadQueue()
    const currentItem = current.find((candidate) => candidate.id === id)
    if (currentItem) {
      currentItem.lastError = `Não foi possível atualizar os preços: ${errorMessage(error)}`
      saveQueue(current)
    }
    return false
  }

  if (totalCents <= 0) return false

  const payment = body.payments[0]
  if (!payment || typeof payment !== 'object') return false

  // Reload after the network calls so a concurrent discard/change wins.
  const current = loadQueue()
  const currentItem = current.find((candidate) => candidate.id === id)
  if (
    !currentItem ||
    currentItem.state !== 'attention' ||
    currentItem.attentionReason !== 'price_changed' ||
    !currentItem.body ||
    typeof currentItem.body !== 'object'
  ) {
    return false
  }

  const currentBody = currentItem.body as Record<string, unknown>
  currentItem.body = {
    ...currentBody,
    items: updatedItems,
    payments: [{ ...(payment as Record<string, unknown>), amount: totalCents / 100 }],
  }
  currentItem.state = 'pending'
  currentItem.createdAt = Date.now()
  currentItem.lastAttemptAt = currentItem.createdAt
  delete currentItem.attentionReason
  delete currentItem.lastError
  saveQueue(current)
  return true
}

function moneyValueCents(value: unknown): number {
  const n = typeof value === 'number' ? value : Number(value ?? 0)
  return Number.isFinite(n) ? Math.round(n * 100) : 0
}

function quantityValueMilli(value: number): number | null {
  if (!Number.isFinite(value) || value <= 0) return null
  const scaled = value * 1000
  const milli = Math.round(scaled)
  if (Math.abs(scaled - milli) > 1e-6) return null
  return milli
}

function roundPositiveInteger(numerator: number, denominator: number): number {
  if (numerator < 0 || denominator <= 0) return 0
  return Math.floor((numerator + Math.floor(denominator / 2)) / denominator)
}

export function clearOfflineQueue(): void {
  const key = queueKey()
  if (key) localStorage.removeItem(key)
}

export function clearAllOfflineQueuesForCurrentUser(): void {
  // The legacy v1 queue has no trustworthy tenant/user ownership metadata.
  // Never delete it as part of account-scoped cleanup; it must be explicitly
  // imported for review or discarded by the operator in the PDV.
  clearAllUserScopedStorage(QUEUE_NAMESPACE)
}

function sanitizeQueuedHeaders(headers?: Record<string, string>): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [key, value] of Object.entries(headers ?? {})) {
    const normalized = key.toLowerCase()
    if (normalized === 'authorization' || normalized === 'cookie' || normalized.includes('token')) {
      continue
    }
    out[key] = value
  }
  return out
}

function sanitizeQueuedBody(path: string, body: unknown): unknown {
  if (!path.includes('/api/v1/sales') || !body || typeof body !== 'object') {
    return body
  }
  const sale = body as {
    cash_session_id?: unknown
    customer_id?: unknown
    discount_value?: unknown
    items?: unknown
    payments?: unknown
  }
  return {
    cash_session_id: sale.cash_session_id,
    customer_id: sale.customer_id ?? null,
    discount_value: sale.discount_value ?? 0,
    items: sale.items,
    payments: sale.payments,
  }
}

function isPermanentQueueError(error: unknown): boolean {
  if (!(error instanceof APIError)) return false
  if (error.status < 400 || error.status >= 500) return false
  return ![401, 408, 425, 429].includes(error.status)
}

export type FlushResult =
  | { ok: true; processed: number; remaining: number; attention: number }
  | { ok: false; processed: number; remaining: number; attention: number; error: string }

export async function flushQueue(): Promise<FlushResult> {
  if (!navigator.onLine) {
    const summary = getQueueSummary()
    return { ok: true, processed: 0, remaining: summary.total, attention: summary.attention }
  }

  const queue = loadQueue().sort((a, b) => a.createdAt - b.createdAt)
  let processed = 0

  for (const item of queue) {
    if (item.state === 'attention') continue

    try {
      await apiJson(item.path, {
        method: item.method,
        body: item.body,
        headers: item.headers,
      })

      const current = loadQueue().filter((x) => x.id !== item.id)
      saveQueue(current)
      processed += 1
    } catch (e: unknown) {
      const msg = errorMessage(e)
      if (isPermanentQueueError(e)) {
        const current = loadQueue()
        const failed = current.find((x) => x.id === item.id)
        if (failed) {
          failed.state = 'attention'
          failed.attentionReason =
            e instanceof APIError && e.code === 'price_changed'
              ? 'price_changed'
              : 'request_rejected'
          failed.lastError = msg
          failed.lastAttemptAt = Date.now()
          saveQueue(current)
        }
        continue
      }

      const summary = getQueueSummary()
      return {
        ok: false,
        processed,
        remaining: summary.total,
        attention: summary.attention,
        error: msg,
      }
    }
  }

  const summary = getQueueSummary()
  return { ok: true, processed, remaining: summary.total, attention: summary.attention }
}
