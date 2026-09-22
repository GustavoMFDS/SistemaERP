import { expect, test } from '@playwright/test'

test('concurrent 401 responses share one refresh request', async ({ page }) => {
  let challengeMode = false
  let refreshed = false
  let refreshCalls = 0
  let successfulRetries = 0

  await page.route('http://127.0.0.1:8080/api/v1/auth/login', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        token: { access_token: 'initial-access-token', token_type: 'Bearer', expires_in: 900 },
        user: { id: 'user-1', email: 'admin@sistema.local', name: 'Admin', roles: ['admin'] },
      }),
    })
  })

  await page.route('http://127.0.0.1:8080/api/v1/auth/me', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ id: 'user-1', email: 'admin@sistema.local', name: 'Admin', roles: ['admin'] }),
    })
  })

  await page.route('http://127.0.0.1:8080/api/v1/auth/refresh', async (route) => {
    refreshCalls += 1
    await new Promise((resolve) => setTimeout(resolve, 200))
    refreshed = true
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ access_token: 'fresh-access-token', token_type: 'Bearer', expires_in: 900 }),
    })
  })

  await page.route(/http:\/\/127\.0\.0\.1:8080\/api\/v1\/products.*/, async (route) => {
    const url = new URL(route.request().url())
    const isProbe = url.searchParams.has('probe')

    if (challengeMode && isProbe && !refreshed) {
      await route.fulfill({
        status: 401,
        contentType: 'application/json',
        body: JSON.stringify({ message: 'expired' }),
      })
      return
    }

    if (challengeMode && isProbe && refreshed) successfulRetries += 1
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ items: [], total: 0 }),
    })
  })

  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()

  await expect(page).toHaveURL(/\/products$/)
  await expect(page.getByRole('heading', { name: 'Produtos' })).toBeVisible()

  challengeMode = true
  refreshed = false
  refreshCalls = 0
  successfulRetries = 0

  const result = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    await Promise.all([
      apiJson('/api/v1/products?probe=one'),
      apiJson('/api/v1/products?probe=two'),
    ])
    return true
  })

  expect(result).toBe(true)
  expect(refreshCalls).toBe(1)
  expect(successfulRetries).toBe(2)
})
