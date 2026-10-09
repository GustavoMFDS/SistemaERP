import { expect, test } from '@playwright/test'

test('estoque: busca de produto além dos primeiros 200 sem rolagem', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
  await page.getByRole('link', { name: 'Estoque', exact: true }).click()
  await page.getByText('Ajustar quantidade no estoque').click()
  const search = page.getByRole('searchbox', { name: 'Buscar produto para ajuste de estoque' })
  await expect(search).toBeVisible()
  const remoteId = '12121212-3434-4567-8123-989898989898'
  let called = false
  await page.route('**/api/v1/products?query=*', async (route) => {
    const query = new URL(route.request().url()).searchParams.get('query')
    if (query === 'produto remoto') {
      called = true
      await route.fulfill({ status: 200, json: { items: [{ id: remoteId, sku: 'REMOTO-999',
        name: 'Produto remoto da prateleira 999', unit: 'un', min_stock: 2,
        qty_on_hand: 10, active: true, price_cash: 14.9 }], total: 1 } })
    } else await route.continue()
  })
  await search.fill('produto remoto')
  await expect(page.getByRole('option', { name: /Produto remoto da prateleira 999/ })).toHaveCount(1)
  expect(called).toBe(true)
  await page.getByLabel('Produto para ajustar').selectOption(remoteId)
  await expect(page.getByLabel('Produto para ajustar')).toHaveValue(remoteId)
  await expect(page.getByRole('button', { name: 'Aplicar ajuste' })).toBeDisabled()
})
