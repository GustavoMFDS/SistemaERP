import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email: string) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('financeiro exibe ranking do CNPJ e período somente após consulta', async ({ page }) => {
  const requests: string[] = []
  await page.route('**/api/v1/finance/products-ranking?*', (route) => {
    requests.push(new URL(route.request().url()).search)
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        from: '2026-10-01', to: '2026-10-08', limit: 20,
        items: [{
          product_id: '00000000-0000-4000-8000-000000000009',
          sku: 'CAFE-500', name: 'Café 500g',
          quantity: '3.000', item_total: '89.70', sales_count: 2,
        }],
      }),
    })
  })
  await login(page, 'gerente@sistema.local')
  await page.goto('/finance')
  const section = page.getByRole('region', { name: 'Produtos mais vendidos' })
  await expect(section.getByText(/não é lucro/)).toBeVisible()
  expect(requests).toHaveLength(0)
  await section.getByLabel('Data inicial').fill('2026-10-01')
  await section.getByLabel('Data final').fill('2026-10-08')
  await section.getByRole('button', { name: 'Ver ranking' }).click()
  await expect(section.getByText('Café 500g')).toBeVisible()
  await expect(section.getByText('CAFE-500')).toBeVisible()
  await expect(section.getByText('89,70')).toBeVisible()
  expect(requests).toHaveLength(1)
  expect(requests[0]).toContain('from=2026-10-01')
  expect(requests[0]).toContain('to=2026-10-08')
  expect(requests[0]).not.toContain('tenant_id')
  await section.getByLabel('Data inicial').fill('2026-10-09')
  await expect(section.getByRole('button', { name: 'Ver ranking' })).toBeDisabled()
})

test('caixa recebe 403 para ranking financeiro e não vê a página no menu', async ({ page }) => {
  await login(page, 'caixa@sistema.local')
  await expect(page.getByRole('link', { name: 'Financeiro', exact: true })).toHaveCount(0)
  const status = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    try {
      await apiJson('/api/v1/finance/products-ranking?from=2026-10-01&to=2026-10-08')
      return 200
    } catch (cause) {
      return cause instanceof APIError ? cause.status : 0
    }
  })
  expect(status).toBe(403)
})

test('API valida datas e tamanho da lista de ranking', async ({ page }) => {
  await login(page, 'gerente@sistema.local')
  const errors = await page.evaluate(async () => {
    const { apiJson, APIError } = await import('/src/lib/api.ts')
    const status = async (params: string) => {
      try {
        await apiJson('/api/v1/finance/products-ranking?' + params)
        return 200
      } catch (cause) {
        return cause instanceof APIError ? cause.status : 0
      }
    }
    return [
      await status('from=2026-10-09&to=2026-10-08'),
      await status('from=2026-02-30&to=2026-10-08'),
      await status('from=2026-10-01&to=2026-10-08&limit=51'),
      await status('to=2026-10-08'),
      await status('from=2026-10-08&from=2026-10-07&to=2026-10-08'),
      await status('from=2026-10-08&to=2026-10-08&tenant_id=other-company'),
      await status('from=2026-10-08&to=2026-10-08&limit=1&limit=2'),
    ]
  })
  expect(errors).toEqual([422, 422, 422, 422, 422, 422, 422])
})
