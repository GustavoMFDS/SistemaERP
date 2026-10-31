import { cashTest as test, expect } from './fixtures'

test('E2E cash cleanup reconciles a synthetic sale to the expected cash balance', async ({ page }) => {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha', { exact: true }).fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/home$/)

  const expectedCash = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 14)
    const product = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `CI-CASH-CLEANUP-${suffix}`,
        barcode: null,
        name: `Synthetic cash cleanup ${suffix}`,
        description: 'Disposable E2E cash cleanup item',
        unit: 'UN',
        ncm: '00000000',
        cost_price: 3,
        price_cash: 10,
        promo_price: null,
        min_stock: 0,
        active: true,
      },
    })

    await apiJson('/api/v1/inventory/adjust', {
      method: 'POST',
      body: {
        product_id: product.id,
        delta: 1,
        reason: 'Synthetic E2E cash cleanup stock',
        type: 'adjustment',
      },
    })

    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 25, notes: 'Synthetic E2E cash cleanup' },
    })
    await apiJson('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        cash_session_id: cash.id,
        customer_id: null,
        discount_value: 0,
        items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
        payments: [{ method: 'cash', amount: 10 }],
      },
    })

    const current = await apiJson<{ expected_cash: number }>('/api/v1/cash/sessions/current')
    return current.expected_cash
  })

  expect(expectedCash).toBe(35)
})
