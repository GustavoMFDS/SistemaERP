import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('administrador cadastra, pesquisa e edita cliente sem selecionar CNPJ', async ({ page }) => {
  const writes: Array<{ method: string; path: string; body: Record<string, unknown> }> = []
  let name = 'Maria Silva'
  await page.route('**/api/v1/customers?*', async (route) => {
    const params = new URL(route.request().url()).searchParams
    const matches = !params.get('q') || name.toLowerCase().includes(params.get('q')!.toLowerCase())
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        items: matches ? [{
          id: '00000000-0000-4000-8000-000000000100',
          name, email: 'maria@example.test', phone: '11990001122',
        }] : [], total: matches ? 1 : 0, limit: 20, offset: 0,
      }),
    })
  })
  await page.route('**/api/v1/customers', async (route) => {
    const body = route.request().postDataJSON() as Record<string, unknown>
    writes.push({ method: route.request().method(), path: route.request().url(), body })
    await route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify({
      id: '00000000-0000-4000-8000-000000000200', ...body,
    }) })
  })
  await page.route('**/api/v1/customers/00000000-0000-4000-8000-000000000100', async (route) => {
    const body = route.request().postDataJSON() as Record<string, unknown>
    writes.push({ method: route.request().method(), path: route.request().url(), body })
    name = String(body.name)
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({
      id: '00000000-0000-4000-8000-000000000100', ...body,
    }) })
  })
  await login(page, 'admin@sistema.local')
  await page.getByRole('link', { name: 'Clientes', exact: true }).click()
  await expect(page).toHaveURL(/\/customers$/)
  const form = page.getByRole('region', { name: 'Cadastro de cliente' })
  await form.getByLabel('Nome').fill('Joana Silva')
  await form.getByLabel('E-mail (opcional)').fill('joana@example.test')
  await form.getByLabel('Telefone (opcional)').fill('11999998888')
  await form.getByRole('button', { name: 'Cadastrar cliente' }).click()
  await expect(page.getByText(/Cliente cadastrado nesta loja/)).toBeVisible()
  await expect(page.getByRole('region', { name: 'Lista de clientes' }).getByText('Maria Silva')).toBeVisible()
  await page.getByRole('button', { name: 'Editar' }).click()
  await form.getByLabel('Nome').fill('Maria Atualizada')
  await form.getByRole('button', { name: 'Salvar alterações' }).click()
  await expect(page.getByRole('region', { name: 'Lista de clientes' }).getByText('Maria Atualizada')).toBeVisible()
  expect(writes.map((item) => item.method)).toEqual(['POST', 'PUT'])
  for (const entry of writes) {
    expect(entry.path).not.toContain('tenant_id')
    expect(entry.body).not.toHaveProperty('tenant_id')
    expect(entry.body).not.toHaveProperty('document')
  }
})

test('operador sem permissão não pode acessar dados pessoais dos clientes', async ({ page }) => {
  await login(page, 'caixa@sistema.local')
  await expect(page.getByRole('link', { name: 'Clientes', exact: true })).toHaveCount(0)
  await page.goto('/customers')
  await expect(page.getByText(/Sem permissão para consultar/)).toBeVisible()
  const results = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    const check = async (path: string) => {
      try {
        await apiJson(path)
        return 200
      } catch (err) {
        return err instanceof APIError ? err.status : 0
      }
    }
    return [
      await check('/api/v1/customers?limit=20'),
      await check('/api/v1/customers?tenant_id=not-current'),
    ]
  })
  expect(results).toEqual([403, 403])
})

test('consulta de clientes recusa paginação inválida e seleção de outro CNPJ', async ({ page }) => {
  await login(page, 'gerente@sistema.local')
  const results = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    const check = async (query: string) => {
      try {
        await apiJson('/api/v1/customers?' + query)
        return 200
      } catch (err) {
        return err instanceof APIError ? err.status : 0
      }
    }
    return [
      await check('limit=51'),
      await check('offset=-1'),
      await check('limit=20&limit=10'),
      await check('tenant_id=another-company'),
      await check('q=' + 'x'.repeat(101)),
    ]
  })
  expect(results).toEqual([422, 422, 422, 422, 422])
})
