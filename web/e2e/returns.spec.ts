import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('partial return is idempotent, bounded by sold quantity and blocks full cancellation', async ({ page }) => {
  await login(page)
  const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 16)

  const result = await page.evaluate(async (suffix) => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')

    type Product = {
      id: string
      qty_on_hand: number
      price_cash: number
    }
    type SaleCreate = { id: string; total: number }
    type SaleDetail = {
      sale: { id: string; status: string; total: number }
      items: Array<{ id: string; product_id: string; qty: number }>
    }
    type ReturnCreate = {
      id: string
      refund_due: number
      refund_status: string
      replayed: boolean
    }

    const product = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `E2E-RETURN-${suffix}`,
        barcode: null,
        name: `Produto Devolucao ${suffix}`,
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
        reason: 'Carga E2E devolucao',
        type: 'adjustment',
      },
    })

    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: null },
    })

    const sale = await apiJson<SaleCreate>('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        cash_session_id: cash.id,
        customer_id: null,
        discount_value: 0,
        items: [{ product_id: product.id, qty: 2, discount_value: 0 }],
        payments: [{ method: 'pix', amount: 20 }],
      },
    })

    const saleDetail = await apiJson<SaleDetail>(`/api/v1/sales/${sale.id}`)
    const saleItemId = saleDetail.items[0].id
    const afterSale = await apiJson<Product>(`/api/v1/products/${product.id}`)

    const returnKey = crypto.randomUUID()
    const partial = await apiJson<ReturnCreate>(`/api/v1/sales/${sale.id}/returns`, {
      method: 'POST',
      headers: { 'Idempotency-Key': returnKey },
      body: {
        kind: 'return',
        reason: 'Devolucao parcial E2E',
        items: [{ sale_item_id: saleItemId, qty: 1, restock: true }],
      },
    })

    const partialReplay = await apiJson<ReturnCreate>(`/api/v1/sales/${sale.id}/returns`, {
      method: 'POST',
      headers: { 'Idempotency-Key': returnKey },
      body: {
        kind: 'return',
        reason: 'Devolucao parcial E2E',
        items: [{ sale_item_id: saleItemId, qty: 1, restock: true }],
      },
    })

    const afterPartial = await apiJson<Product>(`/api/v1/products/${product.id}`)

    let overReturnStatus = 0
    try {
      await apiJson(`/api/v1/sales/${sale.id}/returns`, {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          kind: 'return',
          reason: 'Quantidade acima do restante',
          items: [{ sale_item_id: saleItemId, qty: 2, restock: true }],
        },
      })
      overReturnStatus = 200
    } catch (error) {
      if (error instanceof APIError) overReturnStatus = error.status
      else throw error
    }

    let cancelAfterReturnStatus = 0
    try {
      await apiJson(`/api/v1/sales/${sale.id}/cancel`, {
        method: 'POST',
        body: { reason: 'Nao pode cancelar apos devolucao parcial' },
      })
      cancelAfterReturnStatus = 200
    } catch (error) {
      if (error instanceof APIError) cancelAfterReturnStatus = error.status
      else throw error
    }

    const exchange = await apiJson<ReturnCreate>(`/api/v1/sales/${sale.id}/returns`, {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        kind: 'exchange',
        reason: 'Troca com item avariado E2E',
        items: [{ sale_item_id: saleItemId, qty: 1, restock: false }],
      },
    })

    const afterExchange = await apiJson<Product>(`/api/v1/products/${product.id}`)
    const returnList = await apiJson<{
      items: Array<{ id: string; sale_id: string; kind: string; refund_due: number }>
      total: number
    }>(`/api/v1/returns?sale_id=${sale.id}&limit=20&offset=0`)

    const movements = await apiJson<{
      items: Array<{ movement_type: string; reference_type?: string | null; reference_id?: string | null }>
      total: number
    }>(`/api/v1/inventory/movements?product_id=${product.id}&limit=50&offset=0`)

    await apiJson(`/api/v1/cash/sessions/${cash.id}/close`, {
      method: 'POST',
      body: { closing_amount: 0, notes: 'cleanup return E2E' },
    })

    return {
      saleTotal: sale.total,
      afterSaleQty: afterSale.qty_on_hand,
      partialId: partial.id,
      partialRefund: partial.refund_due,
      partialRefundStatus: partial.refund_status,
      replaySameId: partialReplay.id === partial.id,
      replayFlag: partialReplay.replayed,
      afterPartialQty: afterPartial.qty_on_hand,
      overReturnStatus,
      cancelAfterReturnStatus,
      exchangeRefund: exchange.refund_due,
      afterExchangeQty: afterExchange.qty_on_hand,
      returnCount: returnList.items.filter((item) => item.sale_id === sale.id).length,
      returnMovementCount: movements.items.filter(
        (item) => item.movement_type === 'return' && item.reference_type === 'sale_return',
      ).length,
    }
  }, suffix)

  expect(result.saleTotal).toBe(20)
  expect(result.afterSaleQty).toBe(3)
  expect(result.partialRefund).toBe(10)
  expect(result.partialRefundStatus).toBe('pending')
  expect(result.replaySameId).toBe(true)
  expect(result.replayFlag).toBe(true)
  expect(result.afterPartialQty).toBe(4)
  expect(result.overReturnStatus).toBe(422)
  expect(result.cancelAfterReturnStatus).toBe(409)
  expect(result.exchangeRefund).toBe(10)
  expect(result.afterExchangeQty).toBe(4)
  expect(result.returnCount).toBe(2)
  expect(result.returnMovementCount).toBe(1)

  await page.getByRole('link', { name: 'Devoluções/Trocas' }).click()
  await expect(page).toHaveURL(/\/returns$/)
  await expect(page.getByText('Últimas devoluções/trocas')).toBeVisible()
})
