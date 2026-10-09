import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page, email = 'gerente@sistema.local') {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

function movement(index: number, product: string) {
  return {
    id: '00000000-0000-4000-8000-' + String(index).padStart(12, '0'),
    product_id: product,
    product_sku: index % 2 ? 'ARROZ-5' : 'AGUA-1',
    product_name: index % 2 ? 'Arroz 5kg' : 'Água 1L',
    movement_type: index % 2 ? 'sale' : 'purchase',
    delta: index % 2 ? -1 : 10,
    qty_before: 12,
    qty_after: index % 2 ? 11 : 22,
    reason: 'Movimento validado',
    created_at: '2026-10-08T21:00:00Z',
  }
}

test('histórico mostra entradas e saídas com paginação e filtro por produto', async ({ page }) => {
  const productID = '00000000-0000-4000-8000-000000000333'
  const queries: string[] = []
  await page.route('**/api/v1/inventory/movements?*', async (route) => {
    const url = new URL(route.request().url())
    queries.push(url.search)
    const offset = Number(url.searchParams.get('offset') || 0)
    const itemFilter = url.searchParams.get('product_id')
    const entries = itemFilter ? [movement(1, productID)] : offset === 0
      ? Array.from({ length: 20 }, (_, i) => movement(20 - i, productID))
      : [movement(101, productID)]
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ items: entries, total: itemFilter ? 1 : 21 }),
    })
  })
  await login(page)
  await page.getByRole('link', { name: 'Movimentações', exact: true }).click()
  await expect(page).toHaveURL(/\/stock-movements$/)
  const table = page.getByRole('table')
  await expect(table.getByRole('row')).toHaveCount(21)
  await expect(table.getByText('Recebimento de compra').first()).toBeVisible()
  await expect(table.getByText('Venda').first()).toBeVisible()
  await page.getByRole('button', { name: 'Próxima página' }).click()
  await expect(table.getByRole('row')).toHaveCount(2)
  await page.getByRole('button', { name: 'Página anterior' }).click()
  await expect(table.getByRole('row')).toHaveCount(21)
  await table.getByRole('button', { name: 'Arroz 5kg' }).first().click()
  await expect(page.getByText('Produto: Arroz 5kg')).toBeVisible()
  await expect(table.getByRole('row')).toHaveCount(2)
  await expect.poll(() => queries.some((query) => query.includes('product_id=' + productID))).toBe(true)
  await page.getByRole('button', { name: 'Todos os produtos' }).click()
  await expect(table.getByRole('row')).toHaveCount(21)
  expect(queries).toContain('?limit=20&offset=20')
})

test('operador de caixa tem leitura de movimentos mas não controles de ajuste', async ({ page }) => {
  await page.route('**/api/v1/inventory/movements?*', (route) =>
    route.fulfill({
      status: 200, contentType: 'application/json',
      body: '{"items":[],"total":0}',
    }))
  await login(page, 'caixa@sistema.local')
  await page.goto('/stock-movements')
  await expect(page.getByRole('heading', { name: 'Movimentações de estoque' })).toBeVisible()
  await expect(page.getByText(/apenas para consulta/)).toBeVisible()
  await expect(page.getByRole('button', { name: /Ajustar|Registrar ajuste/i })).toHaveCount(0)
})
