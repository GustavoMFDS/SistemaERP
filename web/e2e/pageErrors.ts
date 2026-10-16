import type { Page } from '@playwright/test'

// Capture only browser errors and HTTP status/path for session requests.
// Never include response bodies, cookies or authentication tokens in CI logs.
export function captureBrowserFailures(page: Page): () => string {
  const notes: string[] = []
  const save = (message: string) => {
    notes.push(message.slice(0, 320))
    if (notes.length > 16) notes.shift()
  }
  page.on('pageerror', (error) => save('pageerror: ' + error.message))
  page.on('console', (entry) => {
    if (entry.type() === 'error') save('console: ' + entry.text())
  })
  page.on('requestfailed', (request) => {
    const pathname = new URL(request.url()).pathname
    if (pathname.startsWith('/api/v1/')) {
      save('requestfailed: ' + request.method() + ' ' + pathname + ' ' + (request.failure()?.errorText ?? ''))
    }
  })
  page.on('response', (response) => {
    const pathname = new URL(response.url()).pathname
    if (pathname.startsWith('/api/v1/auth/') && response.status() >= 400) {
      save('authentication: ' + response.status() + ' ' + pathname)
    }
  })
  return () => notes.join(' | ') || 'Nenhum erro capturado'
}
