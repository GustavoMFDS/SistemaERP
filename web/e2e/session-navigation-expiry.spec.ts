import { expect, test } from '@playwright/test'

const start = new Date('2026-11-26T12:00:00Z')
const expired = new Date('2026-11-26T12:16:00Z')
function fixtureToken(exp: number) {
  return 'fixture.' + Buffer.from(JSON.stringify({ sub: 'lab-user', tenant_id: 'lab-tenant', exp })).toString('base64url') + '.fixture'
}

for (const mode of ['renew', 'reject', 'invalid', 'healthy'] as const) {
  test('access on menu navigation: ' + mode, async ({ page }) => {
    await page.clock.setFixedTime(start)
    let refreshes = 0
    const historyHeaders: (string | undefined)[] = []
    let releaseRefresh!: () => void
    const pendingRefresh = new Promise<void>((resolve) => { releaseRefresh = resolve })
    const initial = fixtureToken(Math.floor(start.getTime() / 1000) + 900)
    const renewed = fixtureToken(Math.floor(expired.getTime() / 1000) + 900)
    await page.route('**/api/v1/**', async (route) => {
      const url = new URL(route.request().url())
      if (url.pathname.endsWith('/auth/refresh')) {
        refreshes++
        if (refreshes === 1) return route.fulfill({ json: { access_token: initial } })
        await pendingRefresh
        if (mode === 'reject') return route.fulfill({ status: 401, json: { message: 'expired' } })
        return route.fulfill({ json: { access_token: mode === 'invalid' ? initial : renewed } })
      }
      if (url.pathname.endsWith('/auth/me')) return route.fulfill({ json: {
        id: 'lab-user', tenant_id: 'lab-tenant', name: 'Lab user', email: 'lab@example.invalid',
        roles: ['admin'], permissions: ['inventory:read'],
      } })
      if (url.pathname.endsWith('/inventory/movements') && url.searchParams.get('limit') === '20') {
        historyHeaders.push(route.request().headers().authorization)
        return route.fulfill({ json: { total: 1, items: [{
          id: 'm1', product_id: 'p1', product_sku: 'LAB', product_name: 'Synthetic movement product',
          movement_type: 'adjustment', delta: 1, qty_before: 0, qty_after: 1, created_at: start.toISOString(),
        }] } })
      }
      return route.fulfill({ json: { items: [], total: 0 } })
    })
    try {
      await page.goto('/setup')
      await expect(page.getByRole('heading', { name: 'Vamos preparar sua loja' })).toBeVisible()
      const movementLink = page.getByRole('navigation').getByRole('link', { name: /^Movimenta/ })
      await expect(movementLink).toBeVisible()
      expect(refreshes).toBe(1)
      if (mode !== 'healthy') await page.clock.setFixedTime(expired)
      await movementLink.click()
      if (mode !== 'healthy') {
        await expect.poll(() => refreshes).toBe(2)
        await expect(page.getByRole('status')).toBeVisible()
        await expect(page.getByRole('button', { name: 'Synthetic movement product' })).toHaveCount(0)
        expect(historyHeaders).toEqual([])
        releaseRefresh()
      }
      if (mode === 'renew' || mode === 'healthy') {
        await expect(page.getByRole('button', { name: 'Synthetic movement product' })).toBeVisible()
        expect(historyHeaders.length).toBeGreaterThan(0)
        expect(historyHeaders.every((header) => header === 'Bearer ' + (mode === 'healthy' ? initial : renewed))).toBe(true)
      } else {
        await expect(page).toHaveURL(/\/login$/)
        expect(historyHeaders).toEqual([])
      }
      expect(refreshes).toBe(mode === 'healthy' ? 1 : 2)
    } finally {
      releaseRefresh()
    }
  })
}
