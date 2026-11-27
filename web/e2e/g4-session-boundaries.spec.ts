import { expect, test } from '@playwright/test'

const start = new Date('2026-11-26T12:00:00Z')
const expired = new Date('2026-11-26T12:16:00Z')
function token(user: string, tenant: string, time: Date) {
  return 'fixture.' + Buffer.from(JSON.stringify({
    sub: user, tenant_id: tenant, exp: Math.floor(time.getTime() / 1000) + 900,
  })).toString('base64url') + '.fixture'
}

for (const mode of ['concurrent', 'offline', 'different-scope'] as const) {
  test('protected navigation boundary: ' + mode, async ({ page }) => {
    await page.clock.setFixedTime(start)
    const initial = token('user-a', 'tenant-a', start)
    const renewed = mode === 'different-scope'
      ? token('user-b', 'tenant-b', expired) : token('user-a', 'tenant-a', expired)
    let refreshes = 0
    let refreshed = false
    let release!: () => void
    const pending = new Promise<void>((resolve) => { release = resolve })
    const protectedHeaders: (string | undefined)[] = []
    const probeHeaders: (string | undefined)[] = []
    await page.route('**/api/v1/**', async (route) => {
      const url = new URL(route.request().url())
      const header = route.request().headers().authorization
      if (url.pathname.endsWith('/auth/refresh')) {
        refreshes++
        if (refreshes === 1) return route.fulfill({ json: { access_token: initial } })
        await pending
        if (mode === 'offline') return route.abort('internetdisconnected')
        refreshed = true
        return route.fulfill({ json: { access_token: renewed } })
      }
      if (url.pathname.endsWith('/auth/me')) {
        const b = mode === 'different-scope' && refreshed
        return route.fulfill({ json: {
          id: b ? 'user-b' : 'user-a', tenant_id: b ? 'tenant-b' : 'tenant-a',
          name: b ? 'Lab B' : 'Lab A', email: b ? 'b@example.invalid' : 'a@example.invalid',
          roles: ['admin'], permissions: ['inventory:read'],
        } })
      }
      if (url.searchParams.has('g4probe')) {
        probeHeaders.push(header)
        return route.fulfill({ status: refreshed ? 200 : 401, json: { items: [], total: 0 } })
      }
      if (url.pathname.endsWith('/inventory/movements') && url.searchParams.get('limit') === '20') {
        protectedHeaders.push(header)
        return route.fulfill({ json: { total: 0, items: [] } })
      }
      return route.fulfill({ json: { items: [], total: 0 } })
    })
    try {
      await page.goto('/setup')
      const link = page.getByRole('navigation').getByRole('link', { name: /^Movimenta/ })
      await expect(link).toBeVisible()
      await page.evaluate(async () => {
        const auth = await import('/src/lib/auth.ts')
        auth.setCashSessionId('cash-a')
      })
      await page.clock.setFixedTime(expired)
      const probes = mode === 'concurrent' ? page.evaluate(async () => {
        const { apiJson } = await import('/src/lib/api.ts')
        await Promise.all([apiJson('/api/v1/products?g4probe=1'), apiJson('/api/v1/products?g4probe=2')])
      }) : null
      if (probes) await expect.poll(() => refreshes).toBe(2)
      await link.click()
      await expect.poll(() => refreshes).toBe(2)
      await expect(page.getByRole('status')).toBeVisible()
      expect(protectedHeaders).toEqual([])
      release()
      if (mode === 'offline' || mode === 'different-scope') {
        await expect(page).toHaveURL(/\/login$/)
        expect(protectedHeaders).toEqual([])
      } else {
        await expect(page.getByRole('heading', { name: /Movimenta/ })).toBeVisible()
        await expect.poll(() => protectedHeaders.length).toBeGreaterThan(0)
        expect(protectedHeaders.every((header) => header === 'Bearer ' + renewed)).toBe(true)
      }
      if (probes) {
        await probes
        expect(probeHeaders.filter((h) => h === 'Bearer ' + renewed)).toHaveLength(2)
      }
      const storage = await page.evaluate(async () => {
        const auth = await import('/src/lib/auth.ts')
        return { cash: auth.getCashSessionId(), original: localStorage.getItem('sistemaemgo:cashSession:v2:tenant-a%3Auser-a') }
      })
      expect(storage.original).toBe('cash-a')
      expect(storage.cash).toBe(mode === 'concurrent' ? 'cash-a' : '')
      expect(refreshes).toBe(2)
    } finally { release() }
  })
}
