import { apiJson, errorMessage } from './api'

export type QueuedRequest = {
  id: string
  createdAt: number
  method: string
  path: string
  body?: unknown
  headers?: Record<string, string>
}

const QUEUE_KEY = 'sistemaemgo:offlineQueue:v1'
const QUEUE_TTL_MS = 24 * 60 * 60 * 1000

function safeJsonParse<T>(raw: string | null): T | null {
  if (!raw) return null
  try {
    return JSON.parse(raw) as T
  } catch {
    return null
  }
}

function loadQueue(): QueuedRequest[] {
  const parsed = safeJsonParse<QueuedRequest[]>(localStorage.getItem(QUEUE_KEY))
  if (!parsed || !Array.isArray(parsed)) return []
  const now = Date.now()
  const valid = parsed.filter(
    (x) =>
      x &&
      typeof x.id === 'string' &&
      typeof x.createdAt === 'number' &&
      now - x.createdAt <= QUEUE_TTL_MS &&
      typeof x.method === 'string' &&
      typeof x.path === 'string',
  )
  if (valid.length !== parsed.length) saveQueue(valid)
  return valid
}

function saveQueue(queue: QueuedRequest[]): void {
  localStorage.setItem(QUEUE_KEY, JSON.stringify(queue))
}

export function getQueueCount(): number {
  return loadQueue().length
}

export function enqueueRequest(req: Omit<QueuedRequest, 'id' | 'createdAt'>): string {
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
  }
  const queue = loadQueue()
  queue.push(next)
  saveQueue(queue)
  return id
}

export function clearOfflineQueue(): void {
  localStorage.removeItem(QUEUE_KEY)
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

export type FlushResult =
  | { ok: true; processed: number; remaining: number }
  | { ok: false; processed: number; remaining: number; error: string }

export async function flushQueue(): Promise<FlushResult> {
  if (!navigator.onLine) {
    return { ok: true, processed: 0, remaining: getQueueCount() }
  }

  const queue = loadQueue().sort((a, b) => a.createdAt - b.createdAt)
  let processed = 0

  for (const item of queue) {
    try {
      await apiJson(item.path, {
        method: item.method,
        body: item.body,
        headers: item.headers,
      })

      // remove item
      const current = loadQueue().filter((x) => x.id !== item.id)
      saveQueue(current)
      processed += 1
    } catch (e: unknown) {
      const msg = errorMessage(e)
      return { ok: false, processed, remaining: getQueueCount(), error: msg }
    }
  }

  return { ok: true, processed, remaining: getQueueCount() }
}
