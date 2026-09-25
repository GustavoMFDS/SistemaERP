import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('malformed UUIDs are rejected at the HTTP boundary and privacy not-found stays 404', async ({
  page,
}) => {
  await login(page)

  const statuses = await page.evaluate(async () => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')

    async function statusOf(
      path: string,
      init: { method?: string; body?: unknown; headers?: Record<string, string> } = {},
    ): Promise<number> {
      try {
        await apiJson(path, init)
        return 200
      } catch (error) {
        if (error instanceof APIError) return error.status
        throw error
      }
    }

    const products = await apiJson<{
      items: Array<{
        category_id?: string | null
        sku: string
        barcode?: string | null
        name: string
        description?: string | null
        unit: string
        cost_price: number
        price_cash: number
        promo_price?: number | null
        min_stock: number
        active: boolean
      }>
    }>('/api/v1/products?limit=1&offset=0')
    const existingProduct = products.items[0]
    if (!existingProduct) throw new Error('seeded product required')

    const duplicateProduct = await statusOf('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: existingProduct.category_id ?? null,
        sku: existingProduct.sku,
        barcode: existingProduct.barcode ?? null,
        name: existingProduct.name,
        description: existingProduct.description ?? null,
        unit: existingProduct.unit,
        cost_price: existingProduct.cost_price,
        price_cash: existingProduct.price_cash,
        promo_price: existingProduct.promo_price ?? null,
        min_stock: existingProduct.min_stock,
        active: existingProduct.active,
      },
    })

    const productGet = await statusOf('/api/v1/products/not-a-uuid')
    const saleGet = await statusOf('/api/v1/sales/not-a-uuid')
    const saleCancel = await statusOf('/api/v1/sales/not-a-uuid/cancel', {
      method: 'POST',
      body: { reason: 'invalid id contract' },
    })
    const cashMovement = await statusOf(
      '/api/v1/cash/sessions/not-a-uuid/movements',
      {
        method: 'POST',
        body: { movement_type: 'supply', amount: 1, notes: null },
      },
    )
    const cashClose = await statusOf('/api/v1/cash/sessions/not-a-uuid/close', {
      method: 'POST',
      body: { closing_amount: 0, notes: null },
    })
    const inventoryQuery = await statusOf(
      '/api/v1/inventory/movements?product_id=not-a-uuid',
    )
    const inventoryAdjust = await statusOf('/api/v1/inventory/adjust', {
      method: 'POST',
      body: {
        product_id: 'not-a-uuid',
        delta: 1,
        reason: 'invalid id contract',
        type: 'adjustment',
      },
    })
    const saleCreate = await statusOf('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        cash_session_id: 'not-a-uuid',
        customer_id: null,
        discount_value: 0,
        items: [
          {
            product_id: '11111111-1111-1111-1111-111111111111',
            qty: 1,
            discount_value: 0,
          },
        ],
        payments: [{ method: 'cash', amount: 1 }],
      },
    })
    const fiscalGenerate = await statusOf('/api/v1/fiscal/nfe/xml', {
      method: 'POST',
      body: { sale_id: 'not-a-uuid' },
    })
    const fiscalDownload = await statusOf(
      '/api/v1/fiscal/nfe/xml/not-a-uuid/download',
    )
    const financeBadFrom = await statusOf(
      '/api/v1/finance/dashboard?from=not-a-date&to=2026-09-25',
    )
    const financeReverseRange = await statusOf(
      '/api/v1/finance/dashboard?from=2026-09-26&to=2026-09-25',
    )
    const privacyRequest = await statusOf(
      '/api/v1/privacy/requests/not-a-uuid',
    )
    const privacyConsentNotFound = await statusOf('/api/v1/privacy/consents', {
      method: 'POST',
      body: {
        subject_type: 'user',
        subject_id: '99999999-9999-9999-9999-999999999999',
        purpose: 'contract-test',
        consent_text_version: 'v1',
        source: 'e2e',
      },
    })

    return {
      duplicateProduct,
      productGet,
      saleGet,
      saleCancel,
      cashMovement,
      cashClose,
      inventoryQuery,
      inventoryAdjust,
      saleCreate,
      fiscalGenerate,
      fiscalDownload,
      financeBadFrom,
      financeReverseRange,
      privacyRequest,
      privacyConsentNotFound,
    }
  })

  expect(statuses).toEqual({
    duplicateProduct: 409,
    productGet: 422,
    saleGet: 422,
    saleCancel: 422,
    cashMovement: 422,
    cashClose: 422,
    inventoryQuery: 422,
    inventoryAdjust: 422,
    saleCreate: 422,
    fiscalGenerate: 422,
    fiscalDownload: 422,
    financeBadFrom: 422,
    financeReverseRange: 422,
    privacyRequest: 422,
    privacyConsentNotFound: 404,
  })
})
