import { expect, test } from '@playwright/test'
import type { Page } from '@playwright/test'

const userID = '6c285ff8-ac36-4d48-b77a-1e2af5c3130a'
const tenantID = '1d36e3cb-9f34-4b67-a6bb-6c9e5c53bd98'

async function mockAccount(page: Page, admin: boolean, readiness: 'pending' | 'ready' | 'unavailable') {
  const token = 'e30.' + Buffer.from(JSON.stringify({
    sub: userID, tenant_id: tenantID, exp: Math.floor(Date.now() / 1000) + 3600,
  })).toString('base64url') + '.signature'
  const role = admin ? 'admin' : 'cashier'
  let checks = 0

  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const method = route.request().method()
    const reply = (data: unknown, status = 200) => route.fulfill({ status, json: data })
    if (path === '/api/v1/auth/login' && method === 'POST') {
      return reply({
        token: { access_token: token },
        user: { id: userID, tenant_id: tenantID, name: 'Operador de teste', roles: [role] },
      })
    }
    if (path === '/api/v1/auth/refresh') return reply({ access_token: token })
    if (path === '/api/v1/auth/me') {
      return reply({
        id: userID, tenant_id: tenantID, name: 'Operador de teste',
        roles: [role], permissions: ['invoice:read'],
      })
    }
    if (path === '/api/v1/fiscal/nfce/readiness') {
      checks += 1
      if (readiness === 'unavailable') return reply({ message: 'erro interno' }, 503)
      const ready = readiness === 'ready'
      return reply({
        tenant_id: tenantID,
        issuer_identity_configured: ready,
        issuer_address_configured: ready,
        municipality_code_configured: ready,
        config_exists: ready,
        certificate_reference_configured: ready,
        ready_for_homologation_data: ready,
        transmission_enabled: false,
        active_products: 0,
        products_missing_ncm: 0,
        blocking_reasons: ready ? [] : ['Faltam dados para preparar a emissão'],
      })
    }
    return reply({ message: 'indisponível neste teste' }, 404)
  })

  await page.goto('/login')
  await page.getByLabel('E-mail').fill('teste@loja.local')
  await page.getByLabel('Senha', { exact: true }).fill('senha-ficticia')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)
  await expect(page.getByRole('heading', { name: /Sua loja está em movimento/ })).toBeVisible()
  return { calls: () => checks }
}

test('administrador é convidado a terminar dados fiscais somente quando faltam informações', async ({ page }) => {
  const probe = await mockAccount(page, true, 'pending')
  const reminder = page.getByRole('dialog', { name: 'Sua configuração fiscal ainda não está concluída' })
  await expect(reminder).toBeVisible()
  await expect(reminder).toContainText('não significa que a SEFAZ autorizou')
  await expect.poll(probe.calls).toBe(1)
  await reminder.getByRole('button', { name: 'Agora não' }).click()
  await expect(reminder).toHaveCount(0)
  await page.reload()
  await expect(page.getByRole('heading', { name: /Sua loja está em movimento/ })).toBeVisible()
  await expect(reminder).toHaveCount(0)
  expect(probe.calls()).toBe(1)
})

test('dados fiscais preparados não provocam lembrete de configuração incompleta', async ({ page }) => {
  const probe = await mockAccount(page, true, 'ready')
  await expect.poll(probe.calls).toBe(1)
  await expect(page.getByRole('dialog')).toHaveCount(0)
})

test('funcionário sem perfil de administrador não recebe lembrete fiscal', async ({ page }) => {
  const probe = await mockAccount(page, false, 'pending')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  expect(probe.calls()).toBe(0)
})

test('falha na consulta fiscal não é confundida com configuração incompleta', async ({ page }) => {
  const probe = await mockAccount(page, true, 'unavailable')
  await expect.poll(probe.calls).toBe(1)
  await expect(page.getByRole('dialog')).toHaveCount(0)
})
