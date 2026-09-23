import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('partial returns are idempotent, bounded and separate from refunds', async ({ page }) => {
  await login(page)

  const result = await page.evaluate(async () => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')
    const suffix = crypto.randomUUID().slice(0, 8)

    const product = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `E2E-RETURN-${suffix}`,
        barcode: null,
        name: `Produto Devolução ${suffix}`,
        description: null,
        unit: 'UN',
        cost_price: 5,
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
        delta: 10,
        reason: 'Estoque inicial E2E devolução',
        type: 'purchase',
      },
    })

    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: 'returns e2e' },
    })

    const sale = await apiJson<{ id: string; total: number }>('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        cash_session_id: cash.id,
        customer_id: null,
        discount_value: 0,
        items: [{ product_id: product.id, qty: 4, discount_value: 0 }],
        payments: [{ method: 'pix', amount: 40 }],
      },
    })

    const saleDetail = await apiJson<{
      sale: { status: string }
      items: Array<{ id: string; product_id: string; qty: number }>
    }>(`/api/v1/sales/${sale.id}`)
    const saleItem = saleDetail.items[0]

    const afterSale = await apiJson<{ qty_on_hand: number }>(`/api/v1/products/${product.id}`)

    const firstKey = crypto.randomUUID()
    const firstRequest = {
      reason: 'Cliente devolveu parte',
      items: [{ sale_item_id: saleItem.id, qty: 1.5, restock: true }],
    }
    const first = await apiJson<{
      return: { id: string; total_amount: number; refunded_amount: number }
      replayed: boolean
    }>(`/api/v1/sales/${sale.id}/returns`, {
      method: 'POST',
      headers: { 'Idempotency-Key': firstKey },
      body: firstRequest,
    })
    const firstReplay = await apiJson<{
      return: { id: string; total_amount: number; refunded_amount: number }
      replayed: boolean
    }>(`/api/v1/sales/${sale.id}/returns`, {
      method: 'POST',
      headers: { 'Idempotency-Key': firstKey },
      body: firstRequest,
    })

    const afterFirstReturn = await apiJson<{ qty_on_hand: number }>(
      `/api/v1/products/${product.id}`,
    )

    const second = await apiJson<{
      return: { id: string; total_amount: number }
      replayed: boolean
    }>(`/api/v1/sales/${sale.id}/returns`, {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        reason: 'Produto danificado sem reposição',
        items: [{ sale_item_id: saleItem.id, qty: 0.5, restock: false }],
      },
    })

    const afterSecondReturn = await apiJson<{ qty_on_hand: number }>(
      `/api/v1/products/${product.id}`,
    )

    let overReturnStatus = 0
    try {
      await apiJson(`/api/v1/sales/${sale.id}/returns`, {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          reason: 'Tentativa acima do restante',
          items: [{ sale_item_id: saleItem.id, qty: 2.5, restock: true }],
        },
      })
    } catch (error) {
      if (error instanceof APIError) overReturnStatus = error.status
      else throw error
    }

    const refundKey = crypto.randomUUID()
    const refundRequest = {
      method: 'store_credit',
      amount: 10,
      external_reference: `CRED-${suffix}`,
      notes: 'Crédito parcial para troca',
    }
    const refund = await apiJson<{
      refund: { id: string; amount: number; method: string }
      replayed: boolean
    }>(`/api/v1/sales/${sale.id}/returns/${first.return.id}/refunds`, {
      method: 'POST',
      headers: { 'Idempotency-Key': refundKey },
      body: refundRequest,
    })
    const refundReplay = await apiJson<{
      refund: { id: string; amount: number; method: string }
      replayed: boolean
    }>(`/api/v1/sales/${sale.id}/returns/${first.return.id}/refunds`, {
      method: 'POST',
      headers: { 'Idempotency-Key': refundKey },
      body: refundRequest,
    })

    let overRefundStatus = 0
    try {
      await apiJson(`/api/v1/sales/${sale.id}/returns/${first.return.id}/refunds`, {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          method: 'cash',
          amount: 6,
          external_reference: null,
          notes: null,
        },
      })
    } catch (error) {
      if (error instanceof APIError) overRefundStatus = error.status
      else throw error
    }

    let cancelAfterReturnStatus = 0
    try {
      await apiJson(`/api/v1/sales/${sale.id}/cancel`, {
        method: 'POST',
        body: { reason: 'Não pode cancelar após devolução parcial' },
      })
    } catch (error) {
      if (error instanceof APIError) cancelAfterReturnStatus = error.status
      else throw error
    }

    const returns = await apiJson<{
      items: Array<{
        id: string
        total_amount: number
        refunded_amount: number
      }>
      total: number
    }>(`/api/v1/sales/${sale.id}/returns?limit=100&offset=0`)

    const firstDetail = await apiJson<{
      return: { id: string; total_amount: number; refunded_amount: number }
      items: Array<{ qty: number; restock: boolean }>
      refunds: Array<{ id: string; amount: number; method: string }>
    }>(`/api/v1/sales/${sale.id}/returns/${first.return.id}`)

    const movements = await apiJson<{
      items: Array<{ movement_type: string; delta: number; reference_type?: string | null }>
      total: number
    }>(`/api/v1/inventory/movements?product_id=${product.id}&limit=50&offset=0`)

    const ledger = await apiJson<{
      items: Array<{ entry_type: string; sale_id?: string | null; amount_net: number }>
      total: number
    }>('/api/v1/finance/ledger?limit=200&offset=0')

    await apiJson(`/api/v1/cash/sessions/${cash.id}/close`, {
      method: 'POST',
      body: { closing_amount: 0, notes: 'returns e2e cleanup' },
    })

    return {
      saleId: sale.id,
      afterSaleQty: afterSale.qty_on_hand,
      firstReturnId: first.return.id,
      firstAmount: first.return.total_amount,
      firstReplaySameId: firstReplay.return.id === first.return.id,
      firstReplayFlag: firstReplay.replayed,
      afterFirstReturnQty: afterFirstReturn.qty_on_hand,
      secondAmount: second.return.total_amount,
      afterSecondReturnQty: afterSecondReturn.qty_on_hand,
      overReturnStatus,
      refundId: refund.refund.id,
      refundReplaySameId: refundReplay.refund.id === refund.refund.id,
      refundReplayFlag: refundReplay.replayed,
      refundMethod: refund.refund.method,
      overRefundStatus,
      cancelAfterReturnStatus,
      returnCount: returns.total,
      firstRefundedAmount: firstDetail.return.refunded_amount,
      firstRefundCount: firstDetail.refunds.length,
      returnMovementCount: movements.items.filter(
        (item) => item.movement_type === 'return' && item.reference_type === 'sale_return',
      ).length,
      returnLedgerCount: ledger.items.filter(
        (item) => item.entry_type === 'sale_return' && item.sale_id === sale.id,
      ).length,
    }
  })

  expect(result.afterSaleQty).toBe(6)
  expect(result.firstAmount).toBe(15)
  expect(result.firstReplaySameId).toBe(true)
  expect(result.firstReplayFlag).toBe(true)
  expect(result.afterFirstReturnQty).toBe(7.5)
  expect(result.secondAmount).toBe(5)
  expect(result.afterSecondReturnQty).toBe(7.5)
  expect(result.overReturnStatus).toBe(409)
  expect(result.refundReplaySameId).toBe(true)
  expect(result.refundReplayFlag).toBe(true)
  expect(result.refundMethod).toBe('store_credit')
  expect(result.overRefundStatus).toBe(409)
  expect(result.cancelAfterReturnStatus).toBe(409)
  expect(result.returnCount).toBe(2)
  expect(result.firstRefundedAmount).toBe(10)
  expect(result.firstRefundCount).toBe(1)
  expect(result.returnMovementCount).toBe(1)
  expect(result.returnLedgerCount).toBe(2)

  await page.getByRole('link', { name: 'Vendas / Devoluções' }).click()
  await expect(page).toHaveURL(/\/returns$/)
  await expect(page.getByText(result.saleId)).toBeVisible()
})
