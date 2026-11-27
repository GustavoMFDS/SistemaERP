import { expect, test } from '@playwright/test'

function fixtureToken(sub: string, tenant_id: string, jti: string) {
  return 'fixture.' + Buffer.from(JSON.stringify({
    sub, tenant_id, jti, exp: Math.floor(Date.now() / 1000) + 900,
  })).toString('base64url') + '.fixture'
}

for (const action of ['login', 'logout'] as const) {
  for (const outcome of ['success', 'failure'] as const) {
    test('late refresh ' + outcome + ' does not supersede ' + action, async ({ page }) => {
      const initial = fixtureToken('a', 'tenant-a', 'initial')
      const renewed = fixtureToken('a', 'tenant-a', 'renewed')
      const replacement = fixtureToken('b', 'tenant-b', 'login')
      let started = false
      let release!: () => void
      const pending = new Promise<void>((resolve) => { release = resolve })
      await page.route('**/api/v1/**', async (route) => {
        if (new URL(route.request().url()).pathname.endsWith('/auth/refresh')) {
          started = true
          await pending
          return route.fulfill(outcome === 'success'
            ? { json: { access_token: renewed } }
            : { status: 401, json: { message: 'expired' } })
        }
        return route.fulfill({ json: {} })
      })
      try {
        await page.goto('/login')
        const refresh = page.evaluate(async (initial) => {
          const auth = await import('/src/lib/auth.ts')
          const api = await import('/src/lib/api.ts')
          auth.setToken(initial)
          return api.refreshAccessToken()
        }, initial)
        await expect.poll(() => started).toBe(true)
        await page.evaluate(async ({ action, replacement }) => {
          const auth = await import('/src/lib/auth.ts')
          if (action === 'login') auth.setToken(replacement)
          else auth.clearToken()
        }, { action, replacement })
        release()
        expect(await refresh).toBe(false)
        const current = await page.evaluate(async () => (await import('/src/lib/auth.ts')).getToken())
        expect(current).toBe(action === 'login' ? replacement : '')
      } finally { release() }
    })
  }
}

for (const operation of ['download', 'print'] as const) {
  for (const mode of ['same-scope', 'changed-scope'] as const) {
    test(operation + ' retry: ' + mode, async ({ page }) => {
      const initial = fixtureToken('a', 'tenant-a', 'initial')
      const renewed = mode === 'same-scope' ? fixtureToken('a', 'tenant-a', 'renewed') : fixtureToken('b', 'tenant-b', 'other')
      const headers: (string | undefined)[] = []
      let refreshes = 0
      await page.route('**/api/v1/**', async (route) => {
        const path = new URL(route.request().url()).pathname
        if (path.endsWith('/auth/refresh')) {
          refreshes++
          return route.fulfill({ json: { access_token: renewed } })
        }
        if (path.endsWith('/g4-document')) {
          headers.push(route.request().headers().authorization)
          if (headers.length === 1) return route.fulfill({ status: 401, json: { message: 'expired' } })
          return route.fulfill({ contentType: 'text/html', body: '<html><body>LAB document</body></html>' })
        }
        return route.fulfill({ json: {} })
      })
      await page.goto('/login')
      const result = await page.evaluate(async ({ initial, operation }) => {
        const auth = await import('/src/lib/auth.ts')
        const api = await import('/src/lib/api.ts')
        auth.setToken(initial)
        // Observe side effects without opening an OS print dialog or writing a file.
        let downloads = 0
        let printable = 0
        let closed = 0
        const originalClick = HTMLAnchorElement.prototype.click
        const originalOpen = window.open
        HTMLAnchorElement.prototype.click = function () { downloads++ }
        window.open = (() => ({
          opener: null, location: { replace: () => { printable++ } },
          close: () => { closed++ },
        })) as unknown as typeof window.open
        try {
          let completed = true
          try {
            if (operation === 'download') await api.apiDownload('/api/v1/g4-document', 'lab.html', 'text/html')
            else await api.apiOpenPrintable('/api/v1/g4-document')
          } catch { completed = false }
          return { completed, downloads, printable, closed, token: auth.getToken() }
        } finally {
          HTMLAnchorElement.prototype.click = originalClick
          window.open = originalOpen
        }
      }, { initial, operation })
      expect(refreshes).toBe(1)
      expect(headers).toEqual(mode === 'same-scope' ? ['Bearer ' + initial, 'Bearer ' + renewed] : ['Bearer ' + initial])
      expect(result).toEqual({
        completed: mode === 'same-scope',
        downloads: operation === 'download' && mode === 'same-scope' ? 1 : 0,
        printable: operation === 'print' && mode === 'same-scope' ? 1 : 0,
        closed: operation === 'print' && mode === 'changed-scope' ? 1 : 0,
        token: mode === 'same-scope' ? renewed : '',
      })
    })
  }
}
