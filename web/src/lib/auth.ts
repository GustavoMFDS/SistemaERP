const CASH_SESSION_KEY = 'cash_session_id'

let accessToken = ''

export function getToken(): string {
  return accessToken
}

export function setToken(token: string): void {
  accessToken = token
}

export function clearToken(): void {
  accessToken = ''
  sessionStorage.removeItem('auth_token')
  localStorage.removeItem('auth_token')
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
