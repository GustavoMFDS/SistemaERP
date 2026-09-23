import { expect, test } from '@playwright/test'

async function login(
  page: import('@playwright/test').Page,
  email = 'admin@sistema.local',
) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill(email)
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('PDV supports shortcuts, quick search, suspended carts, quantity editing and authorized discount', async ({ page }) => {
  await login(page)
  const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 14)

  const setup = await page.evaluate(async (suffix) => {
    const { apiJson } = await import('/src/lib/api.ts')
    const me = await apiJson<{ permissions: string[] }>('/api/v1/auth/me')
    const product = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `E2E-PDV-${suffix}`,
        barcode: `BAR-${suffix}`,
        name: `Produto Operacional ${suffix}`,
        description: null,
        unit: 'UN',
        cost_price: 4,
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
        delta: 5,
        reason: 'Carga E2E operacional',
        type: 'purchase',
      },
    })
    const productDetail = await apiJson<{ cost_price: number }>(`/api/v1/products/${product.id}`)
    return {
      productId: product.id,
      barcode: `BAR-${suffix}`,
      adminCostPrice: productDetail.cost_price,
      canDiscount: me.permissions.includes('sale:discount'),
    }
  }, suffix)

  expect(setup.canDiscount).toBe(true)
  expect(setup.adminCostPrice).toBe(4)

  await page.getByRole('link', { name: 'PDV' }).click()
  await expect(page).toHaveURL(/\/pdv$/)

  const openResponsePromise = page.waitForResponse(
    (response) =>
      response.url().includes('/api/v1/cash/sessions/open') &&
      response.request().method() === 'POST',
  )
  await page.getByRole('button', { name: 'Abrir' }).click()
  const openResponse = await openResponsePromise
  expect(openResponse.ok()).toBe(true)
  const opened = (await openResponse.json()) as { id: string }

  await page.keyboard.press('F4')
  const quickSearch = page.getByLabel('Busca rápida por nome, SKU ou código')
  await expect(quickSearch).toBeFocused()
  await quickSearch.fill(`E2E-PDV-${suffix}`)

  await page.getByLabel('Produto').selectOption(setup.productId)
  await page.getByLabel('Qtd').fill('1')
  await page.getByRole('button', { name: 'Adicionar' }).click()

  const qtyInput = page.getByLabel(/Quantidade de E2E-PDV-/)
  await expect(qtyInput).toHaveValue('1')
  await page.getByRole('button', { name: /Aumentar quantidade de E2E-PDV-/ }).click()
  await expect(qtyInput).toHaveValue('2')

  const discount = page.getByLabel('Desconto da venda (R$)')
  await expect(discount).toBeVisible()
  await discount.fill('2')
  await expect(page.getByText('R$ 18.00', { exact: true })).toBeVisible()

  await page.getByRole('button', { name: 'Suspender' }).click()
  await expect(page.getByText('Vendas suspensas')).toBeVisible()
  await expect(page.getByText('Nenhum item.')).toBeVisible()
  await page.getByRole('button', { name: 'Retomar' }).click()
  await expect(page.getByLabel(/Quantidade de E2E-PDV-/)).toHaveValue('2')
  await expect(page.getByLabel('Desconto da venda (R$)')).toHaveValue('2')

  await page.keyboard.press('F2')
  await expect(page.getByLabel('Código de barras')).toBeFocused()

  const saleResponsePromise = page.waitForResponse(
    (response) =>
      response.url().endsWith('/api/v1/sales') &&
      response.request().method() === 'POST',
  )
  await page.keyboard.press('F8')
  const saleResponse = await saleResponsePromise
  expect(saleResponse.ok()).toBe(true)
  const sale = (await saleResponse.json()) as { id: string; total: number }
  expect(sale.total).toBe(18)

  const adminSaleDetail = await page.evaluate(async (saleId) => {
    const { apiJson } = await import('/src/lib/api.ts')
    return apiJson<{
      sale: { profit_estimated: number }
      items: Array<{ cost_unit: number }>
    }>(`/api/v1/sales/${saleId}`)
  }, sale.id)
  expect(adminSaleDetail.sale.profit_estimated).toBeGreaterThan(0)
  expect(adminSaleDetail.items[0].cost_unit).toBe(4)

  await expect(page.getByText(/Venda finalizada:/)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Imprimir comprovante não fiscal' })).toBeVisible()

  await page.evaluate(
    async ({ cashId }) => {
      const { apiJson } = await import('/src/lib/api.ts')
      await apiJson(`/api/v1/cash/sessions/${cashId}/close`, {
        method: 'POST',
        body: {
          closing_amount: 0,
          closing_by_method: {
            pix: 18,
            debit: 0,
            credit: 0,
            transfer: 0,
            voucher: 0,
          },
          notes: 'cleanup PDV operations E2E',
        },
      })
    },
    { cashId: opened.id },
  )

  await page.getByRole('button', { name: 'Sair' }).click()
  await expect(page).toHaveURL(/\/login$/)

  await login(page, 'caixa@sistema.local')

  const cashierResult = await page.evaluate(async (productId) => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    const me = await apiJson<{ permissions: string[] }>('/api/v1/auth/me')
    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: 'cashier discount guard E2E' },
    })

    let status = 0
    try {
      await apiJson('/api/v1/sales', {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          cash_session_id: cash.id,
          customer_id: null,
          discount_value: 1,
          items: [{ product_id: productId, qty: 1, discount_value: 0 }],
          payments: [{ method: 'pix', amount: 9 }],
        },
      })
      status = 200
    } catch (error) {
      if (error instanceof APIError) status = error.status
      else throw error
    }

    await apiJson(`/api/v1/cash/sessions/${cash.id}/close`, {
      method: 'POST',
      body: {
        closing_amount: 0,
        closing_by_method: {
          pix: 0,
          debit: 0,
          credit: 0,
          transfer: 0,
          voucher: 0,
        },
        notes: 'cleanup cashier guard E2E',
      },
    })

    return {
      hasDiscountPermission: me.permissions.includes('sale:discount'),
      discountedSaleStatus: status,
    }
  }, setup.productId)

  expect(cashierResult.hasDiscountPermission).toBe(false)
  expect(cashierResult.discountedSaleStatus).toBe(403)
})
