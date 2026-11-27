import { expect, test } from '@playwright/test'

const token = (sub: string, tenant_id: string, jti: string) => 'fixture.' + Buffer.from(JSON.stringify({
  sub, tenant_id, jti, exp: Math.floor(Date.now() / 1000) + 900,
})).toString('base64url') + '.fixture'

for (const action of ['login', 'logout'] as const) {
  test(action + ' waits for an outstanding refresh response', async ({ page }) => {
    const initial = token('a', 'tenant-a', 'initial')
    const renewed = token('a', 'tenant-a', 'renewed')
    const replacement = token('b', 'tenant-b', 'login')
    const events: string[] = []
    let release!: () => void
    const pending = new Promise<void>((resolve) => { release = resolve })
    let refreshing = false
    let loginDone = false
    await page.route('**/api/v1/**', async (route) => {
      const path = new URL(route.request().url()).pathname
      if (path.endsWith('/auth/refresh')) {
        if (!refreshing) return route.fulfill({ json: { access_token: initial } })
        events.push('refresh-start')
        await pending
        events.push('refresh-response')
        return route.fulfill({ json: { access_token: renewed } })
      }
      if (path.endsWith('/auth/login')) {
        events.push('login-request')
        loginDone = true
        return route.fulfill({ json: { token: { access_token: replacement }, user: { id: 'b', name: 'Lab B', roles: ['admin'] } } })
      }
      if (path.endsWith('/auth/logout')) {
        events.push('logout-request')
        return route.fulfill({ json: {} })
      }
      if (path.endsWith('/auth/me')) return route.fulfill({ json: {
        id: loginDone ? 'b' : 'a', name: loginDone ? 'Lab B' : 'Lab A', roles: ['admin'], permissions: [],
      } })
      return route.fulfill({ json: { items: [], total: 0 } })
    })
    try {
      await page.goto(action === 'login' ? '/login' : '/setup')
      if (action === 'logout') await expect(page.getByRole('button', { name: 'Sair' })).toBeVisible()
      refreshing = true
      const refresh = page.evaluate(async (initial) => {
        const auth = await import('/src/lib/auth.ts')
        const api = await import('/src/lib/api.ts')
        auth.setToken(initial)
        return api.refreshAccessToken()
      }, initial)
      await expect.poll(() => events).toEqual(['refresh-start'])
      if (action === 'login') {
        await page.getByLabel(/E-mail/).fill('lab-b@example.invalid')
        await page.getByLabel('Senha', { exact: true }).fill('fixture-only-password')
        await page.getByRole('button', { name: 'Entrar', exact: true }).click()
      } else {
        await page.getByRole('button', { name: 'Sair', exact: true }).click()
      }
      // Wait for the actual UI submit handler to enter its pending state.
      await expect(page.getByRole('button', { name: action === 'login' ? /Entrando/ : /Saindo/ })).toBeDisabled()
      // The held response makes ordering deterministic: no cookie-mutating
      // request may overtake the outstanding refresh.
      await page.waitForTimeout(200)
      expect(events).toEqual(['refresh-start'])
      release()
      await refresh
      await expect(page).toHaveURL(action === 'login' ? /\/home$/ : /\/login$/)
      expect(events).toEqual(['refresh-start', 'refresh-response', action + '-request'])
      const current = await page.evaluate(async () => (await import('/src/lib/auth.ts')).getToken())
      expect(current).toBe(action === 'login' ? replacement : '')
    } finally { release() }
  })
}
