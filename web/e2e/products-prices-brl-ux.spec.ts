import { expect, test } from './fixtures'

test('preço do catálogo usa moeda brasileira sem alterar o valor do produto', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  await page.route('**/api/v1/products?*', async (route) => {
    await route.fulfill({
      status: 200, contentType: 'application/json',
      body: JSON.stringify({ total: 1, items: [{
        id: 'produto-preco-exemplo', sku: 'PRECO-100',
        name: 'Produto de teste', unit: 'un',
        price_cash: 1234.5, cost_price: 0, min_stock: 2,
        qty_on_hand: 5, active: true, barcode: null,
      }] }),
    })
  })

  await page.goto('/products')
  await expect(page.getByRole('cell', { name: 'R$ 1.234,50' })).toBeVisible()
  await expect(page.getByRole('cell', { name: '1234.50' })).toHaveCount(0)
})
