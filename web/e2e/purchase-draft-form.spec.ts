import { expect, test } from '@playwright/test'

test('purchase draft reset does not block submitting an added line', async ({ page }) => {
  const supplier = { id: 'supplier-1', name: 'Lab supplier', active: true }
  const product = { id: 'product-1', sku: 'LAB-1', name: 'Lab product', cost_price: 6, active: true }
  const submitted: unknown[] = []
  await page.route('**/api/v1/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    let body: unknown = { items: [], total: 0 }
    if (path.endsWith('/suppliers')) body = { items: [supplier], total: 1 }
    if (path.endsWith('/products')) body = { items: [product], total: 1 }
    if (path.endsWith('/purchases') && route.request().method() === 'POST') {
      submitted.push(route.request().postDataJSON())
      body = { id: 'purchase-1', replayed: false }
    }
    await route.fulfill({ json: body })
  })
  await page.goto('/e2e/fixtures/purchase-form.html')
  await page.getByLabel('Fornecedor da compra').selectOption(supplier.id)
  await page.getByLabel('Produto da compra').selectOption(product.id)
  const section = page.locator('section').filter({ has: page.getByRole('heading', { name: 'Nova compra' }) })
  const qty = section.locator('input[type=number]').nth(0)
  const cost = section.locator('input[type=number]').nth(1)
  const add = section.getByRole('button', { name: 'Incluir item' })
  const submit = section.getByRole('button', { name: 'Criar compra' })
  await qty.fill('0')
  await add.click()
  await expect(submit).toBeDisabled()
  await qty.fill('2')
  await cost.fill('0')
  await add.click()
  await expect(submit).toBeDisabled()
  await cost.fill('6.25')
  await add.click()
  await expect(qty).toHaveValue('1')
  await expect(cost).toHaveValue('0')
  await expect(section.getByText(/Total:.*12,50/)).toBeVisible()
  await section.locator('input[type=date]').fill('2026-12-10')
  await section.locator('textarea').fill('Lab notes')
  await submit.click()
  await expect.poll(() => submitted).toEqual([{
    supplier_id: supplier.id, invoice_number: null, payment_due_date: '2026-12-10',
    notes: 'Lab notes', items: [{ product_id: product.id, qty: 2, unit_cost: 6.25 }],
  }])
  await expect(submit).toBeDisabled()
})