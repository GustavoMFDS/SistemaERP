import { clearToken, getToken, setToken } from './auth'

let refreshPromise: Promise<boolean> | null = null

export class APIError extends Error {
  status: number
  bodyText?: string

  constructor(status: number, message: string, bodyText?: string) {
    super(message)
    this.status = status
    this.bodyText = bodyText
  }
}

export function errorMessage(error: unknown): string {
  if (error instanceof APIError) return error.message
  if (error instanceof Error) return error.message
  return String(error)
}

function baseUrl(): string {
  return (import.meta.env.VITE_API_BASE_URL ?? 'http://localhost:8080').replace(
    /\/$/,
    '',
  )
}

function buildUrl(path: string): string {
  if (path.startsWith('http://') || path.startsWith('https://')) return path
  const normalized = path.startsWith('/') ? path : `/${path}`
  return `${baseUrl()}${normalized}`
}

export async function apiJson<T>(
  path: string,
  init: Omit<RequestInit, 'body'> & { body?: unknown } = {},
): Promise<T> {
  return apiJsonInternal<T>(path, init, true)
}

async function apiJsonInternal<T>(
  path: string,
  init: Omit<RequestInit, 'body'> & { body?: unknown } = {},
  allowRefresh: boolean,
): Promise<T> {
  const token = getToken()

  const headers = new Headers(init.headers)
  headers.set('Accept', 'application/json')
  if (!headers.has('Content-Type') && init.body !== undefined) {
    headers.set('Content-Type', 'application/json')
  }
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(buildUrl(path), {
    ...init,
    headers,
    credentials: 'include',
    body:
      init.body === undefined
        ? undefined
        : typeof init.body === 'string'
          ? init.body
          : JSON.stringify(init.body),
  })

  if (!res.ok) {
    if (res.status === 401 && allowRefresh && !path.includes('/api/v1/auth/')) {
      const refreshed = await refreshAccessToken()
      if (refreshed) return apiJsonInternal<T>(path, init, false)
    }
    const text = await res.text().catch(() => '')
    let message = `HTTP ${res.status}`
    try {
      const parsed = JSON.parse(text) as { message?: string }
      if (parsed.message) message = parsed.message
    } catch {
      // keep fallback
    }
    throw new APIError(res.status, message, text)
  }

  const ct = res.headers.get('content-type') ?? ''
  if (ct.includes('application/json')) {
    return (await res.json()) as T
  }

  // Fallback: backend sometimes returns plain text
  const text = await res.text()
  return text as unknown as T
}

export function refreshAccessToken(): Promise<boolean> {
  if (refreshPromise) return refreshPromise

  refreshPromise = performRefresh().finally(() => {
    refreshPromise = null
  })
  return refreshPromise
}

async function performRefresh(): Promise<boolean> {
  try {
    const token = await apiJsonInternal<{ access_token: string }>(
      '/api/v1/auth/refresh',
      { method: 'POST' },
      false,
    )
    setToken(token.access_token)
    return true
  } catch {
    clearToken()
    return false
  }
}

export async function apiDownload(
  path: string,
  fileName: string,
  mime = 'application/octet-stream',
): Promise<void> {
  return apiDownloadInternal(path, fileName, mime, true)
}

async function apiDownloadInternal(
  path: string,
  fileName: string,
  mime: string,
  allowRefresh: boolean,
): Promise<void> {
  const token = getToken()
  const headers = new Headers()
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(buildUrl(path), { headers, credentials: 'include' })
  if (!res.ok) {
    if (res.status === 401 && allowRefresh) {
      const refreshed = await refreshAccessToken()
      if (refreshed) return apiDownloadInternal(path, fileName, mime, false)
    }
    const text = await res.text().catch(() => '')
    let message = `HTTP ${res.status}`
    try {
      const parsed = JSON.parse(text) as { message?: string }
      if (parsed.message) message = parsed.message
    } catch {
      // keep fallback
    }
    throw new APIError(res.status, message, text)
  }

  const blob = await res.blob()
  const url = URL.createObjectURL(new Blob([blob], { type: mime }))
  try {
    const a = document.createElement('a')
    a.href = url
    a.download = fileName
    document.body.appendChild(a)
    a.click()
    a.remove()
  } finally {
    URL.revokeObjectURL(url)
  }
}

export async function apiOpenPrintable(path: string, autoPrint = false): Promise<void> {
  const popup = window.open('about:blank', '_blank')
  if (!popup) throw new Error('O navegador bloqueou a nova aba de impressão.')
  popup.opener = null
  try {
    await apiOpenPrintableInternal(path, popup, true, autoPrint)
  } catch (error) {
    popup.close()
    throw error
  }
}

async function apiOpenPrintableInternal(
  path: string,
  popup: Window,
  allowRefresh: boolean,
  autoPrint: boolean,
): Promise<void> {
  const token = getToken()
  const headers = new Headers({ Accept: 'text/html' })
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(buildUrl(path), { headers, credentials: 'include' })
  if (!res.ok) {
    if (res.status === 401 && allowRefresh) {
      const refreshed = await refreshAccessToken()
      if (refreshed) return apiOpenPrintableInternal(path, popup, false, autoPrint)
    }
    const text = await res.text().catch(() => '')
    let message = `HTTP ${res.status}`
    try {
      const parsed = JSON.parse(text) as { message?: string }
      if (parsed.message) message = parsed.message
    } catch {
      // keep fallback
    }
    throw new APIError(res.status, message, text)
  }

  const blob = await res.blob()
  const url = URL.createObjectURL(new Blob([blob], { type: 'text/html' }))
  if (autoPrint) {
    // A single user click opens the popup. When the authorized DANFE HTML
    // loads, show the OS printing dialog, never silently print on a device.
    popup.onload = () => {
      popup.onload = null
      popup.focus()
      popup.print()
    }
  }
  popup.location.replace(url)
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
}

