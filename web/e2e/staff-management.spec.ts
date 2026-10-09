import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email = 'admin@sistema.local') {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('administrador convida, altera papel e suspende funcionário somente da loja', async ({ page }) => {
  const sent: Array<{ method: string; url: string; payload: Record<string, unknown> | null }> = []
  let role = 'cashier'
  let active = true
  await page.route('**/api/v1/staff', async (route) => {
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        members: [
          { id: '00000000-0000-4000-8000-000000000001',
            name: 'Admin', email: 'admin@sistema.local', role: 'admin', active: true },
          { id: '00000000-0000-4000-8000-000000000002',
            name: 'Maria Loja', email: 'maria@example.test', role, active },
        ],
        invitations: [],
      }),
    })
  })
  await page.route('**/api/v1/staff/invitations', async (route) => {
    const body = route.request().postDataJSON() as Record<string, unknown>
    sent.push({ method: route.request().method(), url: route.request().url(), payload: body })
    await route.fulfill({
      status: 201, contentType: 'application/json',
      body: JSON.stringify({
        invitation: {
          id: '00000000-0000-4000-8000-000000000003',
          name: body.name, email: body.email, role: body.role, expires_at: '2026-10-10T20:00:00Z',
        },
        token: '1'.repeat(64),
      }),
    })
  })
  await page.route('**/api/v1/staff/00000000-0000-4000-8000-000000000002/*', async (route) => {
    const body = route.request().postDataJSON() as Record<string, unknown>
    sent.push({ method: route.request().method(), url: route.request().url(), payload: body })
    if (route.request().url().endsWith('/role')) role = body.role as string
    if (route.request().url().endsWith('/status')) active = body.active as boolean
    await route.fulfill({ status: 204, body: '' })
  })
  await login(page)
  await page.getByRole('link', { name: 'Funcionários', exact: true }).click()
  await expect(page).toHaveURL(/\/staff$/)
  const form = page.locator('details[aria-label="Adicionar novo acesso"]')
  await form.locator('summary').click()
  await form.getByLabel('Nome completo').fill('Pedro Novo')
  await form.getByLabel('E-mail do funcionário').fill('pedro@example.test')
  await form.getByLabel('Função inicial').selectOption('cashier')
  await form.getByRole('button', { name: 'Criar acesso individual' }).click()
  await expect(form.getByLabel('Link de ativação')).toHaveValue(/\/accept-invite#token=1{64}$/)
  expect(sent[0].payload).toMatchObject({
    name: 'Pedro Novo', email: 'pedro@example.test', role: 'cashier',
  })
  expect(sent[0].payload).not.toHaveProperty('tenant_id')
  const table = page.getByRole('region', { name: 'Equipe da loja' })
  const owner = table.getByRole('row', { name: /admin@sistema.local/ })
  await expect(owner.getByText('Protegido')).toBeVisible()
  const employee = table.getByRole('row', { name: /maria@example.test/ })
  page.once('dialog', (dialog) => void dialog.accept())
  await employee.getByRole('button', { name: 'Mudar para gerente' }).click()
  await expect(employee.getByText('Gerente')).toBeVisible()
  page.once('dialog', (dialog) => void dialog.accept())
  await employee.getByRole('button', { name: 'Suspender acesso' }).click()
  await expect(employee.getByText('Sem acesso nesta loja')).toBeVisible()
  expect(sent.slice(1).map((item) => item.payload)).toEqual([
    { role: 'manager' }, { active: false },
  ])
})

test('ativação remove o segredo da URL e envia token apenas em POST', async ({ page }) => {
  const secret = '2'.repeat(64)
  let posted: Record<string, unknown> | null = null
  await page.route('**/api/v1/staff/accept-invite', async (route) => {
    posted = route.request().postDataJSON() as Record<string, unknown>
    await route.fulfill({
      status: 201, contentType: 'application/json', body: '{"status":"registered"}',
    })
  })
  await page.goto('/accept-invite#token=' + secret)
  await expect(page).toHaveURL(/\/accept-invite$/)
  await page.getByLabel('Criar senha (12 a 72 caracteres)').fill('MinhaSenhaUnica2026!')
  await page.getByLabel('Confirmar nova senha').fill('MinhaSenhaUnica2026!')
  await page.getByRole('button', { name: 'Ativar minha conta' }).click()
  await expect(page.getByText(/Conta ativada/)).toBeVisible()
  expect(posted).toEqual({ token: secret, password: 'MinhaSenhaUnica2026!' })
  expect(page.url()).not.toContain(secret)
})

test('operador de caixa não acessa administração da equipe', async ({ page }) => {
  await login(page, 'caixa@sistema.local')
  await expect(page.getByRole('link', { name: 'Funcionários', exact: true })).toHaveCount(0)
  await page.goto('/staff')
  await expect(page.getByText(/não pode administrar funcionários/)).toBeVisible()
  const results = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    const status = async (path: string, init?: { method: string; body: unknown }) => {
      try {
        await apiJson(path, init)
        return 200
      } catch (error) {
        return error instanceof APIError ? error.status : 0
      }
    }
    return [await status('/api/v1/staff'), await status('/api/v1/staff/invitations', { method: 'POST', body: { name: 'Teste', email: 'test@example.test', role: 'cashier' } })]
  })
  expect(results).toEqual([403, 403])
})
