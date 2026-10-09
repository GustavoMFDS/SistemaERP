import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

function receipt(number: number, kind: 'products' | 'opening-stock', name: string) {
  return {
    batch_id: '00000000-0000-4000-8000-' + String(number).padStart(12, '0'),
    kind,
    actor_name: name,
    item_count: number,
    created_at: new Date(Date.UTC(2026, 9, 8, 21, 0, number)).toISOString(),
  }
}

test('menu abre visão cronológica com lotes dos dois módulos e paginação global', async ({ page }) => {
  const offsets: number[] = []
  await page.route('**/api/v1/imports/history?*', async (route) => {
    const url = new URL(route.request().url())
    const offset = Number(url.searchParams.get('offset'))
    offsets.push(offset)
    const items = offset === 0
      ? [
        receipt(10, 'opening-stock', 'Equipe estoque'),
        receipt(9, 'products', 'Gerência'),
        ...Array.from({ length: 8 }, (_, i) =>
          receipt(8 - i, i % 2 ? 'opening-stock' : 'products', 'Importador')),
      ]
      : [receipt(11, 'products', 'Operação anterior')]
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ items, limit: 10, offset, has_more: offset === 0 }),
    })
  })
  await login(page, 'gerente@sistema.local')
  await page.getByRole('link', { name: 'Histórico de importações' }).click()
  await expect(page).toHaveURL(/\/imports$/)
  const table = page.getByRole('table')
  await expect(table.getByText('Equipe estoque')).toBeVisible()
  await expect(table.getByText('Gerência')).toBeVisible()
  await expect(table.getByText('Cadastro de produtos').first()).toBeVisible()
  await expect(table.getByText('Estoque inicial').first()).toBeVisible()
  const rows = table.locator('tbody tr')
  await expect(rows).toHaveCount(10)
  await expect(rows.first()).toContainText('Equipe estoque')
  await page.getByRole('button', { name: 'Próxima página' }).click()
  await expect(table.getByText('Operação anterior')).toBeVisible()
  await expect(page.getByText('Página 2')).toBeVisible()
  await page.getByRole('button', { name: 'Página anterior' }).click()
  await expect(rows).toHaveCount(10)
  expect(offsets).toContain(0)
  expect(offsets).toContain(10)
})

test('filtros aplicam data de Brasília e reiniciam a página', async ({ page }) => {
  const queries: string[] = []
  await page.route('**/api/v1/imports/history?*', async (route) => {
    queries.push(new URL(route.request().url()).search)
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        items: [receipt(17, 'products', 'Responsável')],
        limit: 10, offset: 0, has_more: false,
      }),
    })
  })
  await login(page, 'gerente@sistema.local')
  await page.goto('/imports')
  await expect(page.getByRole('table').getByRole('cell', { name: 'Responsável' })).toBeVisible()
  const form = page.getByRole('region', { name: 'Filtros do histórico administrativo' })
  await form.getByLabel('Data inicial').fill('2026-10-01')
  await form.getByLabel('Data final').fill('2026-10-08')
  await form.getByRole('button', { name: 'Filtrar período' }).click()
  await expect.poll(() => queries.some(q =>
    q.includes('from=2026-10-01') && q.includes('to=2026-10-08') && q.includes('offset=0'),
  )).toBe(true)
  await form.getByLabel('Data final').fill('2026-09-30')
  await expect(form.getByRole('button', { name: 'Filtrar período' })).toBeDisabled()
  await expect(form.getByText(/período deve estar em ordem/)).toBeVisible()
  await form.getByRole('button', { name: 'Limpar filtros' }).click()
  await expect.poll(() => queries.some(q => !q.includes('from=') && q.includes('offset=0'))).toBe(true)
})

test('caixa não vê o link, não consegue abrir o painel e recebe 403 da API', async ({ page }) => {
  await login(page, 'caixa@sistema.local')
  await expect(page.getByRole('link', { name: 'Histórico de importações' })).toHaveCount(0)
  const response = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    try {
      await apiJson('/api/v1/imports/history?limit=10&offset=0')
      return 200
    } catch (error) {
      return error instanceof APIError ? error.status : 0
    }
  })
  expect(response).toBe(403)
  await page.goto('/imports')
  await expect(page.getByText(/não tem permissão para consultar nenhum dos históricos/)).toBeVisible()
  await expect(page.getByRole('table')).toHaveCount(0)
})

test('API da visão unificada recusa filtros inválidos', async ({ page }) => {
  await login(page, 'gerente@sistema.local')
  const result = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    const getStatus = async (query: string) => {
      try {
        await apiJson('/api/v1/imports/history?' + query)
        return 200
      } catch (error) {
        return error instanceof APIError ? error.status : 0
      }
    }
    return [
      await getStatus('limit=51'),
      await getStatus('offset=-1'),
      await getStatus('from=2026-02-30'),
      await getStatus('from=2026-10-09&to=2026-10-08'),
      await getStatus('from=2026-10-08&from=2026-10-07'),
      await getStatus('offset=5001'),
    ]
  })
  expect(result).toEqual([422, 422, 422, 422, 422, 422])
})
