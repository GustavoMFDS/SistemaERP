import { scopedStorageKey } from './auth'

const SUSPENDED_CART_NAMESPACE = 'sistemaemgo:suspendedCarts:v1'
const MAX_SUSPENDED_CARTS = 20

export type SuspendedCartItem = {
  product_id: string
  qty: number
  unit_price: number
  discount_value: number
}

export type SuspendedCart = {
  id: string
  createdAt: string
  items: SuspendedCartItem[]
  payMethod: string
  saleDiscount: number
}

function storageKey(): string {
  return scopedStorageKey(SUSPENDED_CART_NAMESPACE) ?? ''
}

export function getSuspendedCarts(): SuspendedCart[] {
  const key = storageKey()
  if (!key) return []
  const raw = localStorage.getItem(key)
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw) as SuspendedCart[]
    if (!Array.isArray(parsed)) return []
    return parsed
      .filter((item) => item && typeof item.id === 'string' && Array.isArray(item.items))
      .slice(0, MAX_SUSPENDED_CARTS)
  } catch {
    return []
  }
}

function save(items: SuspendedCart[]): void {
  const key = storageKey()
  if (!key) throw new Error('authenticated tenant/user scope is required')
  if (items.length > MAX_SUSPENDED_CARTS) {
    throw new Error(`limite de ${MAX_SUSPENDED_CARTS} vendas suspensas atingido`)
  }
  localStorage.setItem(key, JSON.stringify(items))
}

export function suspendCart(input: Omit<SuspendedCart, 'id' | 'createdAt'>): SuspendedCart {
  if (input.items.length === 0) throw new Error('empty cart cannot be suspended')
  const current = getSuspendedCarts()
  if (current.length >= MAX_SUSPENDED_CARTS) {
    throw new Error(`limite de ${MAX_SUSPENDED_CARTS} vendas suspensas atingido`)
  }
  const cart: SuspendedCart = {
    ...input,
    id: crypto.randomUUID(),
    createdAt: new Date().toISOString(),
  }
  save([cart, ...current])
  return cart
}

export function removeSuspendedCart(id: string): void {
  save(getSuspendedCarts().filter((item) => item.id !== id))
}

export function clearSuspendedCarts(): void {
  const key = storageKey()
  if (key) localStorage.removeItem(key)
}
