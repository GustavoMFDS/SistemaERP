import { expect, test } from '@playwright/test'

test('compiled ERP interface communicates with the bundled Go API', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', (error) => errors.push(error.message))

  await page.goto('/login')
  await expect(page.getByRole('heading', { name: 'Bem-vindo de volta' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Entrar' })).toBeEnabled()

  const ready = await page.evaluate(async () => {
    const result = await fetch('/health/ready')
    return { status: result.status, sameOrigin: new URL(result.url).origin === location.origin }
  })
  expect(ready.status).toBe(200)
  expect(ready.sameOrigin).toBe(true)

  await page.getByLabel('E-mail ou nome de usuário').fill('ci-nonexistent-user')
  await page.getByLabel('Senha').fill('ci-incorrect-password')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page.getByRole('alert')).toContainText('Não foi possível entrar')

  expect(errors).toEqual([])
})


test('two independent terminals keep administrator and cashier sessions separate', async ({ browser }) => {
  const origin = 'http://127.0.0.1:18080'
  const adminContext = await browser.newContext({ baseURL: origin })
  const cashierContext = await browser.newContext({ baseURL: origin })
  try {
    const admin = await adminContext.newPage()
    const cashier = await cashierContext.newPage()
    const signIn = async (page: typeof admin, username: string) => {
      await page.goto('/login')
      await page.getByLabel('E-mail ou nome de usuário').fill(username)
      await page.getByLabel('Senha').fill('admin123')
      const loginResponse = page.waitForResponse((response) =>
        response.url().endsWith('/api/v1/auth/login') && response.request().method() === 'POST',
      )
      const identityResponse = page.waitForResponse((response) =>
        response.url().endsWith('/api/v1/auth/me') && response.status() === 200,
      )
      await page.getByRole('button', { name: 'Entrar' }).click()
      const response = await loginResponse
      expect(response.status(), `login rejected for ${username}`).toBe(200)
      await expect(page).toHaveURL(/\\/home$/, { timeout: 10000 })
      return await (await identityResponse).json() as {
        tenant_id: string
        name: string
        roles: string[]
      }
    }
    // Two distinct browser contexts, authenticated independently. Sequential
    // login distinguishes permission problems from request-rate races.
    const adminMe = await signIn(admin, 'admin@sistema.local')
    const cashierMe = await signIn(cashier, 'caixa@sistema.local')
    expect(adminMe.roles).toContain('admin')
    expect(cashierMe.roles).toContain('cashier')
    expect(cashierMe.roles).not.toContain('admin')
    expect(adminMe.tenant_id).toBe(cashierMe.tenant_id)
    expect(adminMe.name).not.toBe(cashierMe.name)

    // Closing one terminal must not terminate the other browser session.
    await adminContext.close()
    await expect(cashier).toHaveURL(/\/home$/)
    await expect(cashier.getByRole('navigation', { name: 'Navegação principal' })).toBeVisible()
  } finally {
    await adminContext.close().catch(() => {})
    await cashierContext.close()
  }
})
