import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('filtro de período e CSV usam as mesmas datas sem mudar o CNPJ', async ({ page }) => {
  const queries: string[] = []
  const csvQueries: string[] = []
  await page.route('**/api/v1/products/import-batches/history?*', async (route) => {
    const url = new URL(route.request().url())
    queries.push(url.search)
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({
        items: [{
          batch_id: '00000000-0000-4000-8000-000000000200',
          item_count: 3, actor_name: 'Gestora',
          created_at: '2026-10-08T21:00:00Z',
        }],
        limit: 10, offset: 0, has_more: false,
      }),
    })
  })
  await page.route('**/api/v1/products/import-batches/history/export.csv?*', (route) => {
    csvQueries.push(new URL(route.request().url()).search)
    return route.fulfill({
      status: 200,
      contentType: 'text/csv;charset=utf-8',
      headers: { 'Content-Disposition': 'attachment; filename="historico-importacao-produtos.csv"' },
      body: '\uFEFFData (UTC);Responsável;Produtos;Identificador do lote\n2026-10-08T21:00:00Z;Gestora;3;00000000-0000-4000-8000-000000000200\n',
    })
  })
  await login(page, 'gerente@sistema.local')
  const history = page.getByRole('region', { name: 'Histórico de importações confirmadas' })
  await expect(history.getByText('Gestora')).toBeVisible()
  await history.getByLabel('Data inicial').fill('2026-10-01')
  await history.getByLabel('Data final').fill('2026-10-08')
  await expect(history.getByRole('button', { name: 'Baixar histórico CSV' })).toBeDisabled()
  await history.getByRole('button', { name: 'Filtrar período' }).click()
  await expect.poll(() => queries.some((query) => query.includes('from=2026-10-01') && query.includes('to=2026-10-08'))).toBe(true)
  await expect(history.getByRole('button', { name: 'Baixar histórico CSV' })).toBeEnabled()
  const downloadEvent = page.waitForEvent('download')
  await history.getByRole('button', { name: 'Baixar histórico CSV' }).click()
  const download = await downloadEvent
  expect(download.suggestedFilename()).toBe('historico-importacao-produtos.csv')
  expect(csvQueries).toHaveLength(1)
  expect(csvQueries[0]).toContain('from=2026-10-01')
  expect(csvQueries[0]).toContain('to=2026-10-08')
  await history.getByRole('button', { name: 'Limpar filtros' }).click()
  await expect.poll(() => queries.some((query) => query.includes('offset=0') && !query.includes('from='))).toBe(true)
})

test('interface impede período invertido ou acima de 365 dias', async ({ page }) => {
  await page.route('**/api/v1/products/import-batches/history?*', (route) =>
    route.fulfill({
      status: 200, contentType: 'application/json',
      body: '{"items":[],"limit":10,"offset":0,"has_more":false}',
    }))
  await login(page, 'gerente@sistema.local')
  const history = page.getByRole('region', { name: 'Histórico de importações confirmadas' })
  await history.getByLabel('Data inicial').fill('2026-10-09')
  await history.getByLabel('Data final').fill('2026-10-08')
  await expect(history.getByRole('button', { name: 'Filtrar período' })).toBeDisabled()
  await expect(history.getByRole('button', { name: 'Baixar histórico CSV' })).toBeDisabled()
  await history.getByLabel('Data final').fill('2028-10-08')
  await expect(history.getByText(/intervalo máximo de 365 dias/)).toBeVisible()
  await history.getByRole('button', { name: 'Limpar filtros' }).click()
  await expect(history.getByRole('button', { name: 'Filtrar período' })).toBeEnabled()
})

test('estoque inicial apresenta erro sem baixar arquivo ao exceder limite do relatório', async ({ page }) => {
  await page.route('**/api/v1/inventory/opening-stock/batches/history?*', (route) =>
    route.fulfill({
      status: 200, contentType: 'application/json',
      body: '{"items":[],"limit":10,"offset":0,"has_more":false}',
    }))
  await page.route('**/api/v1/inventory/opening-stock/batches/history/export.csv', (route) =>
    route.fulfill({
      status: 422, contentType: 'application/json',
      body: '{"code":"validation_error","message":"mais de 1000 lotes; escolha um periodo menor para exportar"}',
    }))
  await login(page, 'gerente@sistema.local')
  await page.goto('/inventory')
  const history = page.getByRole('region', { name: 'Histórico de importações confirmadas' })
  await expect(history.getByRole('button', { name: 'Baixar histórico CSV' })).toBeEnabled()
  await history.getByRole('button', { name: 'Baixar histórico CSV' }).click()
  await expect(history.getByText(/mais de 1000 lotes/)).toBeVisible()
})

test('caixa não pode exportar CSV de nenhum módulo', async ({ page }) => {
  await login(page, 'caixa@sistema.local')
  const statuses = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    const check = async (path: string) => {
      try {
        await apiJson(path)
        return 200
      } catch (err) {
        if (err instanceof APIError) return err.status
        throw err
      }
    }
    return [
      await check('/api/v1/products/import-batches/history/export.csv'),
      await check('/api/v1/inventory/opening-stock/batches/history/export.csv'),
    ]
  })
  expect(statuses).toEqual([403, 403])
})
