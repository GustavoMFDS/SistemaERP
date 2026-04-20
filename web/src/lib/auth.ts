const TOKEN_KEY = 'auth_token'
const CASH_SESSION_KEY = 'cash_session_id'

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? ''
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

export function getCashSessionId(): string {
  return localStorage.getItem(CASH_SESSION_KEY) ?? ''
}

export function setCashSessionId(id: string): void {
  localStorage.setItem(CASH_SESSION_KEY, id)
}

export function clearCashSessionId(): void {
  localStorage.removeItem(CASH_SESSION_KEY)
}
