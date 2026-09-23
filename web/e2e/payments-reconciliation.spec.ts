import { expect, test } from '@playwright/test'

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByLabel('E-mail').fill('admin@sistema.local')
  await page.getByLabel('Senha').fill('admin123')
  await page.getByRole('button', { name: 'Entrar' }).click()
  await expect(page).toHaveURL(/\/products$/)
}

test('reconciles digital payments and settles return refunds without double-counting cash', async ({ page }) => {
  await login(page)
  const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 16)

  const result = await page.evaluate(async (suffix) => {
    const { APIError, apiJson } = await import('/src/lib/api.ts')

    const product = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `E2E-RECON-${suffix}`,
        barcode: null,
        name: `Produto Conciliacao ${suffix}`,
        description: null,
        unit: 'UN',
        cost_price: 7,
        price_cash: 20,
        promo_price: null,
        min_stock: 0,
        active: true,
      },
    })

    await apiJson('/api/v1/inventory/adjust', {
      method: 'POST',
      body: {
        product_id: product.id,
        delta: 3,
        reason: 'Carga E2E conciliacao',
        type: 'purchase',
      },
    })

    const cash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 100, notes: 'E2E reconciliation' },
    })

    const sale = await apiJson<{ id: string; total: number }>('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        cash_session_id: cash.id,
        customer_id: null,
        discount_value: 0,
        items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
        payments: [{
          method: 'pix',
          amount: 20,
          provider: 'e2e-provider',
          transaction_ref: `TX-${suffix}`,
          authorization_code: null,
          installments: 1,
        }],
      },
    })

    const paymentList = await apiJson<{
      items: Array<{
        id: string
        sale_id: string
        method: string
        amount: number
        provider?: string | null
        transaction_ref?: string | null
        reconciliation_status: string
      }>
      total: number
    }>('/api/v1/finance/payments?method=pix&status=pending&limit=200&offset=0')
    const payment = paymentList.items.find((item) => item.sale_id === sale.id)
    if (!payment) throw new Error('payment not found in finance list')

    const reconcileKey = crypto.randomUUID()
    const reconciliation = await apiJson<{ id: string; status: string; replayed: boolean }>(
      `/api/v1/finance/payments/${payment.id}/reconcile`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': reconcileKey },
        body: {
          received_amount: 20,
          fee_amount: 1,
          provider: 'e2e-provider',
          external_ref: `SETTLE-${suffix}`,
          notes: 'conciliacao E2E',
        },
      },
    )
    const reconciliationReplay = await apiJson<{ id: string; status: string; replayed: boolean }>(
      `/api/v1/finance/payments/${payment.id}/reconcile`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': reconcileKey },
        body: {
          received_amount: 20,
          fee_amount: 1,
          provider: 'e2e-provider',
          external_ref: `SETTLE-${suffix}`,
          notes: 'conciliacao E2E',
        },
      },
    )

    const saleDetail = await apiJson<{
      items: Array<{ id: string }>
    }>(`/api/v1/sales/${sale.id}`)
    const saleItemId = saleDetail.items[0].id

    const saleReturn = await apiJson<{ id: string; refund_due: number }>(
      `/api/v1/sales/${sale.id}/returns`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          kind: 'return',
          reason: 'Reembolso E2E',
          items: [{ sale_item_id: saleItemId, qty: 1, restock: true }],
        },
      },
    )

    const cashRefundKey = crypto.randomUUID()
    const cashRefund = await apiJson<{
      id: string
      status: string
      remaining_amount: number
      replayed: boolean
    }>(`/api/v1/finance/returns/${saleReturn.id}/refunds`, {
      method: 'POST',
      headers: { 'Idempotency-Key': cashRefundKey },
      body: {
        method: 'cash',
        amount: 5,
        provider: null,
        external_ref: null,
        cash_session_id: cash.id,
        notes: 'parcial em dinheiro',
      },
    })
    const cashRefundReplay = await apiJson<{
      id: string
      status: string
      remaining_amount: number
      replayed: boolean
    }>(`/api/v1/finance/returns/${saleReturn.id}/refunds`, {
      method: 'POST',
      headers: { 'Idempotency-Key': cashRefundKey },
      body: {
        method: 'cash',
        amount: 5,
        provider: null,
        external_ref: null,
        cash_session_id: cash.id,
        notes: 'parcial em dinheiro',
      },
    })

    const pixRefund = await apiJson<{
      id: string
      status: string
      remaining_amount: number
      replayed: boolean
    }>(`/api/v1/finance/returns/${saleReturn.id}/refunds`, {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        method: 'pix',
        amount: 15,
        provider: 'e2e-provider',
        external_ref: `REFUND-${suffix}`,
        cash_session_id: cash.id,
        notes: 'saldo por pix',
      },
    })

    let overRefundStatus = 0
    try {
      await apiJson(`/api/v1/finance/returns/${saleReturn.id}/refunds`, {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          method: 'pix',
          amount: 1,
          provider: 'e2e-provider',
          external_ref: `EXTRA-${suffix}`,
          cash_session_id: cash.id,
          notes: 'deve falhar',
        },
      })
      overRefundStatus = 200
    } catch (error) {
      if (error instanceof APIError) overRefundStatus = error.status
      else throw error
    }

    const refunds = await apiJson<{
      items: Array<{
        return_id: string
        status: string
        refund_due: number
        settled_amount: number
        remaining_amount: number
      }>
      total: number
    }>('/api/v1/finance/refunds?limit=200&offset=0')
    const refundSummary = refunds.items.find((item) => item.return_id === saleReturn.id)
    if (!refundSummary) throw new Error('refund summary missing')

    const paymentsAfter = await apiJson<{
      items: Array<{
        id: string
        reconciliation_status: string
        reconciled_amount?: number | null
        reconciled_fee?: number | null
      }>
      total: number
    }>('/api/v1/finance/payments?method=pix&limit=200&offset=0')
    const paymentAfter = paymentsAfter.items.find((item) => item.id === payment.id)
    if (!paymentAfter) throw new Error('reconciled payment missing')

    const close = await apiJson<{
      expected_cash: number
      closing_amount: number
      closing_difference: number
      expected_by_method: Record<string, number>
      declared_by_method: Record<string, number>
      difference_by_method: Record<string, number>
    }>(`/api/v1/cash/sessions/${cash.id}/close`, {
      method: 'POST',
      body: {
        closing_amount: 95,
        closing_by_method: {
          pix: 5,
          debit: 0,
          credit: 0,
          transfer: 0,
          voucher: 0,
        },
        notes: 'close reconciliation E2E',
      },
    })

    const ledger = await apiJson<{
      items: Array<{ entry_type: string; amount_net: number; sale_id?: string | null }>
      total: number
    }>('/api/v1/finance/ledger?limit=200&offset=0')
    const refundLedgerTotal = ledger.items
      .filter((item) => item.entry_type === 'return_refund' && item.sale_id === sale.id)
      .reduce((sum, item) => sum + item.amount_net, 0)

    return {
      saleTotal: sale.total,
      paymentProvider: payment.provider,
      paymentReference: payment.transaction_ref,
      reconciliationStatus: reconciliation.status,
      reconciliationReplaySameId: reconciliationReplay.id === reconciliation.id,
      reconciliationReplayFlag: reconciliationReplay.replayed,
      paymentAfterStatus: paymentAfter.reconciliation_status,
      paymentAfterAmount: paymentAfter.reconciled_amount,
      paymentAfterFee: paymentAfter.reconciled_fee,
      returnDue: saleReturn.refund_due,
      cashRefundStatus: cashRefund.status,
      cashRefundRemaining: cashRefund.remaining_amount,
      cashRefundReplaySameId: cashRefundReplay.id === cashRefund.id,
      cashRefundReplayFlag: cashRefundReplay.replayed,
      pixRefundStatus: pixRefund.status,
      pixRefundRemaining: pixRefund.remaining_amount,
      overRefundStatus,
      refundSummaryStatus: refundSummary.status,
      refundSummarySettled: refundSummary.settled_amount,
      refundSummaryRemaining: refundSummary.remaining_amount,
      closeExpectedCash: close.expected_cash,
      closeCashDifference: close.closing_difference,
      closeExpectedPix: close.expected_by_method.pix,
      closePixDifference: close.difference_by_method.pix,
      refundLedgerTotal,
    }
  }, suffix)

  expect(result.saleTotal).toBe(20)
  expect(result.paymentProvider).toBe('e2e-provider')
  expect(result.paymentReference).toContain('TX-')
  expect(result.reconciliationStatus).toBe('reconciled')
  expect(result.reconciliationReplaySameId).toBe(true)
  expect(result.reconciliationReplayFlag).toBe(true)
  expect(result.paymentAfterStatus).toBe('reconciled')
  expect(result.paymentAfterAmount).toBe(20)
  expect(result.paymentAfterFee).toBe(1)
  expect(result.returnDue).toBe(20)
  expect(result.cashRefundStatus).toBe('partial')
  expect(result.cashRefundRemaining).toBe(15)
  expect(result.cashRefundReplaySameId).toBe(true)
  expect(result.cashRefundReplayFlag).toBe(true)
  expect(result.pixRefundStatus).toBe('settled')
  expect(result.pixRefundRemaining).toBe(0)
  expect(result.overRefundStatus).toBe(409)
  expect(result.refundSummaryStatus).toBe('settled')
  expect(result.refundSummarySettled).toBe(20)
  expect(result.refundSummaryRemaining).toBe(0)
  expect(result.closeExpectedCash).toBe(95)
  expect(result.closeCashDifference).toBe(0)
  expect(result.closeExpectedPix).toBe(5)
  expect(result.closePixDifference).toBe(0)
  expect(result.refundLedgerTotal).toBe(-20)

  await page.getByRole('link', { name: 'Financeiro' }).click()
  await expect(page).toHaveURL(/\/finance$/)
  await expect(page.getByText('Financeiro e conciliação')).toBeVisible()
})


test('supports a net-negative digital close after refunding an earlier sale', async ({ page }) => {
  await login(page)
  const suffix = crypto.randomUUID().replace(/-/g, '').slice(0, 16)

  const result = await page.evaluate(async (suffix) => {
    const { apiJson } = await import('/src/lib/api.ts')

    const product = await apiJson<{ id: string }>('/api/v1/products', {
      method: 'POST',
      body: {
        category_id: null,
        sku: `E2E-NEG-REFUND-${suffix}`,
        barcode: null,
        name: `Produto Refund Negativo ${suffix}`,
        description: null,
        unit: 'UN',
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
        reason: 'Carga E2E refund negativo',
        type: 'purchase',
      },
    })

    const originalCash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: 'original sale for negative refund close' },
    })

    const sale = await apiJson<{ id: string }>('/api/v1/sales', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        cash_session_id: originalCash.id,
        customer_id: null,
        discount_value: 0,
        items: [{ product_id: product.id, qty: 1, discount_value: 0 }],
        payments: [{ method: 'pix', amount: 10 }],
      },
    })

    await apiJson(`/api/v1/cash/sessions/${originalCash.id}/close`, {
      method: 'POST',
      body: {
        closing_amount: 0,
        closing_by_method: {
          pix: 10,
          debit: 0,
          credit: 0,
          transfer: 0,
          voucher: 0,
        },
        notes: 'close original sale session',
      },
    })

    const saleDetail = await apiJson<{ items: Array<{ id: string }> }>(
      `/api/v1/sales/${sale.id}`,
    )
    const saleReturn = await apiJson<{ id: string; refund_due: number }>(
      `/api/v1/sales/${sale.id}/returns`,
      {
        method: 'POST',
        headers: { 'Idempotency-Key': crypto.randomUUID() },
        body: {
          kind: 'return',
          reason: 'Refund em sessao posterior',
          items: [{ sale_item_id: saleDetail.items[0].id, qty: 1, restock: true }],
        },
      },
    )

    const refundCash = await apiJson<{ id: string }>('/api/v1/cash/sessions/open', {
      method: 'POST',
      body: { opening_amount: 0, notes: 'refund-only session' },
    })

    await apiJson(`/api/v1/finance/returns/${saleReturn.id}/refunds`, {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: {
        method: 'pix',
        amount: 10,
        provider: 'e2e-provider',
        external_ref: `NEG-REFUND-${suffix}`,
        cash_session_id: refundCash.id,
        notes: 'refund without same-session pix sale',
      },
    })

    return apiJson<{
      expected_by_method: Record<string, number>
      declared_by_method: Record<string, number>
      difference_by_method: Record<string, number>
      expected_cash: number
      closing_difference: number
    }>(`/api/v1/cash/sessions/${refundCash.id}/close`, {
      method: 'POST',
      body: {
        closing_amount: 0,
        closing_by_method: {
          pix: -10,
          debit: 0,
          credit: 0,
          transfer: 0,
          voucher: 0,
        },
        notes: 'net-negative digital close E2E',
      },
    })
  }, suffix)

  expect(result.expected_cash).toBe(0)
  expect(result.closing_difference).toBe(0)
  expect(result.expected_by_method.pix).toBe(-10)
  expect(result.declared_by_method.pix).toBe(-10)
  expect(result.difference_by_method.pix).toBe(0)
})
