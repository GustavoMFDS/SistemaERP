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
    await Promise.all([admin.goto('/login'), cashier.goto('/login')])
    await Promise.all([
      admin.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local'),
      cashier.getByLabel('E-mail ou nome de usuário').fill('caixa@sistema.local'),
    ])
    await Promise.all([
      admin.getByLabel('Senha').fill('admin123'),
      cashier.getByLabel('Senha').fill('admin123'),
    ])
    const adminIdentity = admin.waitForResponse((response) =>
      response.url().endsWith('/api/v1/auth/me') && response.status() === 200,
    )
    const cashierIdentity = cashier.waitForResponse((response) =>
      response.url().endsWith('/api/v1/auth/me') && response.status() === 200,
    )
    await Promise.all([
      admin.getByRole('button', { name: 'Entrar' }).click(),
      cashier.getByRole('button', { name: 'Entrar' }).click(),
    ])
    await Promise.all([
      expect(admin).toHaveURL(/\/home$/),
      expect(cashier).toHaveURL(/\/home$/),
    ])
    const [adminMe, cashierMe] = await Promise.all([
      adminIdentity.then((r) => r.json() as Promise<{ tenant_id: string; name: string; roles: string[] }>),
      cashierIdentity.then((r) => r.json() as Promise<{ tenant_id: string; name: string; roles: string[] }>),
    ])
    expect(adminMe.roles).toContain('admin')
    expect(cashierMe.roles).toContain('cashier')
    expect(cashierMe.roles).not.toContain('admin')
    expect(adminMe.tenant_id).toBe(adminMe.tenant_id)
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
