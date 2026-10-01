import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('barcode lookup and PDV scanner add and increment the product', async ({ page }) => {
  await login(page)

  const lookup = await page.evaluate(async () => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{ sku: string; barcode?: string | null; name: string }>(
      '/api/v1/products/barcode/7890000000000',
    )
  })

  expect(lookup.sku).toBe('SKU-COCA-2L')
  expect(lookup.barcode).toBe('7890000000000')

  const invalidPromoStatus = await page.evaluate(async () => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    try {
      await apiJson('/api/v1/products', {
        method: 'POST',
        body: {
          category_id: null,
          sku: `E2E-BAD-PROMO-${crypto.randomUUID().slice(0, 8)}`,
          barcode: null,
          name: 'Promo acima do preco normal',
          description: null,
          unit: 'UN',
          cost_price: 1,
          price_cash: 10,
          promo_price: 11,
          min_stock: 0,
          active: true,
        },
      })
      return 200
    } catch (error) {
      if (error instanceof APIError) return error.status
      throw error
    }
  })
  expect(invalidPromoStatus).toBe(422)

  const duplicateStatus = await page.evaluate(async () => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    try {
      await apiJson('/api/v1/products', {
        method: 'POST',
        body: {
          category_id: null,
          sku: `E2E-DUP-BAR-${crypto.randomUUID().slice(0, 8)}`,
          barcode: '7890000000000',
          name: 'Duplicate barcode must fail',
          description: null,
          unit: 'UN',
          cost_price: 1,
          price_cash: 2,
          promo_price: null,
          min_stock: 0,
          active: true,
        },
      })
      return 200
    } catch (error) {
      if (error instanceof APIError) return error.status
      throw error
    }
  })
  expect(duplicateStatus).toBe(409)

  await page.getByRole('link', { name: 'PDV' }).click()
  await expect(page).toHaveURL(/\/pdv$/)
  const scanner = page.getByLabel('Código de barras')
  await scanner.fill('7890000000000')
  await scanner.press('Enter')

  const row = page.locator('tbody tr').filter({ hasText: 'Coca-Cola 2L' })
  const qty = page.getByLabel('Quantidade de Coca-Cola 2L')
  await expect(row).toHaveCount(1)
  await expect(qty).toHaveValue('1')

  await scanner.fill('7890000000000')
  await scanner.press('Enter')

  await expect(row).toHaveCount(1)
  await expect(qty).toHaveValue('2')
})


test('catalog create/update normalization is audited transactionally', async ({ page }) => {
  await login(page)
  const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 12)

  const result = await page.evaluate(async (suffix) => {
    const { apiJson } = await import('/src/lib/api.ts')

    const created = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `  AUDIT-${suffix}  `,
        barcode: null,
        name: `  Produto Audit ${suffix}  `,
        description: '  descricao normalizada  ',
        unit: '  UN  ',
        cost_price: 3,
        price_cash: 9,
        promo_price: null,
        min_stock: 0,
        active: true,
      },
    })

    const before = await apiJson<{
      id: string
      sku: string
      name: string
      unit: string
      description?: string | null
      barcode?: string | null
      cost_price: number
      price_cash: number
      promo_price?: number | null
      min_stock: number
      active: boolean
      category_id?: string | null
    }>(`/api/v1/products/${created.id}`)

    await apiJson<{ id: string }>(`/api/v1/products/${created.id}`, {
      method: 'PUT',
      body: {
        category_id: before.category_id ?? null,
        sku: before.sku,
        barcode: `  BAR-AUDIT-${suffix}  `,
        name: before.name,
        description: before.description ?? null,
        unit: before.unit,
        cost_price: before.cost_price,
        price_cash: before.price_cash,
        promo_price: before.promo_price ?? null,
        min_stock: before.min_stock,
        active: before.active,
      },
    })

    const after = await apiJson<{
      barcode?: string | null
      sku: string
      name: string
      unit: string
      description?: string | null
    }>(`/api/v1/products/${created.id}`)

    const creates = await apiJson<{ items: Array<{ resource_id: string }> }>(
      '/api/v1/audit/logs?action=product.create&resource_type=product&limit=200&offset=0',
    )
    const updates = await apiJson<{ items: Array<{ resource_id: string }> }>(
      '/api/v1/audit/logs?action=product.update&resource_type=product&limit=200&offset=0',
    )

    return {
      before,
      after,
      createAuditCount: creates.items.filter((item) => item.resource_id === created.id).length,
      updateAuditCount: updates.items.filter((item) => item.resource_id === created.id).length,
    }
  }, suffix)

  expect(result.before.sku).toBe(`AUDIT-${suffix}`)
  expect(result.before.name).toBe(`Produto Audit ${suffix}`)
  expect(result.before.unit).toBe('UN')
  expect(result.before.description).toBe('descricao normalizada')
  expect(result.after.barcode).toBe(`BAR-AUDIT-${suffix}`)
  expect(result.createAuditCount).toBe(1)
  expect(result.updateAuditCount).toBe(1)
})
