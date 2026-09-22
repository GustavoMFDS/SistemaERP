import { APIError, apiJson, errorMessage } from './api'
import { scopedStorageKey } from './auth'

export type QueueState = 'pending' | 'attention'

export type QueuedRequest = {
  id: string
  createdAt: number
  method: string
  path: string
  body?: unknown
  headers?: Record<string, string>
  state?: QueueState
  attentionReason?: 'expired' | 'request_rejected'
  lastError?: string
  lastAttemptAt?: number
}

export type QueueSummary = {
  pending: number
  attention: number
  total: number
}

const QUEUE_NAMESPACE = 'sistemaemgo:offlineQueue:v2'
const QUEUE_TTL_MS = 24 * 60 * 60 * 1000

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

function loadQueue(): QueuedRequest[] {
  const key = queueKey()
  if (!key) return []

  const parsed = safeJsonParse<QueuedRequest[]>(localStorage.getItem(key))
  if (!parsed || !Array.isArray(parsed)) return []

  const now = Date.now()
  let changed = false
  const valid = parsed.filter((x) => {
    const ok =
      x &&
      typeof x.id === 'string' &&
      typeof x.createdAt === 'number' &&
      typeof x.method === 'string' &&
      typeof x.path === 'string'
    if (!ok) changed = true
    return ok
  })

  for (const item of valid) {
    if (!item.state) {
      item.state = 'pending'
      changed = true
    }
    if (item.state === 'pending' && now - item.createdAt > QUEUE_TTL_MS) {
      item.state = 'attention'
      item.attentionReason = 'expired'
      item.lastError = 'Venda offline expirou antes da sincronizacao automatica.'
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

export function getQueueSummary(): QueueSummary {
  const queue = loadQueue()
  const pending = queue.filter((item) => item.state !== 'attention').length
  const attention = queue.filter((item) => item.state === 'attention').length
  return { pending, attention, total: queue.length }
}

export function getQueueCount(): number {
  return getQueueSummary().total
}

export function enqueueRequest(req: Omit<QueuedRequest, 'id' | 'createdAt' | 'state'>): string {
  const key = queueKey()
  if (!key) throw new Error('authenticated tenant/user scope is required')

  const id = crypto.randomUUID()
  const headers = sanitizeQueuedHeaders(req.headers)
  if (req.path.includes('/api/v1/sales') && !headers['Idempotency-Key']) {
    headers['Idempotency-Key'] = id
  }
  const next: QueuedRequest = {
    id,
    createdAt: Date.now(),
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

export function clearOfflineQueue(): void {
  const key = queueKey()
  if (key) localStorage.removeItem(key)
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
          failed.attentionReason = 'request_rejected'
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
