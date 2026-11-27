import { expect, test } from '@playwright/test'

test('a rejected request from one tenant is not retried under another tenant', async ({ page }) => {
  const token = (sub: string, tenant_id: string) => 'fixture.' + Buffer.from(JSON.stringify({
    sub, tenant_id, exp: Math.floor(Date.now() / 1000) + 900,
  })).toString('base64url') + '.fixture'
  const original = token('user-a', 'tenant-a')
  const changed = token('user-b', 'tenant-b')
  const sent: (string | undefined)[] = []
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path.endsWith('/auth/refresh')) return route.fulfill({ json: { access_token: changed } })
    if (path.endsWith('/g4-synthetic-probe')) {
      sent.push(route.request().headers().authorization)
      return route.fulfill({ status: sent.length === 1 ? 401 : 200, json: {} })
    }
    return route.fulfill({ json: { items: [], total: 0 } })
  })
  await page.goto('/login')
  const result = await page.evaluate(async (initial) => {
    const { setToken } = await import('/src/lib/auth.ts')
    const { apiJson } = await import('/src/lib/api.ts')
    setToken(initial)
    try {
      await apiJson('/api/v1/g4-synthetic-probe', { method: 'POST', body: { fixture: true } })
      return 'completed'
    } catch {
      return 'blocked'
    }
  }, original)
  expect(sent).toEqual(['Bearer ' + original])
  expect(result).toBe('blocked')
})
