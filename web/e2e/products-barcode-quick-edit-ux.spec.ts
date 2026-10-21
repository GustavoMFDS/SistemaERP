import { expect, test } from './fixtures'

test('código de barras pode ser salvo sem abrir os campos fiscais', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail ou nome de usuário').fill('admin@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  const product = {
    id: 'b26fa190-864e-41c8-920d-fd0e20d82c19',
    sku: 'BAR-001', name: 'Produto com leitor', unit: 'un',
    price_cash: 12.5, cost_price: 8, min_stock: 2,
    qty_on_hand: 7, active: true, barcode: null,
    return_policy: 'allowed',
  }
  let savedBarcode = ''
  let savedPrice = 0
  await page.route('**/api/v1/products?*', (route) =>
    route.fulfill({ status: 200, contentType: 'application/json',
      body: JSON.stringify({ items: [{ ...product, barcode: savedBarcode || null }], total: 1 }) }))
  await page.route('**/api/v1/products/' + product.id, async (route) => {
    const body = route.request().postDataJSON() as { barcode: string; price_cash: number }
    savedBarcode = body.barcode
    savedPrice = body.price_cash
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ id: product.id }) })
  })

  await page.goto('/products')
  await expect(page.getByRole('columnheader', { name: 'Código de barras' })).toBeVisible()
  await expect(page.getByRole('columnheader', { name: 'Classificação (NCM)' })).toHaveCount(0)
  await page.getByLabel('Código de barras de Produto com leitor').fill('7891234567895')
  await page.getByRole('button', { name: 'Salvar código de barras de Produto com leitor' }).click()
  await expect.poll(() => savedBarcode).toBe('7891234567895')
  expect(savedPrice).toBe(12.5)
  await expect(page.getByLabel('Código de barras de Produto com leitor')).toHaveValue('7891234567895')
})
