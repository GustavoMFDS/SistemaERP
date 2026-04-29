import { apiJson } from './api'

export type QueuedRequest = {
  id: string
  createdAt: number
  method: string
  path: string
  body?: unknown
  headers?: Record<string, string>
}

const QUEUE_KEY = 'sistemaemgo:offlineQueue:v1'

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
  return parsed.filter(
    (x) =>
      x &&
      typeof x.id === 'string' &&
      typeof x.createdAt === 'number' &&
      typeof x.method === 'string' &&
      typeof x.path === 'string',
  )
}

function saveQueue(queue: QueuedRequest[]): void {
  localStorage.setItem(QUEUE_KEY, JSON.stringify(queue))
}

export function getQueueCount(): number {
  return loadQueue().length
}

export function enqueueRequest(req: Omit<QueuedRequest, 'id' | 'createdAt'>): string {
  const id = crypto.randomUUID()
  const next: QueuedRequest = {
    id,
    createdAt: Date.now(),
    method: req.method,
    path: req.path,
    body: req.body,
    headers: req.headers,
  }
  const queue = loadQueue()
  queue.push(next)
  saveQueue(queue)
  return id
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
    } catch (e: any) {
      const msg = String(e?.bodyText ?? e?.message ?? e)
      return { ok: false, processed, remaining: getQueueCount(), error: msg }
    }
  }

  return { ok: true, processed, remaining: getQueueCount() }
}
