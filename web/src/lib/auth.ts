const LEGACY_CASH_SESSION_KEY = 'cash_session_id'
const CASH_SESSION_NAMESPACE = 'sistemaemgo:cashSession:v2'

let accessToken = ''

type TokenListener = (token: string) => void
const tokenListeners = new Set<TokenListener>()

function emitToken(): void {
  for (const listener of tokenListeners) listener(accessToken)
}

export function subscribeToken(listener: TokenListener): () => void {
  tokenListeners.add(listener)
  return () => tokenListeners.delete(listener)
}

type AccessClaims = {
  sub?: string
  tenant_id?: string
  exp?: number
}

export function getToken(): string {
  return accessToken
}

export function setToken(token: string): void {
  accessToken = token
  emitToken()
}

export function clearToken(): void {
  accessToken = ''
  sessionStorage.removeItem('auth_token')
  localStorage.removeItem('auth_token')
  emitToken()
}

function decodeAccessClaims(token: string): AccessClaims | null {
  const parts = token.split('.')
  if (parts.length < 2) return null
  try {
    const base64 = parts[1].replace(/-/g, '+').replace(/_/g, '/')
    const padded = base64.padEnd(Math.ceil(base64.length / 4) * 4, '=')
    return JSON.parse(atob(padded)) as AccessClaims
  } catch {
    return null
  }
}

export function hasUsableAccessToken(nowMs = Date.now()): boolean {
  const claims = decodeAccessClaims(accessToken)
  if (!claims?.sub || !claims.tenant_id || typeof claims.exp !== 'number') return false
  return claims.exp * 1000 > nowMs
}

export function getSessionScope(): string {
  if (!hasUsableAccessToken()) return ''
  const claims = decodeAccessClaims(accessToken)
  if (!claims?.sub || !claims.tenant_id) return ''
  return `${claims.tenant_id}:${claims.sub}`
}

export function scopedStorageKey(namespace: string): string | null {
  const scope = getSessionScope()
  return scope ? `${namespace}:${encodeURIComponent(scope)}` : null
}

export function clearScopedStorage(namespace: string): void {
  const key = scopedStorageKey(namespace)
  if (key) localStorage.removeItem(key)
}

export function getCashSessionId(): string {
  const key = scopedStorageKey(CASH_SESSION_NAMESPACE)
  return key ? (localStorage.getItem(key) ?? '') : ''
}

export function setCashSessionId(id: string): void {
  const key = scopedStorageKey(CASH_SESSION_NAMESPACE)
  if (!key) throw new Error('authenticated tenant/user scope is required')
  localStorage.setItem(key, id)
  localStorage.removeItem(LEGACY_CASH_SESSION_KEY)
}

export function clearCashSessionId(): void {
  clearScopedStorage(CASH_SESSION_NAMESPACE)
  localStorage.removeItem(LEGACY_CASH_SESSION_KEY)
}
