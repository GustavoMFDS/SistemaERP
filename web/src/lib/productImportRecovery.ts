import { scopedStorageKey } from './auth'
import type { ProductImportRow } from './productImport'

const NAMESPACE = 'sistemaemgo:pendingProductImport:v1'
const MAX_AGE_MS = 30 * 24 * 60 * 60 * 1000

// No CSV rows, customer identifiers, product prices, tax data or secrets
// are persisted. This record is scoped by tenant and authenticated user.
export type PendingProductImport = {
  key: string
  digest: string
  createdAt: number
}

function isValid(value: unknown, now: number): value is PendingProductImport {
  if (!value || typeof value !== 'object') return false
  const item = value as Partial<PendingProductImport>
  return typeof item.key === 'string' &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(item.key) &&
    typeof item.digest === 'string' && /^[a-f0-9]{64}$/.test(item.digest) &&
    typeof item.createdAt === 'number' && Number.isFinite(item.createdAt) &&
    item.createdAt > 0 && item.createdAt <= now + 60_000 &&
    now - item.createdAt <= MAX_AGE_MS
}

export function readPendingProductImport(now = Date.now()): PendingProductImport | null {
  const key = scopedStorageKey(NAMESPACE)
  if (!key) return null
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return null
    const record: unknown = JSON.parse(raw)
    return isValid(record, now) ? record : null
  } catch {
    return null
  }
}

export function savePendingProductImport(record: PendingProductImport): void {
  const key = scopedStorageKey(NAMESPACE)
  if (!key) throw new Error('Entre na loja antes de iniciar uma importação.')
  if (!isValid(record, Date.now())) throw new Error('Referência de importação inválida.')
  const previous = readPendingProductImport()
  if (previous && (previous.key !== record.key || previous.digest !== record.digest)) {
    throw new Error('Existe uma importação anterior ainda pendente nesta conta e loja.')
  }
  // Storage failure must abort BEFORE the first POST.
  localStorage.setItem(key, JSON.stringify(record))
}

export function clearPendingProductImport(expectedKey: string): void {
  const key = scopedStorageKey(NAMESPACE)
  if (key && readPendingProductImport()?.key === expectedKey) localStorage.removeItem(key)
}

export async function fingerprintProducts(rows: ProductImportRow[]): Promise<string> {
  if (rows.length < 1 || rows.length > 500) throw new Error('Número de produtos inválido.')
  const normalized = rows.map((row) => ({
    sku: row.sku.trim(),
    name: row.name.trim(),
    unit: row.unit.trim(),
    barcode: row.barcode?.trim() || null,
    ncm: row.ncm?.trim() || null,
    cest: row.cest?.trim() || null,
    price_cash: row.price_cash.toFixed(2),
    min_stock: row.min_stock.toFixed(3),
  })).sort((a, b) => a.sku < b.sku ? -1 : a.sku > b.sku ? 1 : 0)
  const bytes = new TextEncoder().encode(JSON.stringify(normalized))
  const hash = await crypto.subtle.digest('SHA-256', bytes)
  return Array.from(new Uint8Array(hash), (n) => n.toString(16).padStart(2, '0')).join('')
}
