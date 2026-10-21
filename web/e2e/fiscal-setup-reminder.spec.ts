import { expect, test } from '@playwright/test'

type Setup = {
  role: 'admin' | 'manager' | 'cashier'
  ready?: boolean
  readinessError?: boolean
  wrongTenant?: boolean
}

const userId = 'b5c00852-c6d7-43e2-a72b-094e5c4f92b7'
const tenantId = 'ced80ebd-caba-4ec3-8cb0-55735026e144'
const token = 'e30.' + Buffer.from(JSON.stringify({
  sub: userId, tenant_id: tenantId,
  exp: Math.floor(Date.now() / 1000) + 3600,
})).toString('base64url') + '.mock-signature'

async function mockSetup(page: import('@playwright/test').Page, setup: Setup) {
  let readinessCalls = 0
  await page.route('**/api/v1/**', (route) => {
    const path = new URL(route.request().url()).pathname
    const reply = (json: unknown, status = 200) => route.fulfill({ status, json })
    if (path === '/api/v1/auth/login') return reply({
      token: { access_token: token }, user: { id: userId, email: 'teste@loja.local', name: 'Operador de teste', roles: [setup.role] },
    })
    if (path === '/api/v1/auth/me') return reply({
      id: userId, tenant_id: tenantId, email: 'teste@loja.local', name: 'Operador de teste',
      roles: [setup.role], permissions: ['product:read', 'invoice:read'],
    })
    if (path === '/api/v1/auth/refresh') return reply({ access_token: token })
    if (path === '/api/v1/auth/logout') return reply({ ok: true })
    if (path === '/api/v1/fiscal/nfce/readiness') {
      readinessCalls += 1
      if (setup.readinessError) return reply({ message: 'unavailable' }, 503)
      return reply({
        tenant_id: setup.wrongTenant ? 'other-company' : tenantId,
        ready_for_homologation_data: setup.ready ?? false,
        blocking_reasons: ['issuer_identity'],
      })
    }
    if (path === '/api/v1/fiscal/obligations') return reply({ items: [], total: 0 })
    if (path === '/api/v1/products') return reply({ items: [], total: 0 })
    return reply({ items: [], total: 0 })
  })
  return { calls: () => readinessCalls }
}

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('teste@loja.local')
  await page.getByLabel('Senha', { exact: true }).fill('senha-teste')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)
}

test('somente administrador com preparação incompleta vê o lembrete uma vez por login', async ({ page }) => {
  const fixture = await mockSetup(page, { role: 'admin', ready: false })
  await login(page)
  const dialog = page.getByRole('dialog', { name: 'Sua configuração fiscal ainda não está concluída' })
  await expect(dialog).toBeVisible()
  await expect(dialog).toContainText('não significa que a SEFAZ autorizou')
  await expect(page.getByRole('button', { name: 'Continuar configuração' })).toBeFocused()
  await dialog.getByRole('button', { name: 'Agora não' }).click()
  await expect(dialog).toHaveCount(0)
  await page.getByRole('navigation', { name: 'Navegação principal' }).getByRole('link', { name: 'Produtos' }).click()
  await page.getByRole('navigation', { name: 'Navegação principal' }).getByRole('link', { name: 'Início' }).click()
  await expect(dialog).toHaveCount(0)
  const firstCount = fixture.calls()
  await page.reload()
  await expect(page).toHaveURL(/\/home$/)
  await expect(page.getByRole('heading', { name: /Sua loja está em movimento/ })).toBeVisible()
  await expect(dialog).toHaveCount(0)
  expect(fixture.calls()).toBe(firstCount)

  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/login$/)
  await login(page)
  await expect(dialog).toBeVisible()
  await dialog.getByRole('button', { name: 'Continuar configuração' }).click()
  await expect(page).toHaveURL(/\/fiscal$/)
})

test('dados fiscais completos não exibem lembrete, mesmo sem homologação', async ({ page }) => {
  const fixture = await mockSetup(page, { role: 'admin', ready: true })
  await login(page)
  await expect.poll(fixture.calls).toBe(1)
  await expect(page.getByRole('dialog')).toHaveCount(0)
})

test('gerente não recebe lembrete fiscal mesmo com permissão de leitura', async ({ page }) => {
  const fixture = await mockSetup(page, { role: 'manager', ready: false })
  await login(page)
  await expect(page.getByRole('heading', { name: /Sua loja está em movimento/ })).toBeVisible()
  expect(fixture.calls()).toBe(0)
  await expect(page.getByRole('dialog')).toHaveCount(0)
})

test('resultado de prontidão indisponível ou de outra empresa nunca aciona modal', async ({ page }) => {
  const fixture = await mockSetup(page, { role: 'admin', readinessError: true })
  await login(page)
  await expect.poll(fixture.calls).toBe(1)
  await expect(page.getByRole('dialog')).toHaveCount(0)
})

test('prontidão de outro CNPJ não gera lembrete para esta loja', async ({ page }) => {
  const fixture = await mockSetup(page, { role: 'admin', wrongTenant: true })
  await login(page)
  await expect.poll(fixture.calls).toBe(1)
  await expect(page.getByRole('dialog')).toHaveCount(0)
})
