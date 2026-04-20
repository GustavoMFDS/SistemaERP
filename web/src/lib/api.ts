import { getToken } from './auth'

export class APIError extends Error {
  status: number
  bodyText?: string

  constructor(status: number, message: string, bodyText?: string) {
    super(message)
    this.status = status
    this.bodyText = bodyText
  }
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
    body:
      init.body === undefined
        ? undefined
        : typeof init.body === 'string'
          ? init.body
          : JSON.stringify(init.body),
  })

  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new APIError(res.status, `HTTP ${res.status}`, text)
  }

  const ct = res.headers.get('content-type') ?? ''
  if (ct.includes('application/json')) {
    return (await res.json()) as T
  }

  // Fallback: backend sometimes returns plain text
  const text = await res.text()
  return text as unknown as T
}

export async function apiDownload(
  path: string,
  fileName: string,
  mime = 'application/octet-stream',
): Promise<void> {
  const token = getToken()
  const headers = new Headers()
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const res = await fetch(buildUrl(path), { headers })
  if (!res.ok) {
    const text = await res.text().catch(() => '')
    throw new APIError(res.status, `HTTP ${res.status}`, text)
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
