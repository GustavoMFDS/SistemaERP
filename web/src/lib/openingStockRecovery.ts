import { scopedStorageKey } from './auth'
import type { OpeningStockRow } from './openingStockImport'

// Only an opaque batch reference and a hash are retained. Never persist CSV
// rows, product names, quantities, credentials or financial/fiscal secrets.
const NAMESPACE = 'sistemaemgo:pendingOpeningStock:v1'
const MAX_AGE_MS = 30 * 24 * 60 * 60 * 1000

export type PendingOpeningStock = {
  key: string
  digest: string
  createdAt: number
}

function validPending(value: unknown, now: number): value is PendingOpeningStock {
  if (!value || typeof value !== 'object') return false
  const entry = value as Partial<PendingOpeningStock>
  return typeof entry.key === 'string' &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(entry.key) &&
    typeof entry.digest === 'string' && /^[a-f0-9]{64}$/.test(entry.digest) &&
    typeof entry.createdAt === 'number' && Number.isFinite(entry.createdAt) &&
    entry.createdAt > 0 && entry.createdAt <= now + 60_000 &&
    now - entry.createdAt <= MAX_AGE_MS
}

export function readPendingOpeningStock(now = Date.now()): PendingOpeningStock | null {
  const key = scopedStorageKey(NAMESPACE)
  if (!key) return null
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    if (!validPending(parsed, now)) return null
    return parsed
  } catch {
    return null
  }
}

export function savePendingOpeningStock(entry: PendingOpeningStock): void {
  const key = scopedStorageKey(NAMESPACE)
  if (!key) throw new Error('É necessário estar autenticado na loja para preservar uma tentativa.')
  if (!validPending(entry, Date.now())) throw new Error('Referência de lote inválida.')
  const current = readPendingOpeningStock()
  if (current && (current.key !== entry.key || current.digest !== entry.digest)) {
    throw new Error('Já existe uma importação pendente nesta conta e loja.')
  }
  // If storage is blocked or full, abort BEFORE any write to the server.
  localStorage.setItem(key, JSON.stringify(entry))
}

export function clearPendingOpeningStock(expectedKey: string): void {
  const key = scopedStorageKey(NAMESPACE)
  if (!key) return
  if (readPendingOpeningStock()?.key === expectedKey) localStorage.removeItem(key)
}

export async function fingerprintOpeningStock(rows: OpeningStockRow[]): Promise<string> {
  if (rows.length < 1 || rows.length > 100) throw new Error('Lote sem produtos válidos.')
  const canonical = rows.map(({ sku, quantity }) => [sku.trim(), quantity.toFixed(3)])
    .sort((a, b) => a[0].localeCompare(b[0], 'en'))
  const data = new TextEncoder().encode(JSON.stringify(canonical))
  const digest = await crypto.subtle.digest('SHA-256', data)
  return Array.from(new Uint8Array(digest), (n) => n.toString(16).padStart(2, '0')).join('')
}
