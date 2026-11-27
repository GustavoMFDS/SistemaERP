import { expect, test } from '@playwright/test'

const fixture = (sub: string, tenant_id: string, jti: string) => 'fixture.' + Buffer.from(JSON.stringify({
  sub, tenant_id, jti, exp: Math.floor(Date.now() / 1000) + 900,
})).toString('base64url') + '.fixture'

for (const action of ['login', 'logout'] as const) {
  test('shared browser session orders refresh and ' + action + ' across tabs', async ({ context, page }) => {
    const initial = fixture('a', 'tenant-a', 'initial')
    const renewed = fixture('a', 'tenant-a', 'renewed')
    const replacement = fixture('b', 'tenant-b', 'login')
    const events: string[] = []
    let hold = false
    let release!: () => void
    const pending = new Promise<void>((resolve) => { release = resolve })
    await context.route('**/api/v1/**', async (route) => {
      const path = new URL(route.request().url()).pathname
      if (path.endsWith('/auth/refresh')) {
        if (!hold) return route.fulfill({ json: { access_token: initial }, headers: {
          'set-cookie': 'lab_session=initial; Path=/; HttpOnly; SameSite=Strict',
        } })
        events.push('refresh-start')
        await pending
        events.push('refresh-response')
        return route.fulfill({ json: { access_token: renewed }, headers: {
          'set-cookie': 'lab_session=renewed; Path=/; HttpOnly; SameSite=Strict',
        } })
      }
      if (path.endsWith('/auth/login')) {
        events.push('login-request')
        return route.fulfill({ json: { token: { access_token: replacement }, user: { id: 'b', name: 'Lab B', roles: ['admin'] } }, headers: {
          'set-cookie': 'lab_session=replacement; Path=/; HttpOnly; SameSite=Strict',
        } })
      }
      if (path.endsWith('/auth/logout')) {
        events.push('logout-request')
        return route.fulfill({ json: {}, headers: {
          'set-cookie': 'lab_session=; Path=/; HttpOnly; SameSite=Strict; Max-Age=0',
        } })
      }
      if (path.endsWith('/auth/me')) return route.fulfill({ json: {
        id: 'a', name: 'Lab A', roles: ['admin'], permissions: [],
      } })
      return route.fulfill({ json: { items: [], total: 0 } })
    })
    const other = await context.newPage()
    try {
      await page.goto('/setup')
      await expect(page.getByRole('button', { name: 'Sair' })).toBeVisible()
      await other.goto(action === 'login' ? '/login' : '/setup')
      if (action === 'logout') await expect(other.getByRole('button', { name: 'Sair' })).toBeVisible()
      hold = true
      const refreshing = page.evaluate(async () => (await import('/src/lib/api.ts')).refreshAccessToken())
      await expect.poll(() => events).toEqual(['refresh-start'])
      if (action === 'login') {
        await other.getByLabel(/E-mail/).fill('lab-b@example.invalid')
        await other.getByLabel('Senha', { exact: true }).fill('fixture-only-password')
        await other.getByRole('button', { name: 'Entrar', exact: true }).click()
      } else {
        await other.getByRole('button', { name: 'Sair', exact: true }).click()
      }
      await other.waitForTimeout(250)
      expect(events).toEqual(['refresh-start'])
      release()
      await refreshing
      await expect(other).toHaveURL(action === 'login' ? /\/home$/ : /\/login$/)
      await expect(page).toHaveURL(/\/login$/)
      expect(events).toEqual(['refresh-start', 'refresh-response', action + '-request'])
      const cookie = (await context.cookies()).find((item) => item.name === 'lab_session')
      expect(cookie?.value).toBe(action === 'login' ? 'replacement' : undefined)
      if (cookie) expect(cookie.httpOnly).toBe(true)
      expect(await page.evaluate(async () => (await import('/src/lib/auth.ts')).getToken())).toBe('')
    } finally { release() }
  })
}
