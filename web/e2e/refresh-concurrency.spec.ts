import { expect, test } from '@playwright/test'

test('concurrent 401 responses share one refresh request', async ({ page }) => {
  let refreshed = false
  let refreshCalls = 0
  let meSuccesses = 0
  let productSuccesses = 0

  await page.route('http://127.0.0.1:8080/api/v1/auth/login', async (route) => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        token: { access_token: 'expired-access-token', token_type: 'Bearer', expires_in: 1 },
        user: { id: 'user-1', email: 'admin@sistema.local', name: 'Admin', roles: ['admin'] },
      }),
    })
  })

  await page.route('http://127.0.0.1:8080/api/v1/auth/refresh', async (route) => {
    refreshCalls += 1
    await new Promise((resolve) => setTimeout(resolve, 150))
    refreshed = true
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ access_token: 'fresh-access-token', token_type: 'Bearer', expires_in: 900 }),
    })
  })

  await page.route('http://127.0.0.1:8080/api/v1/auth/me', async (route) => {
    if (!refreshed) {
      await route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ message: 'expired' }) })
      return
    }
    meSuccesses += 1
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ id: 'user-1', email: 'admin@sistema.local', name: 'Admin', roles: ['admin'] }),
    })
  })

  await page.route(/http:\/\/127\.0\.0\.1:8080\/api\/v1\/products.*/, async (route) => {
    if (!refreshed) {
      await route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ message: 'expired' }) })
      return
    }
    productSuccesses += 1
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
  await expect.poll(() => refreshCalls).toBe(1)
  await expect.poll(() => meSuccesses).toBeGreaterThan(0)
  await expect.poll(() => productSuccesses).toBeGreaterThan(0)
})
